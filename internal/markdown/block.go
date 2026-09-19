package markdown

import (
	"bytes"
	"math"
)

// blockParser runs the block phase: it reads the input line by line and
// appends the blocks to the builder.
type blockParser struct {
	b   *builder
	src []byte

	containers []container // open containers, innermost last; containers[0] is the document
	blocking   []uint32    // indices of the open block quotes and list items with no child
	footnotes  []uint32    // indices of the open footnote definitions
	leaf       leafBlock   // the open leaf block of the innermost container

	l      line         // the line being parsed
	pos    uint32       // start of the rest of the line: the first byte after the prefix leaves
	col    int          // column at the start of the byte at pos
	used   int          // columns of a tab at pos that structures consumed
	prefix []prefixLeaf // prefix leaves of the line that are not appended yet

	breakMemo uint32 // a thematic break scan from a marker of the line before this offset fails

	// The last run of spaces and tabs that indentation scanned: from spaceFrom
	// to spaceEnd, whose column is spaceCol.
	spaceFrom, spaceEnd uint32
	spaceCol            int

	inline inlineParser
	defs   *definitions
	pass1  bool // the block phase of pass 1, which skips the inline phase
	trace  func(line, int, int, bool)
	label  []byte // a normalized label

	pending []pendingLine // lines of the open leaf block that are not appended yet
	arena   []prefixLeaf  // prefix leaves of the pending lines

	aligns []Alignment    // the alignments of the columns of the open table
	cell   [1]pendingLine // the content of a table cell, for the inline phase
}

// container is an open container. It is 16 bytes: a line of n '>' opens n
// of them.
type container struct {
	kind  Kind
	blank bool // the last line that the container received was blank
	child bool // a block started in the container
	loose bool // a List is loose

	// content is true when a block other than a link reference definition
	// started in the container.
	content bool
	node    uint32
	indent  uint32 // a ListItem continues on this many columns of indentation
	marker  byte   // the marker character of a ListItem, or of the first item of a List
}

type leafKind uint8

const (
	noLeaf leafKind = iota
	paragraphLeaf
	indentedCodeLeaf
	fencedCodeLeaf
	htmlLeaf
	tableLeaf
)

type leafBlock struct {
	kind  leafKind
	blank bool      // the last line that the leaf block received was blank
	fence codeFence // the opening fence of fenced code
	html  uint8     // the kind of an HTML block
	tried bool      // a paragraph tried a table header

	// A table's columns, its rows with the header row, and the cells of its
	// rows up to the column count.
	columns, rows, cells int
}

type prefixLeaf struct {
	kind       Kind
	virt       uint8
	end, owner uint32
}

// pendingLine is a line of a paragraph, or a trailing blank line of indented
// code.
type pendingLine struct {
	rest        line   // the line after its prefix leaves
	col         uint8  // column at rest.start, modulo 4
	used        uint8  // columns of a tab at rest.start that structures consumed
	prefixFirst uint32 // index of the line's first prefix leaf in the arena
	prefixN     uint32
}

