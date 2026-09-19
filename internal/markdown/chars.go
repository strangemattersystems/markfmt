package markdown

import (
	"cmp"
	"slices"
	"unicode"
	"unicode/utf8"
)

//go:generate go run gen_casefold.go
//go:generate go run gen_width.go

// decodeRune decodes the first character of b, which is not empty, as the
// WHATWG UTF-8 decoder does: an invalid sequence gives U+FFFD for its longest
// start that could still be valid, and NUL gives U+FFFD. It returns the
// character and its length.
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

// valueReader reads the value of content bytes one piece at a time: each NUL
// and each maximal invalid UTF-8 subsequence is U+FFFD. With escapes, each
// backslash escape is its character, and with entities, each entity reference
// is its characters.
type valueReader struct {
	b                 []byte
	escapes, entities bool
	char              [8]byte // the characters of the last entity reference
}

func (r *valueReader) next() []byte {
	b, i := r.b, 0
	for i < len(b) && b[i] != 0 && !r.decodes(b[i:]) {
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
	if r.decodes(b) {
		j := 2
		if b[0] == '&' {
			j = int(entityEnd(b, 0, count(len(b))))
			r.b = b[j:]
			return appendEntityValue(r.char[:0], b[:j])
		}
		r.b = b[j:]
		return b[1:2:2]
	}
	_, n := decodeRune(b)
	r.b = b[n:]
	return slices.Clip(replacement)
}

// decodes reports whether b starts with a backslash escape or an entity
// reference that the reader decodes.
func (r *valueReader) decodes(b []byte) bool {
	switch b[0] {
	case '\\':
		return r.escapes && len(b) > 1 && isASCIIPunct(b[1])
	case '&':
		return r.entities && entityEnd(b, 0, count(len(b))) > 0
	}
	return false
}

var replacement = []byte("\uFFFD")

var pipe = []byte("|")

// newValueReader returns a reader of the value of content leaf m. Escapes and
// entity references decode in an Escape, an EntityRef, a Destination, a Title
// and an InfoString, and entity references in an AutolinkText, as cmark
// decodes them. A VerbatimLineEnding is a line feed.
func (t *Tree) newValueReader(m Node) valueReader {
	switch m.kind {
	case Escape, EntityRef, Destination, Title, InfoString:
		return valueReader{b: t.src[m.start:m.end], escapes: true, entities: true}
	case AutolinkText:
		return valueReader{b: t.src[m.start:m.end], entities: true}
	case VerbatimLineEnding:
		return valueReader{b: lineFeed}
	case CellPipeEscape:
		return valueReader{b: pipe}
	}
	return valueReader{b: t.src[m.start:m.end]}
}

// AppendValue appends the value of content leaf id to dst.
func (t *Tree) AppendValue(dst []byte, id NodeID) []byte {
	r := t.newValueReader(t.nodes[id])
	return appendPieces(dst, &r)
}

// runeRange is the characters from lo to hi, both included.
type runeRange struct {
	lo, hi rune
}

// DisplayWidth returns the columns that b takes in a monospace font: 2 for a
// character whose East Asian Width is W or F, 0 for a control character or a
// nonspacing or enclosing mark, and 1 for another character or an invalid
// byte.
func DisplayWidth(b []byte) int {
	n := 0
	for len(b) > 0 {
		r, size := utf8.DecodeRune(b)
		b = b[size:]
		switch {
		case r < utf8.RuneSelf:
			if r >= 0x20 && r != 0x7f {
				n++
			}
		case size == 1:
			n++
		case unicode.In(r, unicode.Cc, unicode.Mn, unicode.Me):
		case isWide(r):
			n += 2
		default:
			n++
		}
	}
	return n
}

// isWide reports whether the East Asian Width of r is W or F.
func isWide(r rune) bool {
	_, ok := slices.BinarySearchFunc(wideRanges[:], r, func(g runeRange, r rune) int {
		switch {
		case g.hi < r:
			return -1
		case g.lo > r:
			return 1
		}
		return 0
	})
	return ok
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
// tabs and line endings as one space, with none at either end.
type labelFolder struct {
	dst   []byte
	start int  // length of dst before the label
	space bool // a run of spaces, tabs and line endings is pending
	link  bool // the label is a link label, where VT and FF are also spaces, as cmark reads them
}

func (f *labelFolder) write(b []byte) {
	for len(b) > 0 {
		switch b[0] {
		case '\v', '\f':
			if !f.link {
				break
			}
			fallthrough
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

// leaf writes the label bytes of a leaf of kind k with bytes b:
// none for a prefix or Indent leaf, a line feed for a line ending, b without
// the backslash of its pair for a cell pipe escape, and b for any other leaf.
func (f *labelFolder) leaf(k Kind, b []byte) {
	switch _, prefix := k.owner(); {
	case prefix, k == Indent:
	case k == LineEnding, k == VerbatimLineEnding:
		f.write(lineFeed)
	case k == CellPipeEscape:
		f.write(b[:len(b)-2])
		f.write(pipe)
	default:
		f.write(b)
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
