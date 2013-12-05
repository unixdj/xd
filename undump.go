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
	"os"
)

// hexdig parses the hexadecimal digit in r.  If parsing was
// successful, hexdig returns the value of hexadecimal digit r
// and true.  Otherwise it returns 0 and false.
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

// dieSyntax prints a syntax error message and exits.
func dieSyntax(s string, pos int) {
	var e string
	if pos == len(s) {
		e += "syntax error at EOL: " + s
	} else {
		e += "syntax error: " + s[:pos] + " >>>" + s[pos:pos+1] +
			"<<< " + s[pos+1:]
	}
	die(errors.New(e))
}

// undump parses a hexdump from stdin and writes the resulting
// bytes to stdout.
//
// syntax:
//	[0-9A-Fa-f]+:( ?[0-9A-Fa-f]{2})(  .*)?
//	[0-9A-Fa-f]+ ([0-9A-Fa-f]{2} *)([|].*)?
//	\*
// i.e. if address followed by a colon, parse until a double
// space, otherwise until a pipe character; or EOL in both cases.
func undump(stdin *bufio.Scanner, stdout *os.File) {
	var outb = make([]byte, 0, 64)
	for stdin.Scan() {
		var (
			addr     int64
			pos      int
			v        rune
			nonempty bool
			line     = stdin.Text()
		)
		if line == "*" {
			continue // XXX: good for xxd but not POSIX hexdump
		}
		for pos, v = range line {
			if dig, ok := hexdig(v); ok {
				addr = addr<<4 | int64(dig)
			} else {
				nonempty = true
				break
			}
		}
		if pos == 0 {
			dieSyntax(line, pos)
		}
		if !nonempty {
			continue
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
					dieSyntax(line, pos+k)
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
					dieSyntax(line, pos+k)
				}
			}
		}
		if hexed {
			dieSyntax(line, len(line))
		}
		if _, err := stdout.WriteAt(outb, addr+g.pos); err != nil {
			die(err)
		}
	}
	if err := stdin.Err(); err != nil {
		die(err)
	}
}
