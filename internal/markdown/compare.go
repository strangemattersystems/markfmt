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
	case Document, BlockQuote, ListItem, Paragraph, ThematicBreak, SoftBreak, HardBreak, RawHTML, Emphasis, Strong:
		return true
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
		return a.AutolinkEmail(ia) == b.AutolinkEmail(ib)
	case CodeSpan:
		ra, rb := newCodeSpanReader(a, ia), newCodeSpanReader(b, ib)
		return equalPieces(&ra, &rb)
	case LinkReferenceDefinition:
		c.labelA, c.labelB = a.AppendLabel(c.labelA[:0], ia), b.AppendLabel(c.labelB[:0], ib)
		return bytes.Equal(c.labelA, c.labelB)
	case Text, CodeText, VerbatimLineEnding, InfoString, HTMLText, LinkLabel, Destination, Title,
		FrontMatterText, BOM, LineEnding, BlankLine, Indent, ThematicRun, ATXMarker, ATXClose,
		Whitespace, CodeIndent, FenceMarker, SetextUnderline, QuoteMarker, ListMarker, ItemIndent,
		Bracket, Colon, AngleBracket, TitleQuote, FrontMatterFence, TrailingSpace, HardBreakMarker, Escape, EntityRef, CodeFence, AutolinkText, Delimiter:
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
			if n.kind == CodeBlock || n.kind == HTMLBlock || n.kind == CodeSpan {
				// All their content is in their key.
				p.i = n.link
			}
			return event{op: enterEvent, id: NodeID(i)}, true
		}
		parent := nodes[p.stack[len(p.stack)-1]]
		p.observe(n)
		group, ok := p.group(n, parent.kind)
		if !ok {
			continue
		}
		for p.i < parent.link {
			m := nodes[p.i]
			if m.kind.class() == classStructure {
				break
			}
			if g, ok := p.group(m, parent.kind); m.kind.class() == classContent && (!ok || g != group) {
				break
			}
			p.observe(m)
			p.i++
		}
		return event{op: contentEvent, id: NodeID(i), end: NodeID(p.i), group: group}, true
	}
}

func (p *projection) observe(m Node) {
	if m.kind == Bracket {
		p.label = !p.label
	}
}

// group returns the content group of leaf m in a node of kind parent, or
// false when m gives no Content event: it is syntax, or its value is in its
// parent's key (design 10.1, 10.3).
func (p *projection) group(m Node, parent Kind) (Kind, bool) {
	switch {
	case m.kind.class() != classContent, parent == LinkReferenceDefinition && p.label:
		return 0, false
	case m.kind == Escape, m.kind == EntityRef:
		return Text, true
	case m.kind != VerbatimLineEnding:
		return m.kind, true
	case parent == FrontMatter:
		return FrontMatterText, true
	case parent == RawHTML:
		return HTMLText, true
	case parent == LinkReferenceDefinition:
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

// appendPieces appends every piece that r reads to dst.
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
