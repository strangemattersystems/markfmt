package markdown

import (
	"bytes"
	"fmt"
	"slices"
)

// Equal reports the first difference between the event projections of a and
// b, or nil when they are equal (design 10). The projection of a tree has an
// Enter event with a key for each structure node, a Content event for each
// run of content leaves of one group, and an Exit event at the end of each
// structure node.
func Equal(a, b *Tree) error {
	c := comparer{a: a, b: b}
	pa, pb := newProjection(a), newProjection(b)
	for {
		ea, okA := pa.next()
		eb, okB := pb.next()
		for okA && okB && ea.op != eb.op {
			switch {
			case ea.op == exitEvent && pb.skipEmptyCell(eb, a.nodes[ea.id].kind):
				eb, okB = pb.next()
				continue
			case eb.op == exitEvent && pa.skipEmptyCell(ea, b.nodes[eb.id].kind):
				ea, okA = pa.next()
				continue
			}
			break
		}
		if !okA && !okB {
			return nil
		}
		if err := c.compare(ea, okA, eb, okB); err != nil {
			return err
		}
		if ea.op == enterEvent && ea.rows != 0 && !equalSpans(&pa, ea.id, &pb, eb.id) {
			return fmt.Errorf("markdown: different dialect spans: %s against %s", describe(a, ea, okA), describe(b, eb, okB))
		}
	}
}

type comparer struct {
	a, b           *Tree
	labelA, labelB []byte // normalized labels, bounded by the label cap

	// The readers of the values that a comparison reads. They are fields, so
	// that comparing a content run or a key allocates nothing.
	runA, runB   runReader
	infoA, infoB valueReader
	textA, textB verbatimReader
	codeA, codeB codeSpanReader
}

// compare reports the difference between event ea of a and event eb of b.
// ok is false after the last event of a tree.
func (c *comparer) compare(ea event, okA bool, eb event, okB bool) error {
	var what string
	switch {
	case !okA || !okB:
		what = "one tree has more events"
	case ea.op != eb.op:
		what = "different events"
	case ea.op == enterEvent && c.a.nodes[ea.id].kind != c.b.nodes[eb.id].kind:
		what = "different kinds"
	case ea.op == enterEvent && ea.rows != eb.rows:
		what = "different dialect rows"
	case ea.op == enterEvent && !c.equalKeys(ea.id, eb.id):
		what = "different keys"
	case ea.op == contentEvent && ea.group != eb.group:
		what = "different content groups"
	case ea.op == contentEvent && !c.equalRuns(ea, eb):
		what = "different content"
	default:
		return nil
	}
	return fmt.Errorf("markdown: %s: %s against %s", what, describe(c.a, ea, okA), describe(c.b, eb, okB))
}

// equalRuns reports whether the content runs of events ea and eb read the
// same bytes.
func (c *comparer) equalRuns(ea, eb event) bool {
	c.runA = runReader{t: c.a, i: uint32(ea.id), end: uint32(ea.end)}
	c.runB = runReader{t: c.b, i: uint32(eb.id), end: uint32(eb.end)}
	return equalPieces(&c.runA, &c.runB)
}

func describe(t *Tree, e event, ok bool) string {
	if !ok {
		return "the end"
	}
	n := t.nodes[e.id]
	return fmt.Sprintf("%v at byte %d", n.kind, n.start)
}

