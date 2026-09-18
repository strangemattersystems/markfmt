package markdown

import "bytes"

// bom is the UTF-8 byte order mark. At offset 0 it is a BOM leaf, and the
// first line starts after it.
const bom = "\xEF\xBB\xBF"

type line struct {
	start, end uint32 // content bytes, without the line ending
	eol        uint32 // end of the line ending: end, end+1 (LF or CR) or end+2 (CRLF)
}

// lines iterates over the lines of an input. LF, CR and CRLF are line
// endings, and no other code looks for them.
type lines struct {
	src       []byte
	pos, size uint32
	lf        uint32 // the next line feed at or after pos, or size
}

func newLines(src []byte) lines {
	it := lines{src: src, size: inputSize(src)}
	if bytes.HasPrefix(src, []byte(bom)) {
		it.pos = uint32(len(bom))
	}
	it.findLF()
	return it
}

// findLF sets lf to the next line feed at or after pos, or to size.
func (it *lines) findLF() {
	it.lf = it.size
	if j := bytes.IndexByte(it.src[it.pos:it.size], '\n'); j >= 0 {
		it.lf = it.pos + count(j)
	}
}

// next returns the next line, or false after the last line. The last line
// has no line ending when the input does not end with one.
func (it *lines) next() (line, bool) {
	if it.pos == it.size {
		return line{}, false
	}
	// A line feed is looked for first, because a carriage return is rare and
	// only the bytes before the line feed can hold one. The line feed is kept
	// until a line passes it: lines that end with a carriage return would
	// otherwise search the rest of the input each, which is O(n^2).
	if it.lf < it.pos {
		it.findLF()
	}
	i := it.lf
	if j := bytes.IndexByte(it.src[it.pos:i], '\r'); j >= 0 {
		i = it.pos + count(j)
	}
	l := line{start: it.pos, end: i, eol: i}
	switch {
	case i == it.size:
	case it.src[i] == '\r' && i+1 < it.size && it.src[i+1] == '\n':
		l.eol += 2
	default:
		l.eol++
	}
	it.pos = l.eol
	return l, true
}
