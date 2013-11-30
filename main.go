package main

import (
	"bufio"
	"errors"
	"io"
	"os"
	"strconv"

	"github.com/unixdj/conf"
)

func die(err error) {
	os.Stderr.WriteString(err.Error() + "\n")
	os.Exit(1)
}

func hexb(buf []byte, b byte) {
	const digits = "0123456789abcdef"
	buf[0], buf[1] = digits[b>>4&0xf], digits[b&0xf]
}

func hex32(buf []byte, n int64) {
	hexb(buf[0:], byte(n>>24))
	hexb(buf[2:], byte(n>>16))
	hexb(buf[4:], byte(n>>8))
	hexb(buf[6:], byte(n))
}

func printable(c byte) byte {
	if c >= 0x20 && c < 0x7f {
		return c
	}
	return '.'
}

func cut(buf []byte, size, skip int) ([]byte, []byte) {
	if size < 0 {
		size += len(buf)
		if size > 0 {
			return buf[size:], buf[:size-skip]
		}
	} else if size < len(buf) {
		return buf[:size], buf[size+skip:]
	}
	return buf, nil
}

func prepare(outb []byte) (out, chars []byte) {
	for k := range outb {
		outb[k] = ' '
	}
	hex32(outb, g.pos)
	outb[8] = ':'
	outb[len(outb)-1] = '\n'
	return outb[10 : len(outb)-g.cols-3], outb[len(outb)-g.cols-1:]
}

func prepareC(outb []byte) (out, chars []byte) {
	out = outb[1:]
	for k := range out {
		out[k] = ' '
	}
	outb[0] = '\t'
	outb[len(outb)-1] = '\n'
	return
}

func dumpGroupBin(in, out []byte) {
	var pos, adj = 0, 8
	if g.le {
		pos, adj = len(out)-8, -8
	}
	for _, v := range in {
		for i := pos + 7; i >= pos; i-- {
			out[i] = '0' | v&1
			v >>= 1
		}
		pos += adj
	}
}

func dumpGroup(in, out []byte) {
	var pos, adj = 0, 2
	if g.le {
		pos, adj = len(out)-2, -2
	}
	for _, v := range in {
		hexb(out[pos:], v)
		pos += adj
	}
}

func octDigits(bytes int) int { return (bytes*8 + 2) / 3 }

func dumpSubGroupOct(in, out []byte) {
	var (
		n  uint64
		od = octDigits(len(in))
	)
	if g.le {
		for k, v := range in {
			n |= uint64(v) << uint(k<<3)
		}
		out = out[len(out)-od:]
	} else {
		for _, v := range in {
			n = n<<8 | uint64(v)
		}
	}
	for od > 0 {
		od--
		out[od] = '0' | byte(n)&7
		n >>= 3
	}
}

func dumpGroupOct(in, out []byte) {
	var adj = -6
	if g.le {
		adj = 6
	}
	for len(in) > 0 {
		var sub, out1 []byte
		sub, in = cut(in, adj, 0)
		out1, out = cut(out, -16, 0)
		dumpSubGroupOct(sub, out1)
	}
}

func dumpGroupC(in, out []byte) {
	for _, v := range in {
		out[0], out[1], out[4] = '0', 'x', ','
		hexb(out[2:], v)
		out = out[6:]
	}
}

func emptyString() string { return "" }

func cHeader() string {
	if g.ident == "" {
		return ""
	}
	return "char " + g.ident + "[] = {\n"
}

func cFooter() string {
	if g.ident == "" {
		return ""
	}
	return "};\nunsigned int " + g.ident + "_len = " +
	strconv.FormatInt(g.size, 10) + ";\n"
}

const (
	HexDumper = iota
	BinDumper
	OctDumper
	CDumper
	Undumper = -1
)

var (
	g = struct {
		ident, outfile          string
		pos, seek, length, size int64
		cols, group, dumper     int
		le, rev                 bool
	}{}

	dumpers = []struct {
		defCols, defGroup int
		lineLen, groupLen func() int
		header, footer    func() string
		prepare           func(outb []byte) (out, chars []byte)
		dump              func(in, out []byte)
	}{{ // HexDumper
		16, 4,
		func() int { return 13 + g.cols*3 + (g.cols-1)/g.group },
		func() int { return g.group * 2 },
		emptyString, emptyString, prepare, dumpGroup,
	}, { // BinDumper
		6, 1,
		func() int { return 13 + g.cols*9 + (g.cols-1)/g.group },
		func() int { return g.group * 8 },
		emptyString, emptyString, prepare, dumpGroupBin,
	}, { // OctDumper
		16, 4,
		func() int {
			return 13 +
			g.cols/g.group*octDigits(g.group) +
			octDigits(g.cols%g.group) +
			(g.cols-1)/g.group + g.cols
		},
		func() int { return octDigits(g.group) },
		emptyString, emptyString, prepare, dumpGroupOct,
	}, { // CDumper
		12, 12,
		func() int { return 1 + g.cols*6 + (g.cols-1)/g.group },
		func() int { return g.group * 6 },
		cHeader, cFooter, prepareC, dumpGroupC,
	}}
)

