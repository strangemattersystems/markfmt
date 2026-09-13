package markdown

import (
	"cmp"
	"slices"
	"unicode/utf8"
)

//go:generate go run gen_casefold.go

// decodeRune decodes the first character of b, which is not empty, as the
// WHATWG UTF-8 decoder does: an invalid sequence gives U+FFFD for its longest
// start that could still be valid, and NUL gives U+FFFD (design 6.7). It
// returns the character and its length.
func decodeRune(b []byte) (rune, int) {
	r, n := utf8.DecodeRune(b)
	switch {
	case r == 0:
		return utf8.RuneError, 1
	case r != utf8.RuneError || n > 1:
		return r, n
	}
	need, lo, hi := 0, byte(0x80), byte(0xBF)
	switch c := b[0]; {
	case 0xC2 <= c && c <= 0xDF:
		need = 1
	case 0xE0 <= c && c <= 0xEF:
		need = 2
		switch c {
		case 0xE0:
			lo = 0xA0
		case 0xED:
			hi = 0x9F
		}
	case 0xF0 <= c && c <= 0xF4:
		need = 3
		switch c {
		case 0xF0:
			lo = 0x90
		case 0xF4:
			hi = 0x8F
		}
	}
	i := 1
	for i <= need && i < len(b) && lo <= b[i] && b[i] <= hi {
		i, lo, hi = i+1, 0x80, 0xBF
	}
	return utf8.RuneError, i
}

// valueReader reads the value of content bytes one piece at a time: each
// NUL and each maximal invalid UTF-8 subsequence is U+FFFD (design 6.7).
type valueReader struct {
	b     []byte
	char  [8]byte // the value of an EntityRef
	charN int
}

func (r *valueReader) next() []byte {
	if n := r.charN; n > 0 {
		r.charN = 0
		return r.char[:n:n]
	}
	b, i := r.b, 0
	for i < len(b) && b[i] != 0 {
		if b[i] < utf8.RuneSelf {
			i++
			continue
		}
		c, n := utf8.DecodeRune(b[i:])
		if c == utf8.RuneError && n == 1 {
			break
		}
		i += n
	}
	switch {
	case len(b) == 0:
		return nil
	case i > 0:
		r.b = b[i:]
		return b[:i:i]
	}
	_, n := decodeRune(b)
	r.b = b[n:]
	return slices.Clip(replacement)
}

var replacement = []byte("\uFFFD")

// newValueReader returns a reader of the value of content leaf m: an Escape
// is its character, an EntityRef its characters, and a VerbatimLineEnding a
// line feed.
func (t *Tree) newValueReader(m Node) valueReader {
	switch m.kind {
	case Escape:
		return valueReader{b: t.src[m.start+1 : m.end]}
	case EntityRef:
		var r valueReader
		r.charN = len(appendEntityValue(r.char[:0], t.src[m.start:m.end]))
		return r
	case VerbatimLineEnding:
		return valueReader{b: lineFeed}
	}
	return valueReader{b: t.src[m.start:m.end]}
}

// AppendValue appends the value of content leaf id to dst.
func (t *Tree) AppendValue(dst []byte, id NodeID) []byte {
	r := t.newValueReader(t.nodes[id])
	for b := r.next(); b != nil; b = r.next() {
		dst = append(dst, b...)
	}
	return dst
}

type caseFold struct {
	r  rune
	to string
}

// appendFold appends the Unicode full case folding of r to dst.
func appendFold(dst []byte, r rune) []byte {
	if r < utf8.RuneSelf {
		if 'A' <= r && r <= 'Z' {
			r += 'a' - 'A'
		}
		return utf8.AppendRune(dst, r)
	}
	if i, ok := slices.BinarySearchFunc(caseFolds[:], r, func(f caseFold, r rune) int {
		return cmp.Compare(f.r, r)
	}); ok {
		return append(dst, caseFolds[i].to...)
	}
	return utf8.AppendRune(dst, r)
}

// labelFolder appends a normalized label to dst, one piece of label bytes at
// a time: UTF-8 decoding, Unicode full case folding, and each run of spaces,
// tabs and line endings as one space, with none at either end (design 6.7).
type labelFolder struct {
	dst   []byte
	start int  // length of dst before the label
	space bool // a run of spaces, tabs and line endings is pending
}

func (f *labelFolder) write(b []byte) {
	for len(b) > 0 {
		switch b[0] {
		case ' ', '\t', '\n', '\r':
			f.space = len(f.dst) > f.start
			b = b[1:]
			continue
		}
		if f.space {
			f.dst = append(f.dst, ' ')
			f.space = false
		}
		r, n := decodeRune(b)
		f.dst = appendFold(f.dst, r)
		b = b[n:]
	}
}

// isASCIIPunct reports whether c is ASCII punctuation, which a backslash
// escapes.
func isASCIIPunct(c byte) bool {
	return '!' <= c && c <= '/' || ':' <= c && c <= '@' || '[' <= c && c <= '`' || '{' <= c && c <= '~'
}

func isASCIILetter(c byte) bool {
	return 'a' <= c|0x20 && c|0x20 <= 'z'
}
