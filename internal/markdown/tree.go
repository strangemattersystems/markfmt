package markdown

import (
	"errors"
	"fmt"
	"math"
)

// Node is one node of a [Tree]: a leaf that covers input bytes, or an
// interior node that groups the nodes after it.
type Node struct {
	kind  Kind
	flags uint8
	virt  uint8 // columns left for the leaf of its first byte, a split tab
	_     uint8
	start uint32 // byte offset
	end   uint32 // byte offset, exclusive
	link  uint32 // interior: index one past the last descendant; leaf: 0
}

// Tree is a lossless concrete syntax tree: a preorder array of nodes whose
// leaves tile the input.
type Tree struct {
	src   []byte
	nodes []Node        // nodes[0] is the document
	spans []dialectSpan // the dialect spans, found on the first call
}

// NodeID is the index of a node in a [Tree].
type NodeID uint32

// Kind returns the kind of node id.
func (t *Tree) Kind(id NodeID) Kind {
	return t.nodes[id].kind
}

// Next returns the node after node id and its subtree, and false after the
// last node.
func (t *Tree) Next(id NodeID) (NodeID, bool) {
	next := uint32(id) + 1
	if n := t.nodes[id]; n.kind.class() == classStructure {
		next = n.link
	}
	return NodeID(next), int(next) < len(t.nodes)
}

// Around returns the input byte before node id and the input byte after it,
// with 0 at the start and at the end of the input.
func (t *Tree) Around(id NodeID) (before, after byte) {
	n := t.nodes[id]
	if n.start > 0 {
		before = t.src[n.start-1]
	}
	if int(n.end) < len(t.src) {
		after = t.src[n.end]
	}
	return before, after
}

// RestOfLine returns the bytes from the start of leaf id to the end of its
// line, without the line ending.
func (t *Tree) RestOfLine(id NodeID) []byte {
	start := t.nodes[id].start
	return t.src[start:lineEnd(t.src, start)]
}

// SplitTab returns the columns left for leaf id of a tab at its start that
// structures consumed in part, or 0 (design 4.3).
func (t *Tree) SplitTab(id NodeID) int {
	return int(t.nodes[id].virt)
}

// Len returns the number of nodes of t.
func (t *Tree) Len() int {
	return len(t.nodes)
}

// NodeSpan returns the input bytes that node id covers.
func (t *Tree) NodeSpan(id NodeID) (start, end uint32) {
	n := t.nodes[id]
	return n.start, n.end
}

// Raw returns the source bytes of node id.
func (t *Tree) Raw(id NodeID) []byte {
	n := t.nodes[id]
	return t.src[n.start:n.end]
}

// Event is the enter or the exit of a node in a walk. A leaf has only an
// enter event.
type Event struct {
	ID   NodeID
	Exit bool
}

// Cursor walks a [Tree] in preorder. Its stack is explicit, so a deep tree
// cannot overflow the Go stack.
type Cursor struct {
	nodes []Node
	next  uint32
	stack []uint32 // entered interior nodes, innermost last
}

// Walk returns a cursor at the start of t.
func (t *Tree) Walk() Cursor {
	return Cursor{nodes: t.nodes}
}

// Next returns the next event, or false after the exit of the document.
func (c *Cursor) Next() (Event, bool) {
	if n := len(c.stack); n > 0 {
		if top := c.stack[n-1]; c.nodes[top].link == c.next {
			c.stack = c.stack[:n-1]
			return Event{ID: NodeID(top), Exit: true}, true
		}
	}
	if int(c.next) == len(c.nodes) {
		return Event{}, false
	}
	id := c.next
	c.next++
	if c.nodes[id].kind.class() == classStructure {
		c.stack = append(c.stack, id)
	}
	return Event{ID: NodeID(id)}, true
}

