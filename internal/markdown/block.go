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
	first, col := p.indent(l.start, l.end, 0)
	if first == l.end {
		p.closeParagraph()
		p.b.leafIf(BlankLine, l.eol)
		return
	}
	if col < 4 {
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
		first, _ := p.indent(l.start, l.end, 0)
		p.b.leafIf(Indent, first)
		p.b.leaf(Text, l.end)
		p.b.leafIf(LineEnding, l.eol)
	}
	p.b.close()
	p.para = p.para[:0]
}

// indent returns the offset of the first byte in src[i:end] that is not a
// space or a tab, or end, and the column of that offset when i is at column
// col. A tab advances to the next multiple of 4.
func (p *blockParser) indent(i, end uint32, col int) (uint32, int) {
	for ; i < end; i++ {
		switch p.src[i] {
		case ' ':
			col++
		case '\t':
			col += 4 - col%4
		default:
			return i, col
		}
	}
	return i, col
}