// parseLine adds one line to the tree.
func (p *blockParser) parseLine(l line) {
	p.l, p.pos, p.col, p.used, p.breakMemo = l, l.start, 0, 0, 0
	p.spaceFrom, p.spaceEnd = 1, 0

	matched, passed, notes := 1, 0, 0
	for matched < len(p.containers) {
		// Blank-line fast path: on an empty rest, each container before the
		// next block quote or list item with no child continues and consumes
		// nothing. A footnote definition continues on an empty rest only when
		// the whole line is empty.
		if p.pos == l.end {
			next := len(p.containers)
			if passed < len(p.blocking) {
				next = int(p.blocking[passed])
			}
			if l.start < l.end && notes < len(p.footnotes) {
				next = min(next, int(p.footnotes[notes]))
			}
			if matched < next {
				matched = next
				continue
			}
		}
		if !p.continues(p.containers[matched]) {
			break
		}
		if passed < len(p.blocking) && int(p.blocking[passed]) == matched {
			passed++
		}
		if notes < len(p.footnotes) && int(p.footnotes[notes]) == matched {
			notes++
		}
		matched++
	}
	allMatched := matched == len(p.containers)
	if allMatched && p.continueLeaf() {
		return
	}

	first, indent := p.indentation()
	if allMatched && p.leaf.kind == paragraphLeaf && indent < 4 && first < l.end {
		if end := setextUnderline(p.src, first, l.end); end > 0 {
			// Definitions come first (CM 215, 216).
			if p.commitDefinitions() {
				p.appendParagraph(Heading)
				p.appendPrefix()
				p.b.leafIf(Indent, first)
				p.b.leaf(SetextUnderline, end)
				p.b.leafIf(Whitespace, l.end)
				p.b.leafIf(LineEnding, l.eol)
				p.b.close()
				p.leaf = leafBlock{}
				return
			}
			// No paragraph line remains: the line is paragraph text, and no
			// other block starts on it, as in cmark.
			p.clearPending()
			p.addPending()
			return
		}
	}
	// cmark-gfm starts a footnote definition only when fewer than 99 blocks
	// started before it on the line (its MAX_LIST_DEPTH).
	opened, note := 0, false
starts:
	for indent < 4 && first < l.end {
		m, item := p.listItemStart(first, allMatched)
		switch labelEnd, contentStart := footnoteStart(p.src, first, l.end); {
		case p.src[first] == '>':
			p.startBlock(matched)
			p.startQuote(indent)
		case labelEnd > 0 && opened < 99:
			p.startBlock(matched)
			p.startFootnote(first, labelEnd, contentStart)
			note = true
		case item:
			p.startItem(m, indent, matched)
		default:
			break starts
		}
		opened++
		matched, allMatched = len(p.containers), true
		first, indent = p.indentation()
	}
	blank := first == l.end
	if !blank && p.startLeaf(first, indent, matched) {
		return
	}
	if !blank && allMatched && p.tableLine(first, indent) {
		return
	}

	if p.leaf.kind == paragraphLeaf && !blank {
		// A continuation line, which is lazy when a container did not match.
		if p.trace != nil {
			n := 0
			for _, c := range p.containers[1:matched] {
				if c.kind != List {
					n++
				}
			}
			p.trace(l, n, p.col+p.used, true)
		}
		p.addPending()
		return
	}
	if blank {
		p.closeUnmatched(matched)
		p.appendPrefix()
		p.receiveBlank()
		p.b.leafIf(BlankLine, l.eol)
		return
	}
	p.startParagraph(matched)
	p.leaf = leafBlock{kind: paragraphLeaf}
	if p.trace != nil && !note {
		p.trace(l, 0, p.col+p.used, false)
	}
	p.addPending()
}

// continues reports whether open container c continues on the rest of the
// line, and consumes its prefix.
func (p *blockParser) continues(c container) bool {
	switch c.kind {
	case BlockQuote:
		return p.continueQuote(c)
	case ListItem:
		return p.continueItem(c)
	case List:
		return true
	case FootnoteDefinition:
		return p.continueFootnote(c)
	}
	return false
}

// continueLeaf adds the line to the open leaf block when that block
// continues on it, and reports whether it did. A paragraph is left to the
// caller, which decides between block starts and continuation.
func (p *blockParser) continueLeaf() bool {
	l := p.l
	first, indent := p.indentation()
	switch p.leaf.kind {
	case fencedCodeLeaf:
		p.appendPrefix()
		if end, ok := p.leaf.fence.closes(p.src, first, l.end); ok && indent < 4 {
			p.b.leafIf(Indent, first)
			p.b.leaf(FenceMarker, end)
			p.b.leafIf(Whitespace, l.end)
			p.b.leafIf(LineEnding, l.eol)
			p.closeLeaf()
			return true
		}
		p.codeLine(p.rest(), p.col, p.used, p.leaf.fence.indent)
		return true
	case htmlLeaf:
		if first == l.end && p.leaf.html >= 6 {
			return false
		}
		p.leaf.blank = first == l.end
		p.appendPrefix()
		p.b.leafIf(HTMLText, l.end)
		p.b.leafIf(VerbatimLineEnding, l.eol)
		if p.leaf.html <= 5 && htmlBlockEnds(p.src, p.pos, l.end, p.leaf.html) {
			p.closeLeaf()
		}
		return true
	case indentedCodeLeaf:
		switch {
		case first == l.end:
			p.leaf.blank = true
			p.addPending()
			return true
		case indent >= 4:
			p.leaf.blank = false
			for _, pl := range p.pending {
				p.appendPendingPrefix(pl)
				p.codeLine(pl.rest, int(pl.col), int(pl.used), 4)
			}
			p.clearPending()
			p.appendPrefix()
			p.codeLine(p.rest(), p.col, p.used, 4)
			return true
		}
	case noLeaf, paragraphLeaf, tableLeaf:
	}
	return false
}