// Verify reports the first broken invariant of t, or nil. Section 3.2 of
// docs/design/parser.md numbers the invariants.
func (t *Tree) Verify() error {
	nodes := t.nodes
	if len(nodes) == 0 || nodes[0].kind != Document {
		return errors.New("markdown: invariant 4: node 0 is not a document")
	}
	if uint64(len(nodes)) > 3*uint64(len(t.src))+3 {
		return fmt.Errorf("markdown: invariant 8: %d nodes for %d bytes", len(nodes), len(t.src))
	}

	var pos uint32    // end of the last leaf
	var open []uint32 // interior nodes whose subtree is not passed yet
	exit := func(i int) error {
		for len(open) > 0 && int(nodes[open[len(open)-1]].link) == i {
			if n := nodes[open[len(open)-1]]; n.end != pos {
				return fmt.Errorf("markdown: invariant 3: node %d ends at %d, want %d", open[len(open)-1], n.end, pos)
			}
			open = open[:len(open)-1]
		}
		return nil
	}

	var aligns []Alignment // the alignments of the columns of the last table entered
	row, col := -1, 0      // the index of the last row entered in its table, and of the next cell in that row
	tasks, boxes := 0, 0   // the task items and the task boxes
	for i, n := range nodes {
		if err := exit(i); err != nil {
			return err
		}
		switch n.kind {
		case Table:
			aligns, row = t.appendAlignments(aligns[:0], NodeID(i)), -1
		case TableRow:
			row, col = row+1, 0
		case TableCell:
			if want := cellFlags(aligns, row, col); n.flags != want {
				return fmt.Errorf("markdown: invariant 7: table cell %d has flags %#x, which its delimiter row and row do not give", i, n.flags)
			}
			col++
		case ListItem:
			if n.flags != 0 {
				tasks++
			}
		case TaskBox:
			boxes++
		}
		if i > 0 && len(open) == 0 {
			return fmt.Errorf("markdown: invariant 4: node %d is after the document", i)
		}
		switch c := n.kind.class(); {
		case c == classInvalid:
			return fmt.Errorf("markdown: invariant 6: node %d has kind %d", i, n.kind)
		case !n.kind.validFlags(n.flags):
			return fmt.Errorf("markdown: invariant 7: node %d has flags %#x", i, n.flags)
		case n.kind == ListItem && n.flags != 0 && !t.taskBoxAgrees(i):
			return fmt.Errorf("markdown: invariant 7: list item %d has flags %#x, which no task box gives", i, n.flags)
		case n.kind == HTMLBlock && !t.htmlKindAgrees(i):
			return fmt.Errorf("markdown: invariant 7: html block %d has kind %d, which its first line does not start", i, n.flags)
		case c == classStructure:
			limit := len(nodes)
			if len(open) > 0 {
				limit = int(nodes[open[len(open)-1]].link)
			}
			if link := int(n.link); link <= i || link > limit {
				return fmt.Errorf("markdown: invariant 4: node %d links to %d, want %d to %d", i, link, i+1, limit)
			}
			if n.start != pos {
				return fmt.Errorf("markdown: invariant 3: node %d starts at %d, want %d", i, n.start, pos)
			}
			open = append(open, count(i))
		case !t.leafLinkValid(i):
			return fmt.Errorf("markdown: invariant 5: leaf %d links to %d", i, n.link)
		case n.start != pos:
			return fmt.Errorf("markdown: invariant 1: leaf %d starts at %d, want %d", i, n.start, pos)
		case n.end <= n.start:
			return fmt.Errorf("markdown: invariant 2: leaf %d is empty", i)
		case n.virt > 3 || n.virt > 0 && (int(n.start) >= len(t.src) || t.src[n.start] != '\t'):
			return fmt.Errorf("markdown: invariant 2: leaf %d has virt %d", i, n.virt)
		default:
			pos = n.end
		}
	}
	if err := exit(len(nodes)); err != nil {
		return err
	}
	if uint64(pos) != uint64(len(t.src)) {
		return fmt.Errorf("markdown: invariant 1: leaves end at %d, want %d", pos, len(t.src))
	}
	if tasks != boxes {
		return fmt.Errorf("markdown: invariant 7: %d task boxes for %d task items", boxes, tasks)
	}
	return nil
}

// leafLinkValid reports whether leaf i links to an ancestor container of
// its owner kind when it is a prefix leaf, and to 0 when it is not.
func (t *Tree) leafLinkValid(i int) bool {
	n := t.nodes[i]
	want, ok := n.kind.owner()
	if !ok {
		return n.link == 0
	}
	return int(n.link) < i && t.nodes[n.link].kind == want && int(t.nodes[n.link].link) > i
}

// inputSize returns len(src). It panics when src is too large for uint32
// offsets and node indices: a tree has up to 3 × len(src) + 3 nodes.
func inputSize(src []byte) uint32 {
	n := len(src)
	if n > (math.MaxUint32-3)/3 {
		panic(fmt.Sprintf("markdown: input of %d bytes is too large for uint32 node indices", n))
	}
	return uint32(n)
}