// equalKeys reports whether the structure nodes ia of a and ib of b, of one
// kind, have equal keys (design 10.2).
func (c *comparer) equalKeys(ia, ib NodeID) bool {
	a, b := c.a, c.b
	//exhaustive:enforce
	switch a.nodes[ia].kind {
	case ListItem:
		return a.nodes[ia].flags == b.nodes[ib].flags
	case Document, BlockQuote, Paragraph, ThematicBreak, SoftBreak, HardBreak, RawHTML, Emphasis, Strong, Strikethrough, TableRow:
		return true
	case Table:
		return a.TableColumns(ia) == b.TableColumns(ib)
	case TableCell:
		return a.nodes[ia].flags == b.nodes[ib].flags
	case FrontMatter:
		return a.FrontMatterTOML(ia) == b.FrontMatterTOML(ib)
	case List:
		startA, orderedA := a.ListStart(ia)
		startB, orderedB := b.ListStart(ib)
		return orderedA == orderedB && startA == startB && a.ListLoose(ia) == b.ListLoose(ib)
	case Heading:
		return a.HeadingLevel(ia) == b.HeadingLevel(ib)
	case CodeBlock:
		c.infoA, c.infoB = a.infoReader(ia), b.infoReader(ib)
		c.textA, c.textB = newVerbatimReader(a, ia, CodeText), newVerbatimReader(b, ib, CodeText)
		return equalPieces(&c.infoA, &c.infoB) && equalPieces(&c.textA, &c.textB)
	case HTMLBlock:
		c.textA, c.textB = newVerbatimReader(a, ia, HTMLText), newVerbatimReader(b, ib, HTMLText)
		return equalPieces(&c.textA, &c.textB)
	case Autolink:
		return a.AutolinkAngle(ia) == b.AutolinkAngle(ib) && a.AutolinkEmail(ia) == b.AutolinkEmail(ib)
	case Link, Image:
		form := a.LinkForm(ia)
		if form != b.LinkForm(ib) {
			return false
		}
		if form == InlineLink {
			return true
		}
		c.labelA, c.labelB = a.AppendLinkLabel(c.labelA[:0], ia), b.AppendLinkLabel(c.labelB[:0], ib)
		return bytes.Equal(c.labelA, c.labelB)
	case CodeSpan:
		c.codeA, c.codeB = newCodeSpanReader(a, ia), newCodeSpanReader(b, ib)
		return equalPieces(&c.codeA, &c.codeB)
	case LinkReferenceDefinition:
		c.labelA, c.labelB = a.AppendLabel(c.labelA[:0], ia), b.AppendLabel(c.labelB[:0], ib)
		return bytes.Equal(c.labelA, c.labelB)
	case FootnoteDefinition:
		return bytes.Equal(a.FootnoteDefinitionLabel(ia), b.FootnoteDefinitionLabel(ib))
	case FootnoteReference:
		resolved := a.FootnoteReferenceResolved(ia)
		if resolved != b.FootnoteReferenceResolved(ib) {
			return false
		}
		// An unresolved label is copied whole: O(label) memory, where design
		// 10.1 compares values with decoding cursors.
		c.labelA = a.AppendFootnoteReferenceLabel(c.labelA[:0], ia, resolved)
		c.labelB = b.AppendFootnoteReferenceLabel(c.labelB[:0], ib, resolved)
		return bytes.Equal(c.labelA, c.labelB)
	case Text, CodeText, VerbatimLineEnding, InfoString, HTMLText, LinkLabel, Destination, Title,
		FrontMatterText, BOM, LineEnding, BlankLine, Indent, ThematicRun, ATXMarker, ATXClose,
		Whitespace, CodeIndent, FenceMarker, SetextUnderline, QuoteMarker, ListMarker, ItemIndent,
		Bracket, Colon, AngleBracket, TitleQuote, FrontMatterFence, TrailingSpace, HardBreakMarker, Escape, EntityRef, CodeFence, AutolinkText, Delimiter, Paren,
		TablePipe, TableDelimiter, CellPipeEscape, TaskBox, FootnoteIndent, FootnoteLabel, Caret:
	}
	panic(fmt.Sprintf("markdown: key of node %d of kind %v, which is not a structure kind", ia, a.nodes[ia].kind))
}

type eventOp uint8

const (
	enterEvent eventOp = iota
	contentEvent
	exitEvent
)

// event is one event of a projection (design 10.1).
type event struct {
	op    eventOp
	id    NodeID // the node of an Enter or Exit event, or the first leaf of a content run
	end   NodeID // one past the last leaf of a content run
	group Kind   // the group of a content run (design 10.3)
	rows  dialectRows
}

// projection yields the events of a tree in order. Its stack is explicit
// (R-walk).
type projection struct {
	t     *Tree
	i     uint32   // the next node
	stack []uint32 // entered structure nodes
	label bool     // the walk is in the label of a link reference definition

	columns int // the columns of the last table entered
	cell    int // the cells entered in the last table row entered

	spans    []dialectSpan // the dialect spans after the walk, in node order
	dialect  bool          // the tree has dialect spans, so walk follows the walk
	walk     containerWalk
	spanWalk containerWalk       // walk at the last dialect span entered
	marks    map[NodeID]spanMark // where a nested span sits in the span that holds it
}

func newProjection(t *Tree) projection {
	p := projection{t: t, spans: t.dialectSpans()}
	p.dialect, p.walk.t = len(p.spans) > 0, t
	return p
}

