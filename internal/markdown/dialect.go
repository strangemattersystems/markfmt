package markdown

import (
	"cmp"
	"slices"
	"strings"
)

// dialectRow is a row of testdata/dialect.md: a rule where GitHub and
// CommonMark 0.31.2 give a document a different meaning (design 2.1).
type dialectRow uint8

const (
	// rowSearch: GitHub's kind 6 tag list has no search, so on GitHub the
	// block is kind 7, which needs a whole tag and cannot interrupt a
	// paragraph.
	rowSearch dialectRow = iota
)

// dialectRows is a set of dialect rows.
type dialectRows uint32

// dialectSpan is a structure node that a dialect predicate matches, with its
// rows (design 10.4).
type dialectSpan struct {
	id   uint32
	rows dialectRows
}

// dialectSpans returns the dialect spans of t in node order.
func (t *Tree) dialectSpans() []dialectSpan {
	var spans []dialectSpan
	var stack []uint32
	// The last leaf that is not a prefix leaf, its parent and the parent of that.
	var last, parent, grandparent uint32
	for i, n := range t.nodes {
		id := uint32(i)
		for len(stack) > 0 && t.nodes[stack[len(stack)-1]].link <= id {
			stack = stack[:len(stack)-1]
		}
		if n.kind.class() != classStructure {
			if _, prefix := n.kind.owner(); !prefix {
				last, parent = id, stack[len(stack)-1]
				if len(stack) > 1 {
					grandparent = stack[len(stack)-2]
				}
			}
			continue
		}
		if n.kind == HTMLBlock && t.htmlBlockNamed(id, "search") {
			spans = append(spans, dialectSpan{id: id, rows: 1 << rowSearch})
			// GitHub can continue the paragraph or the table that the block's
			// first line closes, in any container.
			if t.nodes[last].kind == LineEnding {
				switch t.nodes[parent].kind {
				case Paragraph:
					spans = append(spans, dialectSpan{id: parent, rows: 1 << rowSearch})
				case TableRow:
					spans = append(spans, dialectSpan{id: grandparent, rows: 1 << rowSearch})
				}
			}
		}
		stack = append(stack, id)
	}
	slices.SortFunc(spans, func(a, b dialectSpan) int { return cmp.Compare(a.id, b.id) })
	merged := spans[:0]
	for _, s := range spans {
		if n := len(merged); n > 0 && merged[n-1].id == s.id {
			merged[n-1].rows |= s.rows
			continue
		}
		merged = append(merged, s)
	}
	return merged
}

// htmlBlockNamed reports whether HTML block id is of kind 6 and starts with
// the tag name, in any case.
func (t *Tree) htmlBlockNamed(id uint32, name string) bool {
	if t.nodes[id].flags != 6 {
		return false
	}
	m := t.nodes[id+1]
	j := m.start
	for j < m.end && isSpaceOrTab(t.src[j]) {
		j++
	}
	got, _ := htmlTagName(t.src[j:m.end])
	return strings.EqualFold(string(got), name)
}