// startLeaf starts the leaf block that begins at first, after indent columns
// of indentation, and reports whether one started. matched is the number of
// open containers that the line matched.
func (p *blockParser) startLeaf(first uint32, indent, matched int) bool {
	l, para := p.l, p.leaf.kind == paragraphLeaf
	if indent >= 4 {
		// Indented code never starts while a paragraph is open, matched or
		// not.
		if para {
			return false
		}
		p.startBlock(matched)
		p.b.open(CodeBlock)
		p.leaf.kind = indentedCodeLeaf
		p.codeLine(p.rest(), p.col, p.used, 4)
		return true
	}
	if p.isThematicBreak(first) {
		p.startBlock(matched)
		p.b.open(ThematicBreak)
		p.b.leafIf(Indent, first)
		p.b.leaf(ThematicRun, l.end)
		p.b.leafIf(LineEnding, l.eol)
		p.b.close()
		return true
	}
	if h, ok := parseATXHeading(p.src, first, l.end); ok {
		p.startBlock(matched)
		p.b.open(Heading)
		p.b.leafIf(Indent, first)
		p.b.leaf(ATXMarker, h.markerEnd)
		p.b.leafIf(Whitespace, h.textStart)
		if p.pass1 {
			p.b.leafIf(Text, h.textEnd)
		} else {
			p.inline.inlines([]pendingLine{{rest: line{start: h.textStart, end: h.textEnd, eol: h.textEnd}}})
		}
		p.b.leafIf(Whitespace, h.closeStart)
		p.b.leafIf(ATXClose, h.closeEnd)
		p.b.leafIf(Whitespace, l.end)
		p.b.leafIf(LineEnding, l.eol)
		p.b.close()
		return true
	}
	if n := openingFence(p.src, first, l.end); n > 0 {
		p.startBlock(matched)
		p.b.open(CodeBlock)
		p.b.leafIf(Indent, first)
		p.b.leaf(FenceMarker, first+n)
		infoStart, infoEnd := first+n, l.end
		for infoEnd > infoStart && isInfoSpace(p.src[infoEnd-1]) {
			infoEnd--
		}
		for infoStart < infoEnd && isInfoSpace(p.src[infoStart]) {
			infoStart++
		}
		p.b.leafIf(Whitespace, infoStart)
		p.b.leafIf(InfoString, infoEnd)
		p.b.leafIf(Whitespace, l.end)
		p.b.leafIf(LineEnding, l.eol)
		p.leaf = leafBlock{kind: fencedCodeLeaf, fence: codeFence{char: p.src[first], length: n, indent: indent}}
		return true
	}
	// HTML block kind 7 never starts while a paragraph is open, matched or not
	// (testdata/dialect.md).
	if k := htmlBlockStart(p.src, first, l.end); k != 0 && (k < 7 || !para) {
		p.startBlock(matched)
		p.b.open(HTMLBlock)
		p.b.flag(k)
		p.b.leaf(HTMLText, l.end)
		p.b.leafIf(VerbatimLineEnding, l.eol)
		p.leaf = leafBlock{kind: htmlLeaf, html: k}
		if k <= 5 && htmlBlockEnds(p.src, first, l.end, k) {
			p.closeLeaf()
		}
		return true
	}
	return false
}

// startBlock prepares the start of a block that is not a list item after the
// first matched containers. It closes the other open blocks, and a matched
// list, because the block is not one of its items. It appends the line's
// prefix leaves and records the new child, whose content its container holds.
func (p *blockParser) startBlock(matched int) {
	p.startLeafBlock(matched)
	p.containers[len(p.containers)-1].content = true
}

// startParagraph prepares the start of a paragraph the same way, and records
// no content: every line of a paragraph can be a link reference definition,
// and appendLines records the content of the lines that remain.
func (p *blockParser) startParagraph(matched int) {
	p.startLeafBlock(matched)
}

