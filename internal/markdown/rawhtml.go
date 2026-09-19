package markdown

import (
	"bytes"
	"math"
	"strings"
)

// The closers that a raw HTML search looks for, as indices of closers and of
// inlineParser.failed.
const (
	closeComment = iota
	closeInstruction
	closeDeclaration
	closeCDATA
	closeDoubleQuote
	closeSingleQuote
)

var closers = [...][]byte{[]byte("-->"), []byte("?>"), []byte(">"), []byte("]]>"), []byte(`"`), []byte("'")}

// rawHTML pushes the raw HTML at i, on a line that ends at end, and reports
// whether there is one.
func (s *inlineParser) rawHTML(i, end uint32) bool {
	p, ok := s.rawHTMLEnd(i, end)
	if !ok {
		return false
	}
	n := len(s.pieces)
	s.verbatim(p, HTMLText)
	s.pieces[n].open = RawHTML
	s.pieces[len(s.pieces)-1].close = true
	return true
}

// rawHTMLEnd returns the position after the open tag, closing tag, comment,
// processing instruction, declaration or CDATA section at i, on a line that
// ends at end (CM 613 to 632). Whitespace in a tag has at most one line
// ending.
func (s *inlineParser) rawHTMLEnd(i, end uint32) (pos, bool) {
	line := s.src[i:end]
	switch {
	case bytes.HasPrefix(line, []byte("<!-->")):
		return pos{s.k, i + 5}, true
	case bytes.HasPrefix(line, []byte("<!--->")):
		return pos{s.k, i + 6}, true
	case bytes.HasPrefix(line, []byte("<!--")):
		return s.search(pos{s.k, i + 4}, closeComment)
	case bytes.HasPrefix(line, []byte("<?")):
		return s.search(pos{s.k, i + 2}, closeInstruction)
	case bytes.HasPrefix(line, []byte("<![CDATA[")):
		return s.search(pos{s.k, i + 9}, closeCDATA)
	case len(line) > 2 && line[1] == '!' && isASCIILetter(line[2]):
		return s.search(pos{s.k, i + 3}, closeDeclaration)
	case len(line) > 1 && line[1] == '/':
		p, ok := s.tagName(pos{s.k, i + 2})
		if !ok {
			return pos{}, false
		}
		p, _ = s.skipSpace(p)
		return s.expect(p, '>')
	}
	p, ok := s.tagName(pos{s.k, i + 1})
	if !ok {
		return pos{}, false
	}
	for {
		q, spaced := s.skipSpace(p)
		if c, ok := s.byteAt(q); !spaced || !ok || !isAttrNameStart(c) {
			p = q
			break
		}
		if p, ok = s.attribute(q); !ok {
			return pos{}, false
		}
	}
	if c, ok := s.byteAt(p); ok && c == '/' {
		p.i++
	}
	return s.expect(p, '>')
}

// tagName returns the position after the tag name at p: an ASCII letter,
// then ASCII letters, digits and '-'.
func (s *inlineParser) tagName(p pos) (pos, bool) {
	if c, ok := s.byteAt(p); !ok || !isASCIILetter(c) {
		return pos{}, false
	}
	end := s.lines[p.k].rest.end
	for p.i++; p.i < end && (isASCIIAlphanumeric(s.src[p.i]) || s.src[p.i] == '-'); p.i++ {
	}
	return p, true
}

// attribute returns the position after the attribute at p: a name, then
// optionally '=' and a value, with optional whitespace around the '='.
func (s *inlineParser) attribute(p pos) (pos, bool) {
	end := s.lines[p.k].rest.end
	for p.i++; p.i < end && (isAttrNameStart(s.src[p.i]) || isDecimalDigit(s.src[p.i]) || s.src[p.i] == '.' || s.src[p.i] == '-'); p.i++ {
	}
	q, _ := s.skipSpace(p)
	if c, ok := s.byteAt(q); !ok || c != '=' {
		return p, true
	}
	q, _ = s.skipSpace(pos{q.k, q.i + 1})
	c, ok := s.byteAt(q)
	switch {
	case !ok:
		return pos{}, false
	case c == '"':
		return s.search(pos{q.k, q.i + 1}, closeDoubleQuote)
	case c == '\'':
		return s.search(pos{q.k, q.i + 1}, closeSingleQuote)
	}
	end = s.lines[q.k].rest.end
	i := q.i
	for i < end && strings.IndexByte(" \t\v\f\"'=<>`", s.src[i]) < 0 {
		i++
	}
	if i == q.i {
		return pos{}, false
	}
	return pos{q.k, i}, true
}

// search returns the position after the first closer c that starts at or
// after p, on p's line or a later line. A failed search records where it
// started, and a later search stops there, so no two failed searches for one
// closer read the same bytes.
func (s *inlineParser) search(p pos, c int) (pos, bool) {
	closer := closers[c]
	limit := uint32(math.MaxUint32) // no closer starts at or after limit
	if s.failed[c] > 0 {
		limit = s.failed[c] - 1
	}
	for k := p.k; k < len(s.lines); k++ {
		l := s.lines[k].rest
		i := l.start
		if k == p.k {
			i = p.i
		}
		if i >= limit {
			break
		}
		end := l.end
		if limit < end {
			end = min(end, limit+count(len(closer))-1)
		}
		if j := bytes.Index(s.src[i:end], closer); j >= 0 {
			return pos{k, i + count(j+len(closer))}, true
		}
	}
	s.failed[c] = min(p.i, limit) + 1
	return pos{}, false
}

// AppendRawHTML appends the value of raw HTML id to dst: its [HTMLText] leaves,
// with a line feed for each line ending and a '|' for each cell pipe escape.
func (t *Tree) AppendRawHTML(dst []byte, id NodeID) []byte {
	for i := id + 1; i < NodeID(t.nodes[id].link); i++ {
		if k := t.nodes[i].kind; k == HTMLText || k == VerbatimLineEnding || k == CellPipeEscape {
			dst = t.AppendValue(dst, i)
		}
	}
	return dst
}
