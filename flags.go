// Copyright 2013 Vadim Vygonets
// This program is free software.  It comes without any warranty, to
// the extent permitted by applicable law.  You can redistribute it
// and/or modify it under the terms of the Do What The Fuck You Want
// To Public License, Version 2, as published by Sam Hocevar.  See
// the LICENSE file or http://sam.zoy.org/wtfpl/ for more details.

package main

import (
	"errors"
	"os"
	"strconv"

	"github.com/unixdj/conf"
)

func die(err error) {
	os.Stderr.WriteString(err.Error() + "\n")
	os.Exit(1)
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

func help(e string) error {
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
	if e == "true" { // -h given on command line
		os.Exit(0)
	}
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
			return errors.New("-b, -C, -G, -o and -r are incompatible")
		}
		g.dumper = v
		return nil
	})
}

func parseFlags() []string {
	vars := []conf.Var{
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
	g.length = -1
	g.pkg = "main"
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