func (p *blockParser) startLeafBlock(matched int) {
	if p.containers[matched-1].kind == List {
		matched--
	}
	p.closeUnmatched(matched)
	p.appendPrefix()
	p.addChild()
}

// closeUnmatched closes the open leaf block and every open container after
// the first n.
func (p *blockParser) closeUnmatched(n int) {
	p.closeLeaf()
	for i := len(p.containers) - 1; i >= n; i-- {
		c := p.containers[i]
		if c.loose {
			p.b.flag(1)
		}
		p.b.close()
		p.containers = p.containers[:i]
		if n := len(p.blocking); n > 0 && int(p.blocking[n-1]) == i {
			p.blocking = p.blocking[:n-1]
		}
		if n := len(p.footnotes); n > 0 && int(p.footnotes[n-1]) == i {
			p.footnotes = p.footnotes[:n-1]
		}
		p.orBlank(c.blank)
	}
}

// addChild records that a block starts in the innermost open container. A
// list item or list whose last line was blank makes its list loose.
func (p *blockParser) addChild() {
	i := len(p.containers) - 1
	c := &p.containers[i]
	if c.blank {
		switch c.kind {
		case ListItem:
			p.containers[i-1].loose = true
		case List:
			c.loose = true
		}
		c.blank = false
	}
	if c.kind == ListItem && !c.child {
		p.blocking = p.blocking[:len(p.blocking)-1]
	}
	c.child = true
}

// push opens container c. A block quote, and a list item, which has no child
// yet, go on the blocking stack of the blank-line fast path.
func (p *blockParser) push(c container) {
	switch c.kind {
	case BlockQuote, ListItem:
		p.blocking = append(p.blocking, count(len(p.containers)))
	case FootnoteDefinition:
		p.footnotes = append(p.footnotes, count(len(p.containers)))
	}
	p.containers = append(p.containers, c)
}

// isThematicBreak reports whether the rest of the line from first is a
// thematic break. A failed scan leaves a memo, so a later start on the line
// that the memo covers needs no scan.
func (p *blockParser) isThematicBreak(first uint32) bool {
	if first < p.breakMemo {
		return false
	}
	ok, memo := scanThematicBreak(p.src, first, p.l.end)
	p.breakMemo = max(p.breakMemo, memo)
	return ok
}

// receiveBlank records a blank line in the innermost open container. A block
// quote, and a list item on its own empty marker line, do not record one.
func (p *blockParser) receiveBlank() {
	if c := &p.containers[len(p.containers)-1]; c.kind != BlockQuote && (c.kind != ListItem || c.child) {
		c.blank = true
	}
}

// orBlank adds the blank line bit of a block that closed to its parent, the
// innermost open container, when that is a list or a list item.
func (p *blockParser) orBlank(blank bool) {
	if c := &p.containers[len(p.containers)-1]; c.kind == List || c.kind == ListItem {
		c.blank = c.blank || blank
	}
}

// closeLeaf closes the open leaf block, if any, and appends its pending
// lines.
func (p *blockParser) closeLeaf() {
	switch p.leaf.kind {
	case noLeaf:
	case paragraphLeaf:
		if p.commitDefinitions() {
			p.appendParagraph(Paragraph)
			p.b.close()
		}
		p.clearPending()
	case indentedCodeLeaf:
		// Trailing blank lines follow the code as structural blank lines.
		p.b.close()
		for _, pl := range p.pending {
			p.appendPendingPrefix(pl)
			p.b.split = splitVirt(int(pl.col), int(pl.used))
			p.b.leafIf(BlankLine, pl.rest.eol)
		}
		p.clearPending()
	case fencedCodeLeaf, htmlLeaf, tableLeaf:
		p.b.close()
	}
	p.orBlank(p.leaf.blank)
	p.leaf = leafBlock{}
}

// addPending adds the rest of the line to the pending lines, with the prefix
// leaves that are not appended yet.
func (p *blockParser) addPending() {
	p.pending = append(p.pending, pendingLine{
		rest:        p.rest(),
		col:         uint8(p.col & 3),
		used:        uint8(p.used & 3),
		prefixFirst: count(len(p.arena)),
		prefixN:     count(len(p.prefix)),
	})
	p.arena = append(p.arena, p.prefix...)
	p.prefix = p.prefix[:0]
	p.b.split = 0
}

