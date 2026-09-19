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
// CommonMark 0.31.2 give a document a different meaning.
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
	// rowDefinitionUnderline: commonmark.js reads an underline after
	// definitions alone as a thematic break. markfmt and GitHub read a
	// paragraph.
	rowDefinitionUnderline
	// rowFailedTitle: GitHub keeps a title that other characters follow as
	// the title of the definition.
	rowFailedTitle
	// rowInfoVTFF: GitHub keeps VT and FF at the ends of an info string.
	rowInfoVTFF
	// rowSplitParagraph: CommonMark 0.31.2, which has no tables, reads the
	// cell pipe escapes of a paragraph above a table as escapes.
	rowSplitParagraph
	// rowListItems: GitHub opens at most 99 blocks on a line, and markfmt
	// opens every list item.
	rowListItems
	// rowFootnoteCaret: GitHub garbles a footnote reference whose caret is an
	// escape or an entity reference.
	rowFootnoteCaret
	// rowFootnoteLineEnding: GitHub garbles a footnote reference label across
	// a line ending.
	rowFootnoteLineEnding
)

// dialectRows is a set of dialect rows.
type dialectRows uint32

// dialectSpan is a structure node that a dialect predicate matches, with its
// rows.
type dialectSpan struct {
	id   uint32
	rows dialectRows
}

// DialectSpans returns the structure nodes where GitHub and CommonMark 0.31.2
// give the document a different meaning, in node order. The printer prints
// each top-level block that holds one as written.
func (t *Tree) DialectSpans() []NodeID {
	ids := make([]NodeID, len(t.spans))
	for i, s := range t.spans {
		ids[i] = NodeID(s.id)
	}
	return ids
}

// findDialectSpans returns the dialect spans of t in node order. Each
// predicate is conservative: a span where GitHub gives the same meaning only
// keeps bytes that the printer could have changed.
func (t *Tree) findDialectSpans() []dialectSpan {
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

	opened, firstOpened uint32 // the containers opened on the line, and the first of them
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
			f.lineStart, f.newLine, f.opened = true, true, 0
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
		if n.kind != TableCell {
			f.afterDefinition(id)
		}
	case LinkReferenceDefinition:
		f.inlineBlock(id, n)
	case Table:
		f.table = id
		if t.nodes[f.last].kind == LineEnding && t.nodes[f.parent].kind == Paragraph {
			if p := t.nodes[f.parent]; bytes.Contains(t.src[p.start:p.end], []byte(`\|`)) {
				f.add(f.parent, rowSplitParagraph)
				f.add(id, rowSplitParagraph)
			}
		}
	case CodeBlock:
		i := id + 1
		if t.nodes[i].kind == Indent {
			i++
		}
		if fence := t.nodes[i]; fence.kind == FenceMarker {
			if info := t.src[fence.end:lineEnd(t.src, fence.end)]; bytes.ContainsAny(info, "\v\f") || bytes.Contains(info, []byte("&#")) {
				f.add(id, rowInfoVTFF)
			}
		}
	case BlockQuote, ListItem, FootnoteDefinition:
		if f.opened == 0 {
			f.firstOpened = id
		}
		if n.kind == ListItem && f.opened >= 99 {
			f.add(f.firstOpened, rowListItems)
		}
		f.opened++
	case FootnoteReference:
		// The Caret leaf holds an escape or an entity reference as written.
		if c := t.nodes[id+2]; c.end-c.start > 1 {
			f.add(f.block, rowFootnoteCaret)
		}
		for i := id + 1; i < n.link; i++ {
			if t.nodes[i].kind == VerbatimLineEnding {
				f.add(f.block, rowFootnoteLineEnding)
				break
			}
		}
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

// afterDefinition adds block id, a paragraph or a heading, and the link
// reference definition before it to a row, when no blank line is between
// them and the block starts with an underline or a title quote.
func (f *spanFinder) afterDefinition(id uint32) {
	t := f.t
	if t.nodes[f.last].kind != LineEnding || t.nodes[f.parent].kind != LinkReferenceDefinition {
		return
	}
	def, i, end := f.parent, id+1, t.nodes[id].link
	for i < end && (t.nodes[i].kind.class() == classStructure || t.nodes[i].kind == Indent) {
		i++
	}
	if i == end {
		return
	}
	var row dialectRow
	switch start := t.nodes[i].start; {
	case setextUnderline(t.src, start, lineEnd(t.src, start)) > 0:
		row = rowDefinitionUnderline
	case strings.IndexByte(`"'(`, t.src[start]) >= 0 && !slices.ContainsFunc(t.nodes[def:t.nodes[def].link], func(m Node) bool { return m.kind == TitleQuote }):
		row = rowFailedTitle
	default:
		return
	}
	f.add(def, row)
	f.add(id, row)
}

// addInterrupted adds HTML block id to row, with the paragraph or the table
// that the block's first line closes, in any container. GitHub can continue
// that block on the line, unless the line opens a container, which closes it
// in both readings.
func (f *spanFinder) addInterrupted(id uint32, row dialectRow) {
	f.add(id, row)
	if f.t.nodes[f.last].kind != LineEnding || f.opened > 0 {
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
// first leaf of the line after its prefix and [Indent] leaves.
func (f *spanFinder) line(id uint32, n Node) {
	t := f.t
	if t.src[n.start] != '<' {
		return
	}
	end := lineEnd(t.src, n.start)
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
	if t.nodes[block].kind != Table && htmlBlockStart(t.src, n.start, end) == 7 {
		if matched, _ := f.walk.matched(f.lineFirst); matched < len(f.walk.chain) {
			f.add(block, rowLazyKind7)
		}
	}
}

// lineEnd returns the offset of the first line ending at or after offset i,
// or the end of src.
func lineEnd(src []byte, i uint32) uint32 {
	if j := bytes.IndexAny(src[i:], "\n\r"); j >= 0 {
		return i + count(j)
	}
	return count(len(src))
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

// flankingDiffers reports whether a run of '*', '_' or '~' in b is next to a
// character that markfmt reads as punctuation for flanking and GitHub does
// not: a Unicode symbol that is neither ASCII nor punctuation, or U+FFFD,
// which NUL and invalid UTF-8 also give. cmark-gfm scans a strikethrough run
// with the same punctuation test as emphasis.
func flankingDiffers(b []byte) bool {
	for i := 0; i < len(b); {
		c := b[i]
		if c != '*' && c != '_' && c != '~' {
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
