package markdown

// continueQuote reports whether block quote c continues on the rest of the
// line: up to 3 columns of indentation, '>' and an optional space. Its prefix
// is one QuoteMarker leaf.
func (p *blockParser) continueQuote(c container) bool {
	first, indent := p.indentation()
	if indent >= 4 || first == p.l.end || p.src[first] != '>' {
		return false
	}
	virt := p.consumeQuoteMarker(indent)
	p.prefix = append(p.prefix, prefixLeaf{kind: QuoteMarker, virt: virt, end: p.pos, owner: c.node})
	return true
}

// startQuote opens a block quote whose '>' follows indent columns of
// indentation, and appends its marker.
func (p *blockParser) startQuote(indent int) {
	p.b.open(BlockQuote)
	node := p.b.top()
	p.containers = append(p.containers, container{kind: BlockQuote, node: node})
	p.b.split = p.consumeQuoteMarker(indent)
	p.b.prefix(QuoteMarker, p.pos, node)
}

// consumeQuoteMarker consumes indent columns of indentation, '>' and an
// optional space, and returns the virt of the marker's leaf. A tab that the
// optional space splits is not in the marker.
func (p *blockParser) consumeQuoteMarker(indent int) uint8 {
	virt := splitVirt(p.col, p.used)
	p.consumeColumns(indent)
	p.pos, p.col = p.pos+1, p.col+1
	p.consumeColumns(1)
	return virt
}