// appendParagraph opens a block of kind k, Paragraph or Heading, appends the
// pending lines as its inline content and clears them.
func (p *blockParser) appendParagraph(k Kind) {
	p.appendLines(k, p.pending, false)
	p.clearPending()
}

// appendLines opens a block of kind k, Paragraph or Heading, and appends
// lines, pending lines, as its inline content, with the cell pipe rule when
// pipes is true. The prefix leaves of its first line come before its node.
func (p *blockParser) appendLines(k Kind, lines []pendingLine, pipes bool) {
	p.appendPendingPrefix(lines[0])
	p.b.open(k)
	last := lines[len(lines)-1].rest
	c := &p.containers[len(p.containers)-1]
	// Only the first block of a list item that is not a definition can hold a
	// task box, and these lines are that block's content.
	first := k == Paragraph && c.kind == ListItem && !c.content
	c.content = true
	if p.pass1 {
		p.b.leaf(Text, last.end)
	} else {
		p.inline.arena, p.inline.pipes, p.inline.task = p.arena, pipes, first
		p.inline.inlines(lines)
		if p.inline.box != 0 {
			p.b.flagOpen(p.containers[len(p.containers)-1].node, taskFlags(p.inline.box))
		}
	}
	p.b.leafIf(LineEnding, last.eol)
}

func (p *blockParser) appendPendingPrefix(pl pendingLine) {
	for _, x := range p.arena[pl.prefixFirst : pl.prefixFirst+pl.prefixN] {
		p.b.split = x.virt
		p.b.prefix(x.kind, x.end, x.owner)
	}
}

func (p *blockParser) clearPending() {
	p.pending, p.arena = p.pending[:0], p.arena[:0]
}

// appendPrefix appends the prefix leaves of the line that are not appended
// yet, and gives the builder the virt of the leaf that starts the rest.
func (p *blockParser) appendPrefix() {
	for _, x := range p.prefix {
		p.b.split = x.virt
		p.b.prefix(x.kind, x.end, x.owner)
	}
	p.prefix = p.prefix[:0]
	p.b.split = splitVirt(p.col, p.used)
}

func (p *blockParser) rest() line {
	return line{start: p.pos, end: p.l.end, eol: p.l.eol}
}

// consumeColumns consumes n columns of the spaces and tabs at the start of
// the rest of the line.
func (p *blockParser) consumeColumns(n int) {
	p.pos, p.col, p.used = skipColumns(p.src, p.pos, p.l.end, p.col, p.used, n)
}

// indentation returns the offset of the first byte in the rest of the line
// that is not a space or a tab, and the columns before it.
func (p *blockParser) indentation() (uint32, int) {
	if p.pos < p.spaceFrom || p.pos > p.spaceEnd {
		i, col := p.pos, p.col
		for ; i < p.l.end && isSpaceOrTab(p.src[i]); i++ {
			col = nextColumn(p.src[i], col)
		}
		p.spaceFrom, p.spaceEnd, p.spaceCol = p.pos, i, col
	}
	return p.spaceEnd, p.spaceCol - p.col - p.used
}

// skipSpace returns the offset of the first byte in src[i:end] that is not a
// space or a tab, or end.
func (p *blockParser) skipSpace(i, end uint32) uint32 {
	for i < end && isSpaceOrTab(p.src[i]) {
		i++
	}
	return i
}

// count returns the length n of a slice as a uint32. The slice has a bounded
// number of entries per input byte, and inputSize keeps the input small.
func count(n int) uint32 {
	if n < 0 || n > math.MaxUint32 {
		panic("markdown: too many entries for uint32 indices")
	}
	return uint32(n)
}

// skipColumns returns the position after up to n columns of the spaces and
// tabs at src[i:end]: its offset, the column at that offset, and the columns
// of a tab there that are consumed. The byte at i is at column col, and used
// columns of a tab there are consumed already. A tab that the n columns
// consume only in part stays at the returned offset.
func skipColumns(src []byte, i, end uint32, col, used, n int) (uint32, int, int) {
	for n > 0 && i < end && isSpaceOrTab(src[i]) {
		next := nextColumn(src[i], col)
		left := next - col - used
		if left > n {
			return i, col, used + n
		}
		n -= left
		i, col, used = i+1, next, 0
	}
	return i, col, used
}

