package markdown

type listMarker struct {
	end     uint32 // end of the marker bytes
	ordered bool
	char    byte // the bullet character, or the delimiter of an ordered marker
	start   int  // the number of an ordered marker
}

// parseListMarker returns the list item marker at src[i:end], a non-blank
// line after its indentation, and whether there is one: '-', '+' or '*', or 1
// to 9 digits and '.' or ')', then a space, a tab or the end of the line.
func parseListMarker(src []byte, i, end uint32) (listMarker, bool) {
	var m listMarker
	j := i
	switch c := src[i]; {
	case c == '-' || c == '+' || c == '*':
		m.char = c
		j++
	case '0' <= c && c <= '9':
		for j < end && j-i < 9 && '0' <= src[j] && src[j] <= '9' {
			m.start = m.start*10 + int(src[j]-'0')
			j++
		}
		if j == end || src[j] != '.' && src[j] != ')' {
			return m, false
		}
		m.ordered, m.char = true, src[j]
		j++
	default:
		return m, false
	}
	m.end = j
	return m, j == end || isSpaceOrTab(src[j])
}

// listItemStart returns the marker of the list item that starts at first,
// and whether one starts. allMatched reports whether the line matched every
// open container.
func (p *blockParser) listItemStart(first uint32, allMatched bool) (listMarker, bool) {
	end := p.l.end
	// A thematic break comes first. An item interrupts a paragraph that is a
	// child of the last matched container only when it starts at 1 and has
	// content (design 5.1).
	para := allMatched && p.leaf.kind == paragraphLeaf
	if p.isThematicBreak(first) {
		return listMarker{}, false
	}
	m, ok := parseListMarker(p.src, first, end)
	if ok && para && (m.ordered && m.start != 1 || p.skipSpace(m.end, end) == end) {
		ok = false
	}
	return m, ok
}

// startItem opens a list item with marker m after indent columns of
// indentation, and appends its ListMarker leaf: the indentation, the marker
// and its padding. matched is the number of open containers that the line
// matched. A list opens around the item, unless the last matched container
// is a list that m continues.
func (p *blockParser) startItem(m listMarker, indent, matched int) {
	last := p.containers[matched-1]
	// The character tells a bullet from a delimiter, so equal characters mean
	// the same kind of list.
	same := last.kind == List && last.marker == m.char
	if last.kind == List && !same {
		matched--
	}
	p.closeUnmatched(matched)
	p.appendPrefix()
	p.addChild()
	if !same {
		p.b.open(List)
		p.push(container{kind: List, node: p.b.top(), marker: m.char})
		p.addChild()
	}
	p.b.open(ListItem)
	node := p.b.top()
	virt := splitVirt(p.col, p.used)
	p.consumeColumns(indent)
	width := int(m.end - p.pos)
	p.pos, p.col = m.end, p.col+width
	// Padding of 5 or more columns, or a blank rest, is 1 column: the rest is
	// indented code or empty (design 5.5).
	padding := 1
	if first, n := p.indentation(); n < 5 && first < p.l.end {
		padding = max(n, 1)
	}
	p.consumeColumns(padding)
	p.push(container{kind: ListItem, node: node, marker: m.char, indent: count(indent + width + padding)})
	p.b.split = virt
	p.b.prefix(ListMarker, p.pos, node)
}

// continueItem reports whether list item c continues on the rest of the line:
// on its content indentation, or on a blank rest when it has a child (design
// 5.1). The bytes it consumes are one ItemIndent leaf.
func (p *blockParser) continueItem(c container) bool {
	first, indent := p.indentation()
	switch {
	case indent >= int(c.indent):
		indent = int(c.indent)
	case first == p.l.end && c.child:
	default:
		return false
	}
	start, virt := p.pos, splitVirt(p.col, p.used)
	p.consumeColumns(indent)
	if p.pos > start {
		p.prefix = append(p.prefix, prefixLeaf{kind: ItemIndent, virt: virt, end: p.pos, owner: c.node})
	}
	return true
}

// ListStart returns the start number of list id and whether the list is
// ordered. A bullet list gives 0 and false.
func (t *Tree) ListStart(id NodeID) (int, bool) {
	marker := t.nodes[id+2] // the ListMarker leaf of the first item
	i := marker.start
	for isSpaceOrTab(t.src[i]) {
		i++
	}
	m, _ := parseListMarker(t.src, i, marker.end)
	return m.start, m.ordered
}

// ListLoose reports whether list id is loose.
func (t *Tree) ListLoose(id NodeID) bool {
	return t.nodes[id].flags == 1
}
