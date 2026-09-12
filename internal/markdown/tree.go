package markdown

import (
	"errors"
	"fmt"
)

// Node is one node of a [Tree]: a leaf that covers input bytes, or an
// interior node that groups the nodes after it.
type Node struct {
	kind  Kind
	flags uint8
	start uint32 // byte offset
	end   uint32 // byte offset, exclusive
	link  uint32 // interior: index one past the last descendant; leaf: 0
}

// Tree is a lossless concrete syntax tree: a preorder array of nodes whose
// leaves tile the input.
type Tree struct {
	src   []byte
	nodes []Node // nodes[0] is the document
}

// Verify reports the first broken invariant of t, or nil. Section 3.2 of
// docs/design/parser.md numbers the invariants.
func (t *Tree) Verify() error {
	nodes := t.nodes
	if len(nodes) == 0 || nodes[0].kind != Document {
		return errors.New("markdown: node 0 is not a document")
	}
	if uint64(len(nodes)) > 3*uint64(len(t.src))+3 {
		return fmt.Errorf("markdown: invariant 8: %d nodes for %d bytes", len(nodes), len(t.src))
	}

	var pos uint32 // end of the last leaf
	var open []int // interior nodes whose subtree is not passed yet
	exit := func(i int) error {
		for len(open) > 0 && int(nodes[open[len(open)-1]].link) == i {
			if n := nodes[open[len(open)-1]]; n.end != pos {
				return fmt.Errorf("markdown: invariant 3: node %d ends at %d, want %d", open[len(open)-1], n.end, pos)
			}
			open = open[:len(open)-1]
		}
		return nil
	}

	for i, n := range nodes {
		if err := exit(i); err != nil {
			return err
		}
		if i > 0 && len(open) == 0 {
			return fmt.Errorf("markdown: invariant 4: node %d is after the document", i)
		}
		switch c := n.kind.class(); {
		case c == classInvalid:
			return fmt.Errorf("markdown: invariant 6: node %d has kind %d", i, n.kind)
		case n.flags != 0:
			return fmt.Errorf("markdown: invariant 7: node %d has flags %#x", i, n.flags)
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
			open = append(open, i)
		case n.link != 0:
			return fmt.Errorf("markdown: invariant 5: leaf %d links to %d", i, n.link)
		case n.start != pos:
			return fmt.Errorf("markdown: invariant 1: leaf %d starts at %d, want %d", i, n.start, pos)
		case n.end <= n.start:
			return fmt.Errorf("markdown: invariant 2: leaf %d is empty", i)
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
	return nil
}
