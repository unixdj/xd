// Copyright 2013 Vadim Vygonets
// This program is free software.  It comes without any warranty, to
// the extent permitted by applicable law.  You can redistribute it
// and/or modify it under the terms of the Do What The Fuck You Want
// To Public License, Version 2, as published by Sam Hocevar.  See
// the LICENSE file or http://sam.zoy.org/wtfpl/ for more details.

package main

import (
	"bufio"
	"errors"
	"io"
	"os"
	"strconv"

	"github.com/unixdj/conf"
)

// g is global state, initially set from flags.
var g = struct {
	ident, pkg, outfile string // -V, -P, -O
	pos, seek, length   int64  // -d + size, -s, -l - size || -1
	size                int64  // bytes read so far
	cols, group, dumper int    // -c, -g, -[bCGor] || HexDumper
	le, rev             bool   // -e, (-r)
}{
	pkg:    "main",
	length: -1,
}

func die(err error) {
	os.Stderr.WriteString(err.Error() + "\n")
	os.Exit(1)
}

func isdigit(r rune) bool { return r >= '0' && r <= '9' }
func isalpha(r rune) bool { return r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' }

// makeIdent makes an identifier valid in both C and Go out of s
// by replacing invalid characters with an underscore.
func makeIdent(s string) string {
	id := make([]byte, 0, len(s)+1)
	if s != "" && isdigit(rune(s[0])) {
		id = append(id, '_')
	}
	for _, v := range s {
		var b byte = '_'
		if isdigit(v) || isalpha(v) {
			b = byte(v)
		}
		id = append(id, b)
	}
	return string(id)
}

// help prints help and exits.
func help(v string) error {
	os.Stderr.WriteString("Usage:\n  ")
	os.Stderr.WriteString(os.Args[0])
	os.Stderr.WriteString(" [-beo] [-c bytes] [-d off] [-g bytes] [-l len] [-s off] [file ...]\n  ")
	os.Stderr.WriteString(os.Args[0])
	os.Stderr.WriteString(" -C | -G [-P pkg] [-V var] [-c bytes] [-l len] [-s off] [file ...]\n  ")
	os.Stderr.WriteString(os.Args[0])
	os.Stderr.WriteString(" -r [-d off] [-O outfile] [file ...]\n  ")
	os.Stderr.WriteString(os.Args[0])
	os.Stderr.WriteString(` -h
Options:
  -b         Binary dump
  -c bytes   Number of bytes per line (default: 16, -b: 6, -C/-G: 12)
  -C         Dump as C source array
  -d off     Add <off> to displayed addresses; -r: to addresses read from input
  -e         Little endian byte order
  -g bytes   Number of bytes per group (default: 4, -b: 1)
  -G         Dump as Go source slice
  -h         Show this help
  -l len     Stop after <len> bytes
  -o         Octal dump
  -O outfile Output file to be opened without truncating
  -P pkg     Go package name (default: "main")
  -r         Reverse big-endian hexdump
  -s off     Seek <off> bytes in first input file (negative: from EOF)
  -V var     C/Go variable name (default: 1st filename or -C: none, -G: "dump")
`)
	if v == "true" { // -h given on command line
		os.Exit(0)
	}
	os.Exit(2)
	// NOTREACHED
	return nil
}

// smallValue is a conf.Value representing a small strictly
// positive integer.
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

// int63Value is a conf.Value representing a non-negative integer
// that fits into int64.
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

// setDumperValue returns a conf.Value whose Set method sets
// g.dumper to v.
func setDumperValue(v int) conf.Value {
	return conf.FuncValue(func(string) error {
		if g.dumper != 0 {
			return errors.New("-b, -C, -G, -o and -r are incompatible")
		}
		g.dumper = v
		return nil
	})
}

func parseFlags() []string {
	var vars = []conf.Var{
		{Flag: 'h', Kind: conf.NoArg, Val: conf.FuncValue(help)},
		{Flag: 'b', Kind: conf.NoArg, Val: setDumperValue(BinDumper)},
		{Flag: 'o', Kind: conf.NoArg, Val: setDumperValue(OctDumper)},
		{Flag: 'C', Kind: conf.NoArg, Val: setDumperValue(CDumper)},
		{Flag: 'G', Kind: conf.NoArg, Val: setDumperValue(GoDumper)},
		{Flag: 'r', Kind: conf.NoArg, Val: setDumperValue(Undumper)},
		{Flag: 'e', Kind: conf.NoArg, Val: (*conf.BoolValue)(&g.le)},
		{Flag: 'c', Val: (*smallValue)(&g.cols)},
		{Flag: 'g', Val: (*smallValue)(&g.group)},
		{Flag: 'l', Val: (*int63Value)(&g.length)},
		{Flag: 'd', Val: (*conf.Int64Value)(&g.pos)},
		{Flag: 's', Val: (*conf.Int64Value)(&g.seek)},
		{Flag: 'O', Val: (*conf.StringValue)(&g.outfile)},
		{Flag: 'P', Val: (*conf.StringValue)(&g.pkg)},
		{Flag: 'V', Val: (*conf.StringValue)(&g.ident)},
	}
	if err := conf.GetOpt(vars); err != nil {
		os.Stderr.WriteString(err.Error() + "\n")
		help("")
	}
	if g.dumper == Undumper {
		g.rev = true
		if g.le {
			os.Stderr.WriteString("-e and -r are incompatible\n")
			help("")
		}
	} else {
		if g.cols == 0 {
			g.cols = dumpers[g.dumper].defCols
		}
		if g.group == 0 {
			g.group = dumpers[g.dumper].defGroup
		}
		if g.ident == "" && len(conf.Args) != 0 {
			g.ident = makeIdent(conf.Args[0])
		}
	}
	return conf.Args
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
		stdout, err = os.OpenFile(g.outfile, os.O_WRONLY|os.O_CREATE, 0666)
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
