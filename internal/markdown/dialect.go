package markdown

import (
	"bytes"
	"cmp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
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
	// rowFlanking: GitHub reads only Unicode P and ASCII punctuation as
	// punctuation for flanking.
	rowFlanking
	// rowCodeSpan: cmark-gfm misses a closing backtick run that its search
	// after an unmatched run recorded.
	rowCodeSpan
	// rowDestinationVTFF: GitHub keeps VT and FF in a destination.
	rowDestinationVTFF
	// rowLabelVTFF: GitHub keeps VT and FF in a link label.
	rowLabelVTFF
	// rowLabelLength: GitHub reads at most 1000 bytes as a label, and markfmt
	// at most 999 characters.
	rowLabelLength
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
	f := spanFinder{t: t, lineStart: true, newLine: true, vtff: bytes.ContainsAny(t.src, "\v\f")}
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
	vtff      bool // the input has VT or FF
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
		f.inlineBlock(id, n)
	case LinkReferenceDefinition:
		f.inlineBlock(id, n)
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

// inlineBlock runs the predicates that read the bytes of block id, a block
// with inline content or a link reference definition.
func (f *spanFinder) inlineBlock(id uint32, n Node) {
	b := f.t.src[n.start:n.end]
	if n.kind != LinkReferenceDefinition {
		if flankingDiffers(b) {
			f.add(id, rowFlanking)
		}
		if f.t.codeSpanAfterText(id) {
			f.add(id, rowCodeSpan)
		}
	}
	// A definition with VT or FF can resolve a reference in one reading only,
	// so every block that can hold a reference is a span.
	if f.vtff && bytes.ContainsAny(b, "[]\v\f") {
		f.add(id, rowDestinationVTFF)
		f.add(id, rowLabelVTFF)
	}
	if hasLongLabel(b) {
		f.add(id, rowLabelLength)
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

// flankingDiffers reports whether a run of '*' or '_' in b is next to a
// character that markfmt reads as punctuation for flanking and GitHub does
// not: a Unicode symbol that is neither ASCII nor punctuation, or U+FFFD,
// which NUL and invalid UTF-8 also give (design 6.7).
func flankingDiffers(b []byte) bool {
	for i := 0; i < len(b); {
		c := b[i]
		if c != '*' && c != '_' {
			i++
			continue
		}
		j := i
		for j < len(b) && b[j] == c {
			j++
		}
		if r, _ := utf8.DecodeLastRune(b[:i]); i > 0 && symbolDiffers(r) {
			return true
		}
		if j < len(b) {
			if r, _ := decodeRune(b[j:]); symbolDiffers(r) {
				return true
			}
		}
		i = j
	}
	return false
}

func symbolDiffers(r rune) bool {
	return r == 0 || r >= utf8.RuneSelf && unicode.Is(unicode.S, r) && !unicode.Is(unicode.P, r)
}

// codeSpanAfterText reports whether block id has a code span after a Text
// leaf with a backtick.
func (t *Tree) codeSpanAfterText(id uint32) bool {
	text := false
	for i := id + 1; i < t.nodes[id].link; i++ {
		switch m := t.nodes[i]; m.kind {
		case Text:
			text = text || bytes.IndexByte(t.src[m.start:m.end], '`') >= 0
		case CodeSpan:
			if text {
				return true
			}
		}
	}
	return false
}

// hasLongLabel reports whether b has a bracket pair with at least 1000 bytes
// between its brackets.
func hasLongLabel(b []byte) bool {
	if bytes.IndexByte(b, ']') < 0 {
		return false
	}
	var open []int
	for i := 0; i < len(b); i++ {
		switch b[i] {
		case '\\':
			i++
		case '[':
			open = append(open, i)
		case ']':
			if n := len(open); n > 0 {
				if i-open[n-1]-1 >= 1000 {
					return true
				}
				open = open[:n-1]
			}
		}
	}
	return false
}
