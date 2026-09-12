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

// codeLine appends rest, the rest of a line at column col, as code: up to n
// columns of indentation as a CodeIndent leaf, the rest as CodeText, and the
// line ending.
func (p *blockParser) codeLine(rest line, col, n int) {
	i, c := rest.start, col
	for i < rest.end && c-col < n && isSpaceOrTab(p.src[i]) {
		c = nextColumn(p.src[i], c)
		i++
	}
	p.b.leafIf(CodeIndent, i)
	p.b.leafIf(CodeText, rest.end)
	p.b.leafIf(VerbatimLineEnding, rest.eol)
}

// AppendCode appends the content of code block id to dst: its CodeText
// leaves, and a line feed for each VerbatimLineEnding and after a last
// content line without one.
func (t *Tree) AppendCode(dst []byte, id NodeID) []byte {
	return t.appendVerbatim(dst, id, CodeText)
}

// appendVerbatim appends the value of code or HTML block id to dst: its text
// leaves of kind text, and a line feed for each VerbatimLineEnding and after
// a last content line without one. A fence line is not a content line.
func (t *Tree) appendVerbatim(dst []byte, id NodeID, text Kind) []byte {
	leaves := t.nodes[id+1 : t.nodes[id].link]
	for _, m := range leaves {
		switch m.kind {
		case text:
			dst = append(dst, t.src[m.start:m.end]...)
		case VerbatimLineEnding:
			dst = append(dst, '\n')
		}
	}
	for i, m := range slices.Backward(leaves) {
		switch m.kind {
		case FenceMarker:
			return dst
		case LineEnding, VerbatimLineEnding:
			if i == len(leaves)-1 {
				return dst
			}
			return append(dst, '\n')
		}
	}
	return append(dst, '\n')
}

// AppendInfo appends the info string of code block id to dst.
func (t *Tree) AppendInfo(dst []byte, id NodeID) []byte {
	for _, m := range t.nodes[id+1 : t.nodes[id].link] {
		switch m.kind {
		case Indent, FenceMarker, Whitespace:
		case InfoString:
			return append(dst, t.src[m.start:m.end]...)
		default:
			return dst
		}
	}
	return dst
}
