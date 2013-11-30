// Copyright 2013 Vadim Vygonets
// This program is free software.  It comes without any warranty, to
// the extent permitted by applicable law.  You can redistribute it
// and/or modify it under the terms of the Do What The Fuck You Want
// To Public License, Version 2, as published by Sam Hocevar.  See
// the LICENSE file or http://sam.zoy.org/wtfpl/ for more details.

package main

import (
	"bufio"
	"io"
	"os"
	"strconv"
)

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

func main() {
	var (
		stdin  = os.Stdin
		stdout = os.Stdout
		args   = parseFlags()
	)

	// open first file
	if len(args) != 0 {
		var err error
		if stdin, err = os.Open(args[0]); err != nil {
			die(err)
		}
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
	if len(args) > 1 {
		var (
			files = make([]io.Reader, len(args))
			err   error
		)
		files[0] = reader
		for k, v := range args[1:] {
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
