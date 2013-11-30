package main

import (
	"bufio"
	"errors"
	"io"
	"os"
	"strconv"

	"github.com/unixdj/conf"
)

const hexx = "0123456789abcdef"

var bc = make(chan []byte, 16)

func get(cols int) []byte {
	select {
	case b := <-bc:
		return b
	default:
	}
	return make([]byte, cols)
}

func put(b []byte) {
	select {
	case bc <- b:
	default:
	}
}

func hexb(buf []byte, b byte) {
	buf[0], buf[1] = hexx[b>>4&0xf], hexx[b&0xf]
}

func hex32(buf []byte, n uint64) {
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

func cut(buf []byte, cut, skip int) (start, rest []byte) {
	if cut > len(buf) {
		return buf, nil
	}
	start, rest = buf[:cut], buf[cut:]
	if skip <= len(rest) {
		rest = rest[skip:]
	}
	return
}

func dumpGroupBin(in, out, chars []byte) {
	var pos int
	for k, v := range in {
		chars[k] = printable(v)
		for i := pos + 7; i >= pos; i-- {
			out[i] = '0' | v&1
			v >>= 1
		}
		pos += 9
	}
}

func dumpGroup(in, out, chars []byte) {
	var pos, adj = 0, 2
	if g.le {
		pos, adj = len(out)-2, -2
	}
	for k, v := range in {
		chars[k] = printable(v)
		hexb(out[pos:], v)
		pos += adj
	}
}

const (
	HexDumper = iota
	BinDumper
	CDumper
)

var (
	g = struct {
		pos                 uint64
		cols, group, dumper int
		le                  bool
	}{}
	dumpers = []struct {
		defCols, defGroup int
		lineLen, groupLen func() int
		dump              func(in, out, chars []byte)
	}{
		{
			16, 4,
			func() int {
				return 13 + g.cols*3 + (g.cols-1)/g.group
			},
			func() int { return g.group * 2 },
			dumpGroup,
		},
		{
			6, 1,
			func() int {
				return 13 + g.cols*10 + (g.cols-1)/g.group
			},
			func() int { return g.group * 9 },
			dumpGroupBin,
		},
	}
)

//           1         2         3         4         5         6         7
// 0123456789012345678901234567890123456789012345678901234567890123456789
// 00000000: 666f6f20 62617220 62617a20 71757578  foo bar baz quux
// cols=16 groupSize=4
func dump(c <-chan []byte) {
	var (
		slen  = dumpers[g.dumper].lineLen()
		gsadj = dumpers[g.dumper].groupLen()
		//dumpGroup = dumpers[g.dumper].dump
		outb   = make([]byte, slen)
		stdout = bufio.NewWriter(os.Stdout)
	)
	for buf := range c {
		for k := range outb {
			outb[k] = byte(' ')
		}
		hex32(outb[:8], g.pos)
		outb[8] = ':'

		in, out, chars := buf, outb[10:slen-g.cols-3], outb[slen-g.cols-1:]
		for len(in) > 0 {
			var grp, out1, char1 []byte
			grp, in = cut(in, g.group, 0)
			out1, out = cut(out, gsadj, 1)
			char1, chars = cut(chars, g.group, 0)
			dumpers[g.dumper].dump(grp, out1, char1)
		}

		outb[slen-1] = '\n'
		stdout.Write(outb[:])
		g.pos += uint64(len(buf))
		put(buf[:cap(buf)])
	}
	stdout.Flush()
}

func feed(c chan<- []byte) {
	var (
		n     int
		err   error
		stdin = bufio.NewReader(os.Stdin)
	)
	for err != io.ErrUnexpectedEOF {
		buf := get(g.cols)
		n, err = io.ReadFull(stdin, buf)
		if err == io.EOF {
			put(buf)
			break
		}
		c <- buf[:n]
	}
	close(c)
}

func help(e string) error {
	os.Stderr.WriteString("Usage:\n\t")
	os.Stderr.WriteString(os.Args[0])
	os.Stderr.WriteString(` [-bceghi]

Options:
  -b        Binary dump
  -c bytes  Number of bytes per line (default: 16, -b: 6)
  -e        Little endian byte order hexdump
  -g bytes  Number of bytes per group (default: 4, -b: 1)
  -h        Show this help
  -i        Dump in C include file format
`)
	os.Exit(2)
	// NOTREACHED
	return nil
}

type smallValue int

func (v *smallValue) Set(s string) error {
	u, err := strconv.ParseUint(s, 0, 8)
	if err != nil {
		return err.(*strconv.NumError).Err
	}
	if u == 0 {
		return errors.New("value cannot be zero")
	}
	/*
		if u&(u-1) != 0 {
			return errors.New("value must be a power of two")
		}
	*/
	*v = smallValue(u)
	return nil
}

func parseFlags() {
	setDumperValue := func(v int) conf.Value {
		return conf.FuncValue(func(string) error {
			if g.dumper != 0 {
				return errors.New("-b and -i are incompatible")
			}
			g.dumper = v
			return nil
		})
	}
	vars := []conf.Var{
		{Flag: 'b', Kind: conf.NoArg, Val: setDumperValue(BinDumper)},
		{Flag: 'c', Val: (*smallValue)(&g.cols)},
		{Flag: 'g', Val: (*smallValue)(&g.group)},
		{Flag: 'e', Kind: conf.NoArg, Val: (*conf.BoolValue)(&g.le)},
		{Flag: 'h', Kind: conf.NoArg, Val: conf.FuncValue(help)},
		{Flag: 'i', Kind: conf.NoArg, Val: setDumperValue(CDumper)},
	}
	if err := conf.GetOpt(vars); err != nil {
		os.Stderr.WriteString(err.Error() + "\n")
		help("")
	}
	if g.cols == 0 {
		g.cols = dumpers[g.dumper].defCols
	}
	if g.group == 0 {
		g.group = dumpers[g.dumper].defGroup
	}
	return
}

func main() {
	parseFlags()
	var c = make(chan []byte, 8)
	go feed(c)
	dump(c)
}
