package markdown

import (
	"slices"
	"unicode/utf8"
)

//go:generate go run gen_entities.go

type entity struct {
	name, chars string
}

// maxEntityName is the length of the longest entity name.
const maxEntityName = 31

// entityEnd returns the end of the entity or numeric character reference at
// src[i:end], which starts with '&', or 0: a known entity name, '#' and 1 to
// 7 decimal digits, or '#', 'x' or 'X' and 1 to 6 hexadecimal digits, then
// ';' (CM 25 to 27).
func entityEnd(src []byte, i, end uint32) uint32 {
	j := i + 1
	isDigit, most := isASCIIAlphanumeric, uint32(maxEntityName)
	if j < end && src[j] == '#' {
		j++
		isDigit, most = isDecimalDigit, 7
		if j < end && src[j]|0x20 == 'x' {
			j++
			isDigit, most = isHexDigit, 6
		}
	}
	start := j
	for j < end && j-start < most && isDigit(src[j]) {
		j++
	}
	switch {
	case j == start || j == end || src[j] != ';':
		return 0
	case src[i+1] != '#':
		if _, ok := lookupEntity(src[start:j]); !ok {
			return 0
		}
	}
	return j + 1
}

// appendEntityValue appends the characters of reference ref, an entity or
// numeric character reference, to dst. A numeric reference to NUL, to a
// surrogate or beyond U+10FFFF gives U+FFFD.
func appendEntityValue(dst, ref []byte) []byte {
	name := ref[1 : len(ref)-1]
	if name[0] != '#' {
		e, _ := lookupEntity(name)
		return append(dst, e.chars...)
	}
	base, digits := rune(10), name[1:]
	if digits[0]|0x20 == 'x' {
		base, digits = 16, digits[1:]
	}
	var r rune
	for _, c := range digits {
		d := rune(c - '0')
		if c > '9' {
			d = rune(c|0x20-'a') + 10
		}
		r = r*base + d
	}
	if r == 0 || !utf8.ValidRune(r) {
		r = utf8.RuneError
	}
	return utf8.AppendRune(dst, r)
}

func lookupEntity(name []byte) (entity, bool) {
	i, ok := slices.BinarySearchFunc(entities[:], name, func(e entity, name []byte) int {
		switch {
		case e.name < string(name):
			return -1
		case e.name > string(name):
			return 1
		}
		return 0
	})
	if !ok {
		return entity{}, false
	}
	return entities[i], true
}

func isASCIIAlphanumeric(c byte) bool {
	return isASCIILetter(c) || isDecimalDigit(c)
}

func isDecimalDigit(c byte) bool {
	return '0' <= c && c <= '9'
}

func isHexDigit(c byte) bool {
	return isDecimalDigit(c) || 'a' <= c|0x20 && c|0x20 <= 'f'
}