// splitVirt returns the virt of a leaf that starts at a byte at column col
// with used columns of it consumed: the columns left of that split tab, or 0.
func splitVirt(col, used int) uint8 {
	if used == 0 {
		return 0
	}
	return uint8((4 - col%4 - used) & 3)
}

// nextColumn returns the column after byte c at column col. A tab advances
// to the next multiple of 4.
func nextColumn(c byte, col int) int {
	if c == '\t' {
		return col + 4 - col%4
	}
	return col + 1
}

// InterruptsParagraph reports whether line, the content of a paragraph line
// after its container prefixes and without indentation, would start a block,
// a setext underline or a table delimiter row there. On a lazy line a list
// item of any start or content starts a list, and no underline or delimiter
// row is read.
func InterruptsParagraph(line []byte, lazy bool) bool {
	end := count(len(line))
	if end == 0 {
		return false
	}
	if ok, _ := scanThematicBreak(line, 0, end); ok || line[0] == '>' || openingFence(line, 0, end) > 0 {
		return true
	}
	if _, ok := parseATXHeading(line, 0, end); ok {
		return true
	}
	if k := htmlBlockStart(line, 0, end); 1 <= k && k <= 6 {
		return true
	}
	if labelEnd, _ := footnoteStart(line, 0, end); labelEnd > 0 {
		return true
	}
	if m, ok := parseListMarker(line, 0, end); ok {
		empty := trimSpaceRight(line, m.end, end) == m.end
		if lazy || !empty && (!m.ordered || m.start == 1) {
			return true
		}
	}
	if lazy {
		return false
	}
	_, row := appendDelimiterRow(nil, line, 0, end)
	return row || setextUnderline(line, 0, end) > 0
}

// StartsBlock reports whether line, without indentation, would start a block
// outside a paragraph: a block that interrupts a paragraph on a lazy line, or
// an HTML block of kind 7.
func StartsBlock(line []byte) bool {
	return InterruptsParagraph(line, true) || htmlBlockStart(line, 0, count(len(line))) == 7
}

// A Layout follows a walk over the nodes of a tree in order. It gives the
// columns of each leaf, and the containers that each line matched.
type Layout struct {
	w containerWalk
}

// Layout returns a layout before the first node of t.
func (t *Tree) Layout() Layout {
	return Layout{w: containerWalk{t: t}}
}

// Visit moves l to node id, the node after the last one it visited. For a
// leaf it returns the column where the leaf's own columns start, after the
// columns of a split tab that structures consumed, and the column after the
// leaf.
func (l *Layout) Visit(id NodeID) (start, end int) {
	return l.w.visit(uint32(id))
}

// ItemIndent returns the columns that the list item that [Layout.Visit]
// entered last continues on: its indentation, marker and padding.
func (l *Layout) ItemIndent() int {
	return l.w.chain[len(l.w.chain)-1].indent
}

// ItemIndentOf returns the columns that the open list item id continues on,
// counted from its own first column: its indentation, marker and padding.
// The item must be one that [Layout.Visit] entered and has not left.
func (l *Layout) ItemIndentOf(id NodeID) int {
	for _, c := range l.w.chain {
		if c.id == uint32(id) {
			return c.indent
		}
	}
	return 0
}

// Matched returns how many of the open block quotes, list items and footnote
// definitions the line whose first leaf is node id matched, and the column
// where the prefixes of the line's containers end. Call it before visiting
// id. A lazy line matches fewer than are open.
func (l *Layout) Matched(id NodeID) (matched, end int) {
	return l.w.matched(uint32(id))
}

// containerWalk follows a walk over the nodes of a tree in order. It keeps the
// open block quotes, list items and footnote definitions with the columns that
// each continues on, and the column after the last leaf, so that it can count
// the containers that a line matched.
type containerWalk struct {
	t     *Tree
	col   int
	chain []openContainer // outermost first
}

type openContainer struct {
	id     uint32
	indent int // the columns that a list item or a footnote definition continues on
}

