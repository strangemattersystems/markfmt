package markdown

import (
	"bytes"
	"slices"
)

// codeFence is the opening fence of a fenced code block.
type codeFence struct {
	char   byte   // '`' or '~'
	length uint32 // 0 when no fenced code block is open
	indent int    // columns of indentation before the fence
}

// openingFence returns the length of the opening code fence at src[i:end], a
// non-blank line after its indentation, or 0. A backtick fence cannot have a
// backtick after it on its line.
func openingFence(src []byte, i, end uint32) uint32 {
	c := src[i]
	if c != '`' && c != '~' {
		return 0
	}
	j := i
	for j < end && src[j] == c {
		j++
	}
	if j-i < 3 || c == '`' && bytes.IndexByte(src[j:end], '`') >= 0 {
		return 0
	}
	return j - i
}

// closes returns the end of the closing fence of f at src[i:end], a line
// after its indentation, and whether there is one: a run of f's character as
// long as f or longer, with only spaces and tabs after it.
func (f codeFence) closes(src []byte, i, end uint32) (uint32, bool) {
	j := i
	for j < end && src[j] == f.char {
		j++
	}
	return j, j-i >= f.length && trimSpaceRight(src, j, end) == j
}

// codeLine appends rest, the rest of a line, as code: up to n columns of
// indentation as a CodeIndent leaf, the rest as CodeText, and the line
// ending. The byte at rest.start is at column col, with used columns of it
// consumed.
func (p *blockParser) codeLine(rest line, col, used, n int) {
	virt := splitVirt(col, used)
	i, col, used := skipColumns(p.src, rest.start, rest.end, col, used, n)
	if i > rest.start {
		p.b.split = virt
		p.b.leaf(CodeIndent, i)
	}
	if i < rest.end {
		p.b.split = splitVirt(col, used)
		p.b.leaf(CodeText, rest.end)
	}
	p.b.leafIf(VerbatimLineEnding, rest.eol)
}

// AppendCode appends the content of code block id to dst: its CodeText
// leaves, and a line feed for each VerbatimLineEnding and after a last
// content line without one.
func (t *Tree) AppendCode(dst []byte, id NodeID) []byte {
	return t.appendVerbatim(dst, id, CodeText)
}

// appendVerbatim appends the value of code or HTML block id to dst, as
// verbatimReader reads it.
func (t *Tree) appendVerbatim(dst []byte, id NodeID, text Kind) []byte {
	r := newVerbatimReader(t, id, text)
	for b := r.next(); b != nil; b = r.next() {
		dst = append(dst, b...)
	}
	return dst
}

var (
	spaces   = []byte("   ")
	lineFeed = []byte{'\n'}
)

// verbatimReader reads the value of a code or HTML block one piece at a time:
// its leaves of kind text, with virt spaces for a split tab, a line feed for
// each VerbatimLineEnding, and a line feed after a last content line without
// one (design 8.2). A fence line is not a content line.
type verbatimReader struct {
	t      *Tree
	i, end uint32 // the next leaf, and the end of the block
	text   Kind
	rest   []byte // the rest of a leaf after the spaces of its split tab
	final  bool   // a line feed is due after the last leaf
}

func newVerbatimReader(t *Tree, id NodeID, text Kind) verbatimReader {
	r := verbatimReader{t: t, i: uint32(id) + 1, end: t.nodes[id].link, text: text, final: true}
	leaves := t.nodes[r.i:r.end]
	for i, m := range slices.Backward(leaves) {
		if m.kind == FenceMarker {
			r.final = false
			break
		}
		if m.kind == LineEnding || m.kind == VerbatimLineEnding {
			r.final = i < len(leaves)-1
			break
		}
	}
	return r
}

func (r *verbatimReader) next() []byte {
	if len(r.rest) > 0 {
		b := r.rest
		r.rest = nil
		return b
	}
	for r.i < r.end {
		m := r.t.nodes[r.i]
		r.i++
		switch m.kind {
		case r.text:
			b := r.t.src[m.start:m.end:m.end]
			if m.virt == 0 {
				return b
			}
			// The columns left of a split tab are spaces.
			r.rest = b[1:]
			return spaces[:m.virt:m.virt]
		case VerbatimLineEnding:
			return lineFeed[:1:1]
		}
	}
	if r.final {
		r.final = false
		return lineFeed[:1:1]
	}
	return nil
}

// AppendInfo appends the info string of code block id to dst.
func (t *Tree) AppendInfo(dst []byte, id NodeID) []byte {
	return append(dst, t.infoString(id)...)
}

// infoString returns the source bytes of the info string of code block id,
// or nil when it has none.
func (t *Tree) infoString(id NodeID) []byte {
	for _, m := range t.nodes[id+1 : t.nodes[id].link] {
		switch m.kind {
		case Indent, FenceMarker, Whitespace:
		case InfoString:
			return t.src[m.start:m.end:m.end]
		default:
			return nil
		}
	}
	return nil
}