func (p *projection) next() (event, bool) {
	nodes := p.t.nodes
	for {
		if top := len(p.stack) - 1; top >= 0 && nodes[p.stack[top]].link == p.i {
			id := p.stack[top]
			p.stack = p.stack[:top]
			return event{op: exitEvent, id: NodeID(id)}, true
		}
		if int(p.i) == len(nodes) {
			return event{}, false
		}
		i, n := p.i, nodes[p.i]
		p.i++
		p.visit(i)
		if n.kind.class() == classStructure {
			p.stack = append(p.stack, i)
			p.label = false
			switch n.kind {
			case Table:
				p.columns = p.t.TableColumns(NodeID(i))
			case TableRow:
				p.cell = 0
			case TableCell:
				p.cell++
			}
			e := event{op: enterEvent, id: NodeID(i)}
			for len(p.spans) > 0 && p.spans[0].id <= i {
				if p.spans[0].id == i {
					e.rows = p.spans[0].rows
					// The chain is shared, not copied: a container that is open
					// at a span stays open until after the span, so the walk
					// never rewrites these entries, and the full capacity makes
					// an append by the span's own walk copy.
					p.spanWalk = containerWalk{t: p.t, col: p.walk.col, chain: slices.Clip(p.walk.chain)}
				}
				p.spans = p.spans[1:]
			}
			if n.kind == CodeBlock || n.kind == HTMLBlock || n.kind == CodeSpan {
				// All their content is in their key. A block ends at the end of a
				// line, and no container starts later on the line of a code span.
				p.i, p.walk.col = n.link, 0
			}
			return e, true
		}
		parent := nodes[p.stack[len(p.stack)-1]]
		p.observe(n)
		group, ok := p.group(n, parent)
		if !ok {
			continue
		}
		for p.i < parent.link {
			m := nodes[p.i]
			if m.kind.class() == classStructure {
				break
			}
			if g, ok := p.group(m, parent); m.kind.class() == classContent && (!ok || g != group) {
				break
			}
			p.observe(m)
			p.visit(p.i)
			p.i++
		}
		return event{op: contentEvent, id: NodeID(i), end: NodeID(p.i), group: group}, true
	}
}

// skipEmptyCell skips e, the event that next returned last, and reports true,
// when e enters an empty table cell within the column count while the other
// tree exits a table row, of kind other: an empty cell equals a missing cell
// (design 10.3).
func (p *projection) skipEmptyCell(e event, other Kind) bool {
	n := p.t.nodes[e.id]
	if other != TableRow || e.op != enterEvent || n.kind != TableCell || p.cell > p.columns {
		return false
	}
	for _, m := range p.t.nodes[e.id+1 : n.link] {
		if m.kind.class() != classSyntax {
			return false
		}
	}
	p.stack = p.stack[:len(p.stack)-1]
	p.i = n.link
	return true
}

// visit follows the walk to node i when the tree has dialect spans.
func (p *projection) visit(i uint32) {
	if p.dialect {
		p.walk.visit(i)
	}
}

func (p *projection) observe(m Node) {
	if m.kind == Bracket {
		p.label = !p.label
	}
}

// group returns the content group of leaf m in node parent, or false when m
// gives no Content event: it is syntax, or its value is in a key (design
// 10.1, 10.3).
func (p *projection) group(m, parent Node) (Kind, bool) {
	if m.kind == CellPipeEscape {
		m.kind = Kind(m.flags)
	}
	switch {
	case m.kind.class() != classContent, m.kind == LinkLabel, m.kind == FootnoteLabel, parent.kind == LinkReferenceDefinition && p.label:
		return 0, false
	case m.kind == Escape, m.kind == EntityRef:
		return Text, true
	case m.kind != VerbatimLineEnding:
		return m.kind, true
	case parent.kind == FrontMatter:
		return FrontMatterText, true
	case parent.kind == RawHTML:
		return HTMLText, true
	case (parent.kind == Link || parent.kind == Image) && LinkForm(parent.flags) == FullReference:
		// A line ending in the label of a full reference.
		return 0, false
	case parent.kind == FootnoteReference:
		// A line ending in the label of a footnote reference.
		return 0, false
	case parent.kind == LinkReferenceDefinition, parent.kind == Link, parent.kind == Image:
		return Title, true
	}
	return m.kind, true
}