func dump(stdin *bufio.Reader, stdout *bufio.Writer) {
	var (
		inb   = make([]byte, g.cols)
		outb  = make([]byte, dumpers[g.dumper].lineLen())
		gsadj = dumpers[g.dumper].groupLen()
		n     int
	)
	_, err := stdout.WriteString(dumpers[g.dumper].header())
	for err == nil {
		if g.length != -1 && g.length < int64(len(inb)) {
			if g.length == 0 {
				break
			}
			inb = inb[:g.length]
		}

		n, err = io.ReadFull(stdin, inb)
		if err != nil && err != io.ErrUnexpectedEOF {
			break
		}

		in := inb[:n]
		out, chars := dumpers[g.dumper].prepare(outb)
		if len(chars) != 0 {
			for k, v := range in {
				chars[k] = printable(v)
			}
		}
		for len(in) > 0 {
			var grp, out1 []byte
			grp, in = cut(in, g.group, 0)
			out1, out = cut(out, gsadj, 1)
			dumpers[g.dumper].dump(grp, out1)
		}

		_, err = stdout.Write(outb)
		g.pos += int64(n)
		g.size += int64(n)
		if g.length != -1 {
			g.length -= int64(n)
		}
	}
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		die(err)
	}
	if _, err = stdout.WriteString(dumpers[g.dumper].footer()); err != nil {
		die(err)
	}
	if err = stdout.Flush(); err != nil {
		die(err)
	}
}

func hexdig(r rune) (byte, bool) {
	switch {
	case r >= '0' && r <= '9':
		return byte(r) - '0', true
	case r >= 'A' && r <= 'F':
		return byte(r) - ('A' - 0xa), true
	case r >= 'a' && r <= 'f':
		return byte(r) - ('a' - 0xa), true
	}
	return 0, false
}

func errSyntax(s string, pos int) {
	var e string
	if pos == len(s) {
		e += "syntax error at EOL: " + s
	} else {
		e += "syntax error: " + s[:pos] + " >>>" + s[pos:pos+1] +
		"<<< " + s[pos+1:]
	}
	die(errors.New(e))
}

// syntax:
//	[0-9A-Fa-f]+:( ?[0-9A-Fa-f]{2})(  .*)?
//	[0-9A-Fa-f]+ ([0-9A-Fa-f]{2} *)([|].*)?
// i.e. if address followed by a colon, parse until a double
// space, otherwise until a pipe character; or EOL in both cases.
func undump(stdin *bufio.Scanner, stdout *os.File) {
	var outb = make([]byte, 0, 64)
	for stdin.Scan() {
		var (
			addr int64
			pos  int
			v    rune
			line = stdin.Text()
		)
		for pos, v = range line {
			if dig, ok := hexdig(v); ok {
				addr = addr<<4 | int64(dig)
			} else {
				break
			}
		}
		if pos == 0 || pos == len(line) {
			errSyntax(line, pos)
		}
		var (
			xxdStyle, spaced, hexed bool
			dig                     byte
		)
		if line[pos] == ':' {
			pos++
			xxdStyle = true
		}
		outb = outb[:0]
		scanloop:
		for k, v := range line[pos:] {
			switch {
			case hexed:
				if d2, ok := hexdig(v); ok {
					outb = append(outb, dig<<4|d2)
					hexed = false
				} else {
					errSyntax(line, pos+k)
				}
			case v == ' ':
				if xxdStyle {
					if spaced {
						break scanloop
					}
					spaced = true
				}
			case v == '|':
				break scanloop
			default:
				spaced = false
				if dig, hexed = hexdig(v); !hexed {
					errSyntax(line, pos+k)
				}
			}
		}
		if hexed {
			errSyntax(line, len(line))
		}
		if _, err := stdout.WriteAt(outb, addr-int64(g.pos)); err != nil {
			die(err)
		}
	}
	if err := stdin.Err(); err != nil {
		die(err)
	}
}

func help(e string) error {
	os.Stderr.WriteString("Usage:\n  ")
	os.Stderr.WriteString(os.Args[0])
	os.Stderr.WriteString(" [-behio] [-c bytes] [-d off] [-g bytes] [-l len] [-s [-]off] [file ...]\n  ")
	os.Stderr.WriteString(os.Args[0])
	os.Stderr.WriteString(` -r [-d [-]<off>] [-O outfile] [file ...]
Options:
  -b        Binary dump
  -c bytes  Number of bytes per line (default: 16, -b: 6, -i: 12)
  -d off    Add <off> to displayed addresses; -r: subtract from address
  -e        Little endian byte order hexdump
  -g bytes  Number of bytes per group (default: 4, -b: 1, -i: 12)
  -h        Show this help
  -i        Dump in C include file format
  -l len    Stop after <len> bytes
  -o        Octal dump
  -O        Output file to be opened without truncating
  -r        Reverse big-endian hexdump
  -s [-]off Seek <off> bytes in first input file (negative: from EOF)
`)
	os.Exit(2)
	// NOTREACHED
	return nil
}

