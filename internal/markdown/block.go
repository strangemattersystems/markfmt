package markdown

// blockParser runs the block phase: it reads the input line by line and
// appends the blocks to the builder (design 5).
type blockParser struct {
	b      *builder
	src    []byte
	para   []line // lines of the open paragraph, pending until it closes
	code   bool   // an indented code block is open
	blanks []line // trailing blank lines of the open indented code, pending
}

// line adds one line to the tree.
func (p *blockParser) line(l line) {
	first, col := p.indent(l.start, l.end, 0)
	blank := first == l.end
	if p.code {
		switch {
		case blank:
			p.blanks = append(p.blanks, l)
			return
		case col >= 4:
			for _, b := range p.blanks {
				p.codeLine(b)
			}
			p.blanks = p.blanks[:0]
			p.codeLine(l)
			return
		}
		p.closeCode()
	}
	if blank {
		p.closeParagraph()
		p.b.leafIf(BlankLine, l.eol)
		return
	}
	if col >= 4 {
		if len(p.para) == 0 {
			p.b.open(CodeBlock)
			p.code = true
			p.codeLine(l)
			return
		}
		p.para = append(p.para, l)
		return
	}
	if isThematicBreak(p.src, first, l.end) {
		p.closeParagraph()
		p.b.open(ThematicBreak)
		p.b.leafIf(Indent, first)
		p.b.leaf(ThematicRun, l.end)
		p.b.leafIf(LineEnding, l.eol)
		p.b.close()
		return
	}
	if h, ok := parseATXHeading(p.src, first, l.end); ok {
		p.closeParagraph()
		p.b.open(Heading)
		p.b.leafIf(Indent, first)
		p.b.leaf(ATXMarker, h.markerEnd)
		p.b.leafIf(Whitespace, h.textStart)
		p.b.leafIf(Text, h.textEnd)
		p.b.leafIf(Whitespace, h.closeStart)
		p.b.leafIf(ATXClose, h.closeEnd)
		p.b.leafIf(Whitespace, l.end)
		p.b.leafIf(LineEnding, l.eol)
		p.b.close()
		return
	}
	p.para = append(p.para, l)
}

// closeBlocks closes every open block at the end of the input.
func (p *blockParser) closeBlocks() {
	p.closeParagraph()
	p.closeCode()
}

// closeParagraph appends the pending paragraph, if one is open.
func (p *blockParser) closeParagraph() {
	if len(p.para) == 0 {
		return
	}
	p.b.open(Paragraph)
	for _, l := range p.para {
		first, _ := p.indent(l.start, l.end, 0)
		p.b.leafIf(Indent, first)
		p.b.leaf(Text, l.end)
		p.b.leafIf(LineEnding, l.eol)
	}
	p.b.close()
	p.para = p.para[:0]
}

// closeCode closes the open indented code block, if one is open. Its pending
// blank lines follow it as structural blank lines.
func (p *blockParser) closeCode() {
	if !p.code {
		return
	}
	p.b.close()
	p.code = false
	for _, l := range p.blanks {
		p.b.leafIf(BlankLine, l.eol)
	}
	p.blanks = p.blanks[:0]
}

// indent returns the offset of the first byte in src[i:end] that is not a
// space or a tab, or end, and the column of that offset when i is at column
// col.
func (p *blockParser) indent(i, end uint32, col int) (uint32, int) {
	for ; i < end && isSpaceOrTab(p.src[i]); i++ {
		col = nextColumn(p.src[i], col)
	}
	return i, col
}

// nextColumn returns the column after c, a space or a tab, at column col. A
// tab advances to the next multiple of 4.
func nextColumn(c byte, col int) int {
	if c == '\t' {
		return col + 4 - col%4
	}
	return col + 1
}
