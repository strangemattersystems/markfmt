package format

import (
	"slices"

	"github.com/strangemattersystems/markfmt/internal/markdown"
)

// spanAt reports whether node id is a dialect span.
func (p *printer) spanAt(id markdown.NodeID) bool {
	for len(p.spans) > 0 && p.spans[0] < id {
		p.spans = p.spans[1:]
	}
	return len(p.spans) > 0 && p.spans[0] == id
}

// isSpan reports whether node id, which the walk has not passed, is a
// dialect span.
func (p *printer) isSpan(id markdown.NodeID) bool {
	_, ok := slices.BinarySearch(p.spans, id)
	return ok
}

// spanInside reports whether a dialect span is inside node id, which the walk
// has not passed.
func (p *printer) spanInside(id markdown.NodeID) bool {
	end, _ := p.tree.Next(id)
	i, _ := slices.BinarySearch(p.spans, id)
	return i < len(p.spans) && p.spans[i] < end
}