type smallValue int

func (v *smallValue) Set(s string) error {
	u, err := strconv.ParseUint(s, 0, 11)
	if err != nil {
		return err.(*strconv.NumError).Err
	}
	if u == 0 {
		return errors.New("value cannot be zero")
	}
	*v = smallValue(u)
	return nil
}

type int63Value int64

func (v *int63Value) Set(s string) error {
	u, err := strconv.ParseUint(s, 0, 63)
	if err != nil {
		// strip fluff from strconf.ParseUint
		return err.(*strconv.NumError).Err
	}
	*v = int63Value(u)
	return nil
}

func setDumperValue(v int) conf.Value {
	return conf.FuncValue(func(string) error {
		if g.dumper != 0 {
			return errors.New("-b, -i, -o and -r are incompatible")
		}
		g.dumper = v
		return nil
	})
}

func parseFlags() {
	vars := []conf.Var{
		{Flag: 'b', Kind: conf.NoArg, Val: setDumperValue(BinDumper)},
		{Flag: 'c', Val: (*smallValue)(&g.cols)},
		{Flag: 'd', Val: (*conf.Int64Value)(&g.pos)},
		{Flag: 'g', Val: (*smallValue)(&g.group)},
		{Flag: 'e', Kind: conf.NoArg, Val: (*conf.BoolValue)(&g.le)},
		{Flag: 'h', Kind: conf.NoArg, Val: conf.FuncValue(help)},
		{Flag: 'i', Kind: conf.NoArg, Val: setDumperValue(CDumper)},
		{Flag: 'l', Val: (*int63Value)(&g.length)},
		{Flag: 'o', Kind: conf.NoArg, Val: setDumperValue(OctDumper)},
		{Flag: 'O', Val: (*conf.StringValue)(&g.outfile)},
		{Flag: 'r', Kind: conf.NoArg, Val: setDumperValue(Undumper)},
		{Flag: 's', Val: (*conf.Int64Value)(&g.seek)},
	}
	g.length = -1
	if err := conf.GetOpt(vars); err != nil {
		os.Stderr.WriteString(err.Error() + "\n")
		help("")
	}
	if g.dumper == Undumper {
		g.rev = true
		return
	}
	if g.cols == 0 {
		g.cols = dumpers[g.dumper].defCols
	}
	if g.group == 0 {
		g.group = dumpers[g.dumper].defGroup
	}
	return
}

func isdigit(b byte) bool { return b >= '0' && b <= '9' }
func isalpha(b byte) bool { return b >= 'A' && b <= 'Z' || b >= 'a' && b <= 'z' }

func makeIdent(s string) string {
	id := make([]byte, 0, len(s)+1)
	if isdigit(s[0]) {
		id = append(id, '_')
	}
	for _, v := range s {
		var b byte = '_'
		if isdigit(byte(v)) || isalpha(byte(v)) {
			b = byte(v)
		}
		id = append(id, b)
	}
	return string(id)
}

func main() {
	var (
		stdin  = os.Stdin
		stdout = os.Stdout
	)
	parseFlags()

	// open first file
	if len(conf.Args) != 0 {
		var err error
		if stdin, err = os.Open(conf.Args[0]); err != nil {
			die(err)
		}
		g.ident = makeIdent(conf.Args[0])
	}

	// seek first file
	if g.seek != 0 {
		var whence = os.SEEK_SET
		if g.seek < 0 {
			whence = os.SEEK_END
		}
		pos, err := stdin.Seek(g.seek, whence)
		if err != nil {
			die(err)
		}
		if !g.rev {
			g.pos += pos
		}
	}

	// open rest of files
	var reader io.Reader = stdin
	if len(conf.Args) > 1 {
		var (
			files = make([]io.Reader, len(conf.Args))
			err   error
		)
		files[0] = reader
		for k, v := range conf.Args[1:] {
			if files[k+1], err = os.Open(v); err != nil {
				die(err)
			}
		}
		reader = io.MultiReader(files...)
	}

	// open output file
	if g.outfile != "" {
		var err error
		stdout, err = os.OpenFile(g.outfile, os.O_WRONLY, 0666)
		if err != nil {
			die(err)
		}
	}

	if g.rev {
		undump(bufio.NewScanner(reader), stdout)
	} else {
		dump(bufio.NewReader(reader), bufio.NewWriter(stdout))
	}
}
