package markdown

// blockParser runs the block phase: it reads the input line by line and
// appends the blocks to the builder (design 5).
type blockParser struct {
	b    *builder
	src  []byte
	para []line // lines of the open paragraph, pending until it closes
}

// line adds one line to the tree.
func (p *blockParser) line(l line) {
	if p.skipSpace(l.start, l.end) == l.end {
		p.closeParagraph()
		p.b.leafIf(BlankLine, l.eol)
		return
	}
	p.para = append(p.para, l)
}

// closeParagraph appends the pending paragraph, if one is open.
func (p *blockParser) closeParagraph() {
	if len(p.para) == 0 {
		return
	}
	p.b.open(Paragraph)
	for _, l := range p.para {
		p.b.leafIf(Indent, p.skipSpace(l.start, l.end))
		p.b.leaf(Text, l.end)
		p.b.leafIf(LineEnding, l.eol)
	}
	p.b.close()
	p.para = p.para[:0]
}

// skipSpace returns the offset of the first byte in src[i:end] that is not a
// space or a tab, or end.
func (p *blockParser) skipSpace(i, end uint32) uint32 {
	for i < end && (p.src[i] == ' ' || p.src[i] == '\t') {
		i++
	}
	return i
}