// visit moves the walk to node i, the node after the last one it visited,
// and returns the columns of a leaf, as Tree.leafColumns gives them.
func (w *containerWalk) visit(i uint32) (start, end int) {
	w.pop(i)
	t := w.t
	n := t.nodes[i]
	switch n.kind {
	case BlockQuote:
		w.chain = append(w.chain, openContainer{id: i})
	case ListItem:
		w.chain = append(w.chain, openContainer{id: i, indent: t.itemIndent(NodeID(i), w.col)})
	case FootnoteDefinition:
		w.chain = append(w.chain, openContainer{id: i, indent: 4})
	}
	if n.kind.class() == classStructure {
		return 0, 0
	}
	start, end = t.leafColumns(n, w.col)
	w.col = end
	if c := t.src[n.end-1]; c == '\n' || c == '\r' {
		w.col = 0
	}
	return start, end
}

// pop closes the containers that end before node i.
func (w *containerWalk) pop(i uint32) {
	for len(w.chain) > 0 && w.t.nodes[w.chain[len(w.chain)-1].id].link <= i {
		w.chain = w.chain[:len(w.chain)-1]
	}
}

// matched returns how many open containers the line whose first leaf is node
// i matched: the owners of its prefix leaves and the containers before them,
// then the list items and footnote definitions whose indentation ends inside
// the split tab of the first leaf that is not a prefix leaf. It also returns
// the column where the prefixes of the line's containers end, with the
// containers that start on the line.
func (w *containerWalk) matched(i uint32) (int, int) {
	w.pop(i)
	t, chain := w.t, w.chain
	k, col, partial := 0, 0, 0 // partial: columns of that tab that the last prefix leaf's container consumed
	for ; int(i) < len(t.nodes); i++ {
		n := t.nodes[i]
		if n.kind.class() == classStructure {
			continue
		}
		if _, prefix := n.kind.owner(); !prefix {
			if n.virt == 0 {
				return k, col
			}
			consumed := nextColumn('\t', col) - col - int(n.virt)
			end := col + max(min(partial, consumed), 0)
			consumed -= partial
			for k < len(chain) && t.nodes[chain[k].id].kind != BlockQuote && chain[k].indent <= consumed {
				consumed -= chain[k].indent
				end += chain[k].indent
				k++
			}
			return k, end
		}
		j, indent := k, 0
		for j < len(chain) && chain[j].id != n.link {
			j++
		}
		switch {
		case j < len(chain):
			k, indent = j+1, chain[j].indent
		case t.nodes[n.link].kind == ListItem:
			// A list item that starts on the line.
			indent = t.itemIndent(NodeID(n.link), col)
		}
		start, end := t.leafColumns(n, col)
		partial = indent - (end - start)
		if n.kind == QuoteMarker {
			// The optional space after '>' takes a column of a tab.
			partial = 0
			if t.src[n.end-1] == '>' {
				partial = 1
			}
		}
		col = end
	}
	return k, col
}

// itemIndent returns the columns that list item id continues on, when its
// ListMarker leaf is at column col: its indentation, marker and padding.
func (t *Tree) itemIndent(id NodeID, col int) int {
	marker := t.nodes[id+1]
	start, end := t.leafColumns(marker, col)
	next := end
	for i := int(id) + 2; i < len(t.nodes); i++ {
		if n := t.nodes[i]; n.kind.class() != classStructure {
			next, _ = t.leafColumns(n, end)
			break
		}
	}
	// After a marker with a blank rest, the padding is 1 column that no leaf
	// holds.
	if c := t.src[marker.end-1]; next == end && c != ' ' && c != '\t' {
		next++
	}
	return next - start
}

// leafColumns returns the columns of leaf n, whose first byte is at column
// col: the column where its own columns start, after the columns of a split
// tab that structures consumed, and the column after it.
func (t *Tree) leafColumns(n Node, col int) (start, end int) {
	b := t.src[n.start:n.end]
	start, end = col, col+len(b)
	if n.virt > 0 {
		start = nextColumn('\t', col) - int(n.virt)
	}
	if bytes.IndexByte(b, '\t') >= 0 {
		end = col
		for _, c := range b {
			end = nextColumn(c, end)
		}
	}
	return start, end
}
