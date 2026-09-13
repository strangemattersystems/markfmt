package markdown

import (
	"bytes"
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
	// rowSource: GitHub's kind 6 tag list has source, so on GitHub a line
	// that starts with the tag starts a block, also in a paragraph.
	rowSource
	// rowDeclaration: GitHub starts a declaration only with an uppercase
	// letter after "<!".
	rowDeclaration
	// rowLazyKind7: GitHub starts HTML block kind 7 on a lazy line.
	rowLazyKind7
	// rowComment: GitHub reads the older comment grammar.
	rowComment
)

// dialectRows is a set of dialect rows.
type dialectRows uint32

// dialectSpan is a structure node that a dialect predicate matches, with its
// rows (design 10.4).
type dialectSpan struct {
	id   uint32
	rows dialectRows
}

// dialectSpans returns the dialect spans of t in node order. Each predicate
// is conservative: a span where GitHub gives the same meaning only keeps
// bytes that the printer could have changed.
func (t *Tree) dialectSpans() []dialectSpan {
	f := spanFinder{t: t, lineStart: true, newLine: true}
	f.walk.t = t
	for i := range t.nodes {
		f.node(uint32(i))
	}
	slices.SortFunc(f.spans, func(a, b dialectSpan) int { return cmp.Compare(a.id, b.id) })
	merged := f.spans[:0]
	for _, s := range f.spans {
		if n := len(merged); n > 0 && merged[n-1].id == s.id {
			merged[n-1].rows |= s.rows
			continue
		}
		merged = append(merged, s)
	}
	return merged
}

// spanFinder runs the dialect predicates over the nodes of a tree in order.
type spanFinder struct {
	t     *Tree
	spans []dialectSpan
	stack []uint32 // the open structure nodes

	// The last leaf that is not a prefix leaf, its parent and the parent of
	// that.
	last, parent, grandparent uint32

	block     uint32 // the last Paragraph, Heading or TableCell entered
	table     uint32 // the last Table entered
	lineStart bool   // only prefix and Indent leaves follow the last line ending
	newLine   bool   // no leaf follows the last line ending
	lineFirst uint32 // the first leaf of the line
	walk      containerWalk
}

func (f *spanFinder) add(id uint32, row dialectRow) {
	f.spans = append(f.spans, dialectSpan{id: id, rows: 1 << row})
}

// node runs the predicates at node id.
func (f *spanFinder) node(id uint32) {
	t := f.t
	for len(f.stack) > 0 && t.nodes[f.stack[len(f.stack)-1]].link <= id {
		f.stack = f.stack[:len(f.stack)-1]
	}
	n := t.nodes[id]
	switch _, prefix := n.kind.owner(); {
	case n.kind.class() == classStructure:
		f.enter(id, n)
		f.stack = append(f.stack, id)
	case prefix:
		f.leaf(id)
	default:
		f.leaf(id)
		if f.lineStart && n.kind != Indent {
			f.lineStart = false
			f.line(id, n)
		}
		f.last, f.parent = id, f.stack[len(f.stack)-1]
		if len(f.stack) > 1 {
			f.grandparent = f.stack[len(f.stack)-2]
		}
		if c := t.src[n.end-1]; c == '\n' || c == '\r' {
			f.lineStart, f.newLine = true, true
		}
	}
	f.walk.visit(id)
}

// leaf records leaf id as the first leaf of its line when no leaf came before
// it on the line.
func (f *spanFinder) leaf(id uint32) {
	if f.newLine {
		f.newLine, f.lineFirst = false, id
	}
}

func (f *spanFinder) enter(id uint32, n Node) {
	t := f.t
	switch n.kind {
	case Paragraph, Heading, TableCell:
		f.block = id
	case Table:
		f.table = id
	case HTMLBlock:
		switch s := t.htmlBlockLine(id); {
		case n.flags == 6 && tagNamed(s, "search"):
			f.addInterrupted(id, rowSearch)
		case n.flags == 4 && 'a' <= s[2] && s[2] <= 'z':
			f.addInterrupted(id, rowDeclaration)
		}
	case RawHTML:
		b := t.src[n.start:n.end]
		switch {
		case len(b) > 2 && b[1] == '!' && 'a' <= b[2] && b[2] <= 'z':
			f.add(f.block, rowDeclaration)
		case bytes.HasPrefix(b, []byte("<!--")) && !isGitHubComment(t.AppendRawHTML(nil, NodeID(id))):
			f.add(f.block, rowComment)
		}
	}
}

// addInterrupted adds HTML block id to row, with the paragraph or the table
// that the block's first line closes, in any container. GitHub can continue
// that block on the line.
func (f *spanFinder) addInterrupted(id uint32, row dialectRow) {
	f.add(id, row)
	if f.t.nodes[f.last].kind != LineEnding {
		return
	}
	switch f.t.nodes[f.parent].kind {
	case Paragraph:
		f.add(f.parent, row)
	case TableRow:
		f.add(f.grandparent, row)
	}
}

// line runs the predicates that read the start of a line at leaf id, the
// first leaf of the line after its prefix and Indent leaves.
func (f *spanFinder) line(id uint32, n Node) {
	t := f.t
	if t.src[n.start] != '<' {
		return
	}
	end := int(n.start)
	for end < len(t.src) && t.src[end] != '\n' && t.src[end] != '\r' {
		end++
	}
	var block uint32
	switch top, b := f.stack[len(f.stack)-1], t.nodes[f.block]; {
	case t.nodes[top].kind == LinkReferenceDefinition:
		block = top
	case b.link <= id || b.kind != Paragraph && b.kind != Heading && b.kind != TableCell:
		return
	case b.kind == TableCell:
		block = f.table
	default:
		block = f.block
	}
	if tagNamed(t.src[n.start:end], "source") {
		f.add(block, rowSource)
	}
	if t.nodes[block].kind != Table && htmlBlockStart(t.src, n.start, count(end)) == 7 &&
		f.walk.matched(f.lineFirst) < len(f.walk.chain) {
		f.add(block, rowLazyKind7)
	}
}

// htmlBlockLine returns the first line of HTML block id after its
// indentation.
func (t *Tree) htmlBlockLine(id uint32) []byte {
	m := t.nodes[id+1]
	j := m.start
	for j < m.end && isSpaceOrTab(t.src[j]) {
		j++
	}
	return t.src[j:m.end]
}

// tagNamed reports whether s starts with an HTML block kind 6 start of the
// tag name, in any case: "<" or "</", the name, then the end of s, whitespace,
// ">" or "/>".
func tagNamed(s []byte, name string) bool {
	got, k := htmlTagName(s)
	return len(s) > 1 && s[0] == '<' && strings.EqualFold(string(got), name) &&
		(k == len(s) || isTagSpace(s[k]) || s[k] == '>' || bytes.HasPrefix(s[k:], []byte("/>")))
}

// isGitHubComment reports whether v, a raw HTML comment, is a comment in the
// grammar that GitHub reads: its text does not start with ">" or "->", does
// not end with "-", and does not contain "--".
func isGitHubComment(v []byte) bool {
	if len(v) < len("<!---->") {
		return false
	}
	text := v[4 : len(v)-3]
	return !bytes.HasPrefix(text, []byte(">")) && !bytes.HasPrefix(text, []byte("->")) &&
		!bytes.HasSuffix(text, []byte("-")) && !bytes.Contains(text, []byte("--"))
}
