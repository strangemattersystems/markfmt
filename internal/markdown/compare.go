package markdown

import (
	"bytes"
	"fmt"
)

// Equal reports the first difference between the event projections of a and
// b, or nil when they are equal (design 10). The projection of a tree has an
// Enter event with a key for each structure node, a Content event for each
// run of content leaves of one group, and an Exit event at the end of each
// structure node.
func Equal(a, b *Tree) error {
	c := comparer{a: a, b: b}
	pa, pb := projection{t: a}, projection{t: b}
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
	}
}

type comparer struct {
	a, b           *Tree
	labelA, labelB []byte // normalized labels, bounded by the label cap
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
	case ea.op == enterEvent && !c.equalKeys(ea.id, eb.id):
		what = "different keys"
	case ea.op == contentEvent && ea.group != eb.group:
		what = "different content groups"
	case ea.op == contentEvent && !equalPieces(
		&runReader{t: c.a, i: uint32(ea.id), end: uint32(ea.end)},
		&runReader{t: c.b, i: uint32(eb.id), end: uint32(eb.end)}):
		what = "different content"
	default:
		return nil
	}
	return fmt.Errorf("markdown: %s: %s against %s", what, describe(c.a, ea, okA), describe(c.b, eb, okB))
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
		infoA, infoB := a.infoReader(ia), b.infoReader(ib)
		ra, rb := newVerbatimReader(a, ia, CodeText), newVerbatimReader(b, ib, CodeText)
		return equalPieces(&infoA, &infoB) && equalPieces(&ra, &rb)
	case HTMLBlock:
		ra, rb := newVerbatimReader(a, ia, HTMLText), newVerbatimReader(b, ib, HTMLText)
		return equalPieces(&ra, &rb)
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
		ra, rb := newCodeSpanReader(a, ia), newCodeSpanReader(b, ib)
		return equalPieces(&ra, &rb)
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
			if n.kind == CodeBlock || n.kind == HTMLBlock || n.kind == CodeSpan {
				// All their content is in their key.
				p.i = n.link
			}
			return event{op: enterEvent, id: NodeID(i)}, true
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
