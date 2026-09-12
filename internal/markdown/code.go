package markdown

// codeLine appends a line of indented code: up to 4 columns of indentation
// as a CodeIndent leaf, the rest as CodeText, and the line ending.
func (p *blockParser) codeLine(l line) {
	i, col := l.start, 0
	for i < l.end && col < 4 && isSpaceOrTab(p.src[i]) {
		col = nextColumn(p.src[i], col)
		i++
	}
	p.b.leafIf(CodeIndent, i)
	p.b.leafIf(CodeText, l.end)
	p.b.leafIf(VerbatimLineEnding, l.eol)
}

// AppendCode appends the content of code block id to dst: its CodeText
// leaves, and a line feed for each VerbatimLineEnding and after a last line
// without one.
func (t *Tree) AppendCode(dst []byte, id NodeID) []byte {
	n := t.nodes[id]
	for _, m := range t.nodes[id+1 : n.link] {
		switch m.kind {
		case CodeText:
			dst = append(dst, t.src[m.start:m.end]...)
		case VerbatimLineEnding:
			dst = append(dst, '\n')
		}
	}
	if t.nodes[n.link-1].kind != VerbatimLineEnding {
		dst = append(dst, '\n')
	}
	return dst
}