// pieceReader reads a value one piece at a time. next returns nil after the
// last piece, and no empty piece before it.
type pieceReader interface {
	next() []byte
}

// equalPieces reports whether a and b read the same bytes.
func equalPieces(a, b pieceReader) bool {
	var pa, pb []byte
	for {
		if len(pa) == 0 {
			pa = a.next()
		}
		if len(pb) == 0 {
			pb = b.next()
		}
		if len(pa) == 0 || len(pb) == 0 {
			return len(pa) == len(pb)
		}
		n := min(len(pa), len(pb))
		if !bytes.Equal(pa[:n], pb[:n]) {
			return false
		}
		pa, pb = pa[n:], pb[n:]
	}
}

func appendPieces(dst []byte, r pieceReader) []byte {
	for b := r.next(); b != nil; b = r.next() {
		dst = append(dst, b...)
	}
	return dst
}

// runReader reads the value of a content run: the values of its content
// leaves.
type runReader struct {
	t      *Tree
	i, end uint32
	leaf   valueReader
}

func (r *runReader) next() []byte {
	for {
		if b := r.leaf.next(); b != nil {
			return b
		}
		if r.i == r.end {
			return nil
		}
		if m := r.t.nodes[r.i]; m.kind.class() == classContent {
			r.leaf = r.t.newValueReader(m)
		}
		r.i++
	}
}

// equalSpans reports whether the dialect spans ia of pa's tree and ib of pb's
// tree, which the projections entered last, have equal bytes outside prefix
// leaves, and the same number of matched containers on each line after the
// first that is not blank (design 10.4).
func equalSpans(pa *projection, ia NodeID, pb *projection, ib NodeID) bool {
	if ma, okA := pa.marks[ia]; okA {
		// The streams of the span that holds both are equal, so the same
		// positions in them prove these spans equal.
		if mb, okB := pb.marks[ib]; okB {
			return ma == mb
		}
	}
	a, b := pa.t, pb.t
	ra := spanReader{t: a, span: uint32(ia), i: uint32(ia) + 1, end: a.nodes[ia].link, marks: pa.marker(false)}
	rb := spanReader{t: b, span: uint32(ib), i: uint32(ib) + 1, end: b.nodes[ib].link, marks: pb.marker(false)}
	if !equalPieces(&ra, &rb) {
		return false
	}
	la := spanLines{w: pa.spanWalk, i: uint32(ia) + 1, end: a.nodes[ia].link, marks: pa.marker(true)}
	lb := spanLines{w: pb.spanWalk, i: uint32(ib) + 1, end: b.nodes[ib].link, marks: pb.marker(true)}
	for {
		ma, okA := la.next()
		mb, okB := lb.next()
		if okA != okB || ma != mb {
			return false
		}
		if !okA {
			return true
		}
	}
}

// spanMark is where a nested dialect span starts and ends in the byte stream
// and the line stream of the span that holds it.
type spanMark struct{ startBytes, endBytes, startLines, endLines int }

// marker returns a marker for the dialect spans inside the span that the
// projection entered last, which fills p.marks as a reader of that span
// passes them. lines chooses the line stream.
func (p *projection) marker(lines bool) *marker {
	if p.marks == nil {
		p.marks = make(map[NodeID]spanMark)
	}
	// p.spans holds the spans after id, so the spans inside it come first.
	return &marker{t: p.t, spans: p.spans, marks: p.marks, lines: lines}
}

// marker records the marks of the dialect spans inside one span. A reader of
// that span calls at for each of its nodes, in order, with the position that
// the node starts at in the reader's stream.
type marker struct {
	t     *Tree
	spans []dialectSpan // the spans after the one being read, in node order
	open  []openSpan    // the spans entered, innermost last
	marks map[NodeID]spanMark
	lines bool
}

type openSpan struct {
	id  NodeID
	end uint32 // one past the last node of the span
}

func (m *marker) at(i, pos int) {
	for len(m.open) > 0 && int(m.open[len(m.open)-1].end) <= i {
		id := m.open[len(m.open)-1].id
		m.open = m.open[:len(m.open)-1]
		m.record(id, pos, false)
	}
	for len(m.spans) > 0 && int(m.spans[0].id) <= i {
		span := m.spans[0]
		m.spans = m.spans[1:]
		if int(span.id) < i {
			continue
		}
		m.record(NodeID(span.id), pos, true)
		m.open = append(m.open, openSpan{NodeID(span.id), m.t.nodes[span.id].link})
	}
}

