package markdown

import (
	"fmt"
	"math"
)

// builder is the only writer of tree nodes. Its checks are always on and
// panic.
type builder struct {
	tree  Tree
	stack []uint32 // open interior nodes, innermost last
	pos   uint32   // end of the last leaf
	size  uint32   // len(tree.src)
	split uint8    // virt of the next leaf: the columns left of a split tab at its start
}

func newBuilder(src []byte) *builder {
	return &builder{tree: Tree{src: src}, size: inputSize(src)}
}

func (b *builder) next() uint32 {
	n := len(b.tree.nodes)
	if n >= math.MaxUint32 {
		panic("markdown: too many nodes for uint32 node indices")
	}
	return uint32(n)
}

// open appends an interior node of kind k at the end of the last leaf.
func (b *builder) open(k Kind) {
	switch {
	case k.class() != classStructure:
		panic(fmt.Sprintf("markdown: open of kind %d, which is not an interior kind", k))
	case len(b.stack) == 0 && (len(b.tree.nodes) > 0 || k != Document):
		panic(fmt.Sprintf("markdown: node %d of kind %d is outside the document", len(b.tree.nodes), k))
	}
	b.stack = append(b.stack, b.next())
	b.tree.nodes = append(b.tree.nodes, Node{kind: k, start: b.pos})
}

// flag sets the flags of the innermost open node.
func (b *builder) flag(f uint8) {
	if len(b.stack) == 0 {
		panic("markdown: flag with no open node")
	}
	n := &b.tree.nodes[b.stack[len(b.stack)-1]]
	if !n.kind.validFlags(f) {
		panic(fmt.Sprintf("markdown: flags %#x out of range for kind %v", f, n.kind))
	}
	n.flags = f
}

// top returns the index of the innermost open node.
func (b *builder) top() uint32 {
	return b.stack[len(b.stack)-1]
}

// leaf appends a leaf of kind k from the end of the last leaf to end.
func (b *builder) leaf(k Kind, end uint32) {
	if _, ok := k.owner(); ok {
		panic(fmt.Sprintf("markdown: leaf of prefix kind %v, which needs an owner", k))
	}
	b.appendLeaf(k, end, 0)
}

// prefix appends a prefix leaf of kind k from the end of the last leaf to
// end, owned by the open container at index owner.
func (b *builder) prefix(k Kind, end, owner uint32) {
	want, ok := k.owner()
	switch {
	case !ok:
		panic(fmt.Sprintf("markdown: prefix leaf of kind %v, which is not a prefix kind", k))
	case uint64(owner) >= uint64(len(b.tree.nodes)) || b.tree.nodes[owner].kind != want || b.tree.nodes[owner].link != 0:
		panic(fmt.Sprintf("markdown: prefix leaf of kind %v owned by node %d, which is not an open %v", k, owner, want))
	}
	b.appendLeaf(k, end, owner)
}

func (b *builder) appendLeaf(k Kind, end, link uint32) {
	switch c := k.class(); {
	case c == classInvalid || c == classStructure:
		panic(fmt.Sprintf("markdown: leaf of kind %d, which is not a leaf kind", k))
	case len(b.stack) == 0:
		panic(fmt.Sprintf("markdown: leaf %d is outside the document", len(b.tree.nodes)))
	case end <= b.pos:
		panic(fmt.Sprintf("markdown: leaf %d ends at %d, not after its start %d", len(b.tree.nodes), end, b.pos))
	case end > b.size:
		panic(fmt.Sprintf("markdown: leaf %d ends at %d, after the input end %d", len(b.tree.nodes), end, len(b.tree.src)))
	case uint64(len(b.tree.nodes)) >= 3*uint64(end)+3:
		panic(fmt.Sprintf("markdown: more than 3 × %d + 3 nodes", end))
	case b.split > 3 || b.split > 0 && b.tree.src[b.pos] != '\t':
		panic(fmt.Sprintf("markdown: leaf %d has virt %d at a byte %q", len(b.tree.nodes), b.split, b.tree.src[b.pos]))
	}
	b.tree.nodes = append(b.tree.nodes, Node{kind: k, virt: b.split, start: b.pos, end: end, link: link})
	b.pos, b.split = end, 0
}

// leafIf is [builder.leaf], but appends nothing when end is the end of the
// last leaf.
func (b *builder) leafIf(k Kind, end uint32) {
	if end != b.pos {
		b.leaf(k, end)
	}
}

// close closes the innermost open node at the end of the last leaf.
func (b *builder) close() {
	if len(b.stack) == 0 {
		panic("markdown: close with no open node")
	}
	top := len(b.stack) - 1
	n := &b.tree.nodes[b.stack[top]]
	b.stack = b.stack[:top]
	n.end = b.pos
	n.link = b.next()
}

func (b *builder) finish() *Tree {
	switch {
	case len(b.tree.nodes) == 0:
		panic("markdown: finish without a document")
	case len(b.stack) > 0:
		panic(fmt.Sprintf("markdown: finish with %d open nodes", len(b.stack)))
	case uint64(b.pos) != uint64(len(b.tree.src)):
		panic(fmt.Sprintf("markdown: leaves end at %d, before the input end %d", b.pos, len(b.tree.src)))
	case uint64(len(b.tree.nodes)) > 3*uint64(len(b.tree.src))+3:
		panic(fmt.Sprintf("markdown: %d nodes, more than 3 × %d + 3", len(b.tree.nodes), len(b.tree.src)))
	}
	return &b.tree
}
