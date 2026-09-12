package markdown

// quoteMarkerEnd returns the end of the block quote marker whose '>' is at
// src[i]: the '>' and an optional space.
func quoteMarkerEnd(src []byte, i, end uint32) uint32 {
	i++
	if i < end && src[i] == ' ' {
		i++
	}
	return i
}

// continueQuote reports whether block quote c continues on the rest of the
// line: up to 3 columns of indentation, '>' and an optional space. Its prefix
// is one QuoteMarker leaf.
func (p *blockParser) continueQuote(c container) bool {
	first, indent := p.indentation()
	if indent >= 4 || first == p.l.end || p.src[first] != '>' {
		return false
	}
	end := quoteMarkerEnd(p.src, first, p.l.end)
	p.prefix = append(p.prefix, prefixLeaf{kind: QuoteMarker, end: end, owner: c.node})
	p.consume(end)
	return true
}

// startQuote opens a block quote whose '>' is at first, and appends its
// marker.
func (p *blockParser) startQuote(first uint32) {
	p.b.open(BlockQuote)
	node := p.b.top()
	p.containers = append(p.containers, container{kind: BlockQuote, node: node})
	end := quoteMarkerEnd(p.src, first, p.l.end)
	p.b.prefix(QuoteMarker, end, node)
	p.consume(end)
}