// record writes one position of span id, the start or the end of its stream.
func (m *marker) record(id NodeID, pos int, start bool) {
	mark := m.marks[id]
	switch {
	case start && m.lines:
		mark.startLines = pos
	case start:
		mark.startBytes = pos
	case m.lines:
		mark.endLines = pos
	default:
		mark.endBytes = pos
	}
	m.marks[id] = mark
}

// spanReader reads the bytes of the leaves of a dialect span that are not
// prefix leaves or code indentation, with a split tab as its virt spaces
// (design 4.3) and each line ending as a line feed. Code indentation is not
// read because it can take columns inside a split tab too, where no leaf
// holds them.
type spanReader struct {
	t       *Tree
	marks   *marker
	off     int    // bytes given
	span    uint32 // the span node, whose own prefix leaves are not read
	i, end  uint32
	b       []byte // the rest of the last leaf
	last    byte   // the last byte read
	started bool   // a byte was read
	done    bool   // the reader gave the line feed that ends the input
}

func (r *spanReader) next() []byte {
	b := r.read()
	switch {
	case b != nil:
		r.last, r.started = b[len(b)-1], true
		r.off += len(b)
	case r.started && r.last != '\n' && !r.done:
		// The end of the input is a line ending (design 8.2).
		r.done = true
		return lineFeed[:1:1]
	}
	return b
}

func (r *spanReader) read() []byte {
	for len(r.b) == 0 {
		if r.i == r.end {
			return nil
		}
		m := r.t.nodes[r.i]
		r.marks.at(int(r.i), r.off)
		r.i++
		if m.kind.class() == classStructure {
			continue
		}
		// A prefix leaf of a container inside the span is read: past 99 blocks
		// on a line GitHub reads a marker as text (dialect.md). The markers of
		// the containers around the span are not. Indentation is not read
		// either: it means its columns, which a tab writes from the column it
		// starts at, and the printer writes them as spaces. A change of
		// indentation that changes a block shows in the events.
		if _, prefix := m.kind.owner(); prefix && m.link <= r.span || m.kind == CodeIndent || m.kind == Indent {
			// The leaf is not read, but its line holds it, so the end of the
			// input ends the line.
			r.last, r.started = ' ', true
			continue
		}
		r.b = r.t.src[m.start:m.end]
		if m.virt > 0 {
			r.b = r.b[1:]
			return spaces[:m.virt:m.virt]
		}
	}
	b := r.b
	switch i := bytes.IndexByte(b, '\r'); {
	case i < 0:
		r.b = nil
		return b
	case i > 0:
		r.b = b[i:]
		return b[:i]
	}
	r.b = bytes.TrimPrefix(b[1:], lineFeed)
	return lineFeed[:1:1]
}

// spanLines yields the number of containers that each line of a dialect span
// matched, for the lines after its first line that are not blank.
type spanLines struct {
	w         containerWalk // at the span node
	marks     *marker
	lines     int // lines given
	i, end    uint32
	lineStart bool
}

func (s *spanLines) next() (int, bool) {
	t := s.w.t
	for s.i < s.end {
		i := s.i
		s.marks.at(int(i), s.lines)
		s.i++
		n := t.nodes[i]
		matched, found := 0, false
		if s.lineStart && n.kind.class() != classStructure {
			s.lineStart = false
			if !t.blankRest(i, s.end) {
				matched, _ = s.w.matched(i)
				found = true
			}
		}
		s.w.visit(i)
		if c := t.src[n.end-1]; n.kind.class() != classStructure && (c == '\n' || c == '\r') {
			s.lineStart = true
		}
		if found {
			s.lines++
			return matched, true
		}
	}
	return 0, false
}

// blankRest reports whether the line from leaf i, before node end, has only
// prefix leaves, spaces and tabs before its line ending.
func (t *Tree) blankRest(i, end uint32) bool {
	for ; i < end; i++ {
		m := t.nodes[i]
		if _, prefix := m.kind.owner(); prefix || m.kind.class() == classStructure {
			continue
		}
		for _, c := range t.src[m.start:m.end] {
			switch c {
			case '\n', '\r':
				return true
			case ' ', '\t':
			default:
				return false
			}
		}
	}
	return true
}
