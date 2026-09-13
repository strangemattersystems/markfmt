package markdown

import "math"

// blockParser runs the block phase: it reads the input line by line and
// appends the blocks to the builder (design 5).
type blockParser struct {
	b   *builder
	src []byte

	containers []container // open containers, innermost last; containers[0] is the document
	blocking   []uint32    // indices of the open block quotes and list items with no child
	leaf       leafBlock   // the open leaf block of the innermost container

	l      line         // the line being parsed
	pos    uint32       // start of the rest of the line: the first byte after the prefix leaves
	col    int          // column at the start of the byte at pos
	used   int          // columns of a tab at pos that structures consumed (design 4.3)
	prefix []prefixLeaf // prefix leaves of the line that are not appended yet

	// interrupt blocks the starts that cannot interrupt a paragraph, on a
	// setext underline after definitions that took every paragraph line.
	interrupt bool

	breakMemo uint32 // a thematic break scan from a marker of the line before this offset fails

	pending []pendingLine // lines of the open leaf block that are not appended yet (design 5.3)
	arena   []prefixLeaf  // prefix leaves of the pending lines
}

// container is an open container. It is 16 bytes: a line of n '>' opens n
// of them.
type container struct {
	kind   Kind
	blank  bool // the last line that the container received was blank (design 5.6)
	child  bool // a block started in the container
	loose  bool // a List is loose
	node   uint32
	indent uint32 // a ListItem continues on this many columns of indentation
	marker byte   // the marker character of a ListItem, or of the first item of a List
}

type leafKind uint8

const (
	noLeaf leafKind = iota
	paragraphLeaf
	indentedCodeLeaf
	fencedCodeLeaf
	htmlLeaf
)

type leafBlock struct {
	kind  leafKind
	blank bool      // the last line that the leaf block received was blank
	fence codeFence // the opening fence of fenced code
	html  uint8     // the kind of an HTML block
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

// parseLine adds one line to the tree (design 5.1 and 5.2).
func (p *blockParser) parseLine(l line) {
	p.l, p.pos, p.col, p.used, p.interrupt, p.breakMemo = l, l.start, 0, 0, false, 0

	matched, passed := 1, 0
	for matched < len(p.containers) {
		// Blank-line fast path (design 5.1): on an empty rest, each container
		// before the next block quote or list item with no child continues
		// and consumes nothing.
		if p.pos == l.end {
			next := len(p.containers)
			if passed < len(p.blocking) {
				next = int(p.blocking[passed])
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
			// No paragraph line remains: dispatch the line again as if an
			// empty paragraph were open (design 5.4).
			p.clearPending()
			p.leaf = leafBlock{}
			p.interrupt = true
		}
	}
starts:
	for indent < 4 && first < l.end {
		switch m, item := p.listItemStart(first, allMatched); {
		case p.src[first] == '>':
			p.startBlock(matched)
			p.startQuote(indent)
		case item:
			p.startItem(m, indent, matched)
		default:
			break starts
		}
		matched, allMatched = len(p.containers), true
		first, indent = p.indentation()
	}
	blank := first == l.end
	if !blank && p.startLeaf(first, indent, matched) {
		return
	}

	if p.leaf.kind == paragraphLeaf && !blank {
		// A continuation line, which is lazy when a container did not match.
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
	p.startBlock(matched)
	p.leaf.kind = paragraphLeaf
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
	case noLeaf, paragraphLeaf:
	}
	return false
}

// startLeaf starts the leaf block that begins at first, after indent columns
// of indentation, and reports whether one started. matched is the number of
// open containers that the line matched.
func (p *blockParser) startLeaf(first uint32, indent, matched int) bool {
	l, para := p.l, p.leaf.kind == paragraphLeaf || p.interrupt
	if indent >= 4 {
		// Indented code never starts while a paragraph is open, matched or
		// not (design 5.1).
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
		p.b.leafIf(Text, h.textEnd)
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
		infoEnd := trimSpaceRight(p.src, first+n, l.end)
		p.b.leafIf(Whitespace, p.skipSpace(first+n, infoEnd))
		p.b.leafIf(InfoString, infoEnd)
		p.b.leafIf(Whitespace, l.end)
		p.b.leafIf(LineEnding, l.eol)
		p.leaf = leafBlock{kind: fencedCodeLeaf, fence: codeFence{char: p.src[first], length: n, indent: indent}}
		return true
	}
	// HTML block kind 7 never starts while a paragraph is open, matched or not
	// (design 5.1, testdata/dialect.md).
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
// list, because the block is not one of its items (design 5.5). It appends
// the line's prefix leaves and records the new child.
func (p *blockParser) startBlock(matched int) {
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
		p.orBlank(c.blank)
	}
}

// addChild records that a block starts in the innermost open container. A
// list item or list whose last line was blank makes its list loose (design
// 5.6).
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
	if c.kind == BlockQuote || c.kind == ListItem {
		p.blocking = append(p.blocking, count(len(p.containers)))
	}
	p.containers = append(p.containers, c)
}

// isThematicBreak reports whether the rest of the line from first is a
// thematic break. A failed scan leaves a memo, so a later start on the line
// that the memo covers needs no scan (design 5.1).
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
	case fencedCodeLeaf, htmlLeaf:
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
// pending lines as its lines and clears them. The prefix leaves of its first
// line come before its node.
func (p *blockParser) appendParagraph(k Kind) {
	for n, pl := range p.pending {
		p.appendPendingPrefix(pl)
		if n == 0 {
			p.b.open(k)
		}
		p.b.split = splitVirt(int(pl.col), int(pl.used))
		p.b.leafIf(Indent, p.skipSpace(pl.rest.start, pl.rest.end))
		p.b.leaf(Text, pl.rest.end)
		p.b.leafIf(LineEnding, pl.rest.eol)
	}
	p.clearPending()
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

// rest returns the rest of the line.
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
	i, col := p.pos, p.col
	for ; i < p.l.end && isSpaceOrTab(p.src[i]); i++ {
		col = nextColumn(p.src[i], col)
	}
	return i, col - p.col - p.used
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
// consume only in part stays at the returned offset (design 4.3).
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
