package markdown

import (
	"slices"
	"strings"
	"testing"
)

func TestTree_Walk(t *testing.T) {
	t.Parallel()

	t.Run("enters and exits nodes in preorder", func(t *testing.T) {
		t.Parallel()

		tree := &Tree{src: []byte("ab"), nodes: []Node{
			{kind: Document, start: 0, end: 2, link: 5},
			{kind: Paragraph, start: 0, end: 1, link: 3},
			{kind: Text, start: 0, end: 1},
			{kind: Paragraph, start: 1, end: 1, link: 4},
			{kind: Text, start: 1, end: 2},
		}}
		want := []Event{{0, false}, {1, false}, {2, false}, {1, true}, {3, false}, {3, true}, {4, false}, {0, true}}
		var got []Event
		c := tree.Walk()
		for e, ok := c.Next(); ok; e, ok = c.Next() {
			got = append(got, e)
		}
		if !slices.Equal(got, want) {
			t.Fatalf("events = %v, want %v", got, want)
		}
	})
}

func TestTree_Verify(t *testing.T) {
	t.Parallel()

	t.Run("accepts a valid tree", func(t *testing.T) {
		t.Parallel()

		tree := &Tree{src: []byte("ab"), nodes: []Node{
			{kind: Document, start: 0, end: 2, link: 4},
			{kind: Document, start: 0, end: 1, link: 3},
			{kind: Text, start: 0, end: 1},
			{kind: Text, start: 1, end: 2},
		}}
		if err := tree.Verify(); err != nil {
			t.Fatalf("Verify() = %v, want nil", err)
		}
	})

	type badTree struct {
		name string
		tree Tree
	}
	invariants := []struct {
		name  string
		trees []badTree
	}{
		{"invariant 1", []badTree{
			{"rejects a gap between leaves", Tree{src: []byte("abc"), nodes: []Node{
				{kind: Document, start: 0, end: 3, link: 3},
				{kind: Text, start: 0, end: 1},
				{kind: Text, start: 2, end: 3},
			}}},
			{"rejects overlapping leaves", Tree{src: []byte("abc"), nodes: []Node{
				{kind: Document, start: 0, end: 3, link: 3},
				{kind: Text, start: 0, end: 2},
				{kind: Text, start: 1, end: 3},
			}}},
			{"rejects a first leaf that does not start at 0", Tree{src: []byte("ab"), nodes: []Node{
				{kind: Document, start: 0, end: 2, link: 2},
				{kind: Text, start: 1, end: 2},
			}}},
			{"rejects a last leaf that ends before the input", Tree{src: []byte("ab"), nodes: []Node{
				{kind: Document, start: 0, end: 1, link: 2},
				{kind: Text, start: 0, end: 1},
			}}},
			{"rejects a last leaf that ends after the input", Tree{src: []byte("ab"), nodes: []Node{
				{kind: Document, start: 0, end: 3, link: 2},
				{kind: Text, start: 0, end: 3},
			}}},
			{"rejects input with no leaves", Tree{src: []byte("a"), nodes: []Node{
				{kind: Document, start: 0, end: 0, link: 1},
			}}},
		}},
		{"invariant 2", []badTree{
			{"rejects an empty leaf", Tree{src: []byte("a"), nodes: []Node{
				{kind: Document, start: 0, end: 1, link: 3},
				{kind: Text, start: 0, end: 0},
				{kind: Text, start: 0, end: 1},
			}}},
			{"rejects virt on a leaf that does not start with a tab", Tree{src: []byte("a"), nodes: []Node{
				{kind: Document, start: 0, end: 1, link: 2},
				{kind: Text, virt: 1, start: 0, end: 1},
			}}},
			{"rejects virt above 3", Tree{src: []byte("\t"), nodes: []Node{
				{kind: Document, start: 0, end: 1, link: 2},
				{kind: CodeText, virt: 4, start: 0, end: 1},
			}}},
		}},
		{"invariant 3", []badTree{
			{"rejects a start before the first leaf", Tree{src: []byte("ab"), nodes: []Node{
				{kind: Document, start: 0, end: 2, link: 4},
				{kind: Text, start: 0, end: 1},
				{kind: Document, start: 0, end: 2, link: 4},
				{kind: Text, start: 1, end: 2},
			}}},
			{"rejects an end after the last leaf", Tree{src: []byte("ab"), nodes: []Node{
				{kind: Document, start: 0, end: 2, link: 4},
				{kind: Document, start: 0, end: 2, link: 3},
				{kind: Text, start: 0, end: 1},
				{kind: Text, start: 1, end: 2},
			}}},
			{"rejects an empty node away from the next leaf", Tree{src: []byte("a"), nodes: []Node{
				{kind: Document, start: 0, end: 1, link: 3},
				{kind: Document, start: 1, end: 1, link: 2},
				{kind: Text, start: 0, end: 1},
			}}},
			{"rejects an empty last node away from the input end", Tree{src: []byte("a"), nodes: []Node{
				{kind: Document, start: 0, end: 1, link: 3},
				{kind: Text, start: 0, end: 1},
				{kind: Document, start: 0, end: 0, link: 3},
			}}},
		}},
		{"invariant 4", []badTree{
			{"rejects a first node that is not a document", Tree{src: []byte("a"), nodes: []Node{
				{kind: Text, start: 0, end: 1},
			}}},
			{"rejects a tree with no nodes", Tree{}},
			{"rejects a link not after its own index", Tree{src: []byte("a"), nodes: []Node{
				{kind: Document, start: 0, end: 1, link: 3},
				{kind: Document, start: 0, end: 1, link: 1},
				{kind: Text, start: 0, end: 1},
			}}},
			{"rejects a link after its parent's link", Tree{src: []byte("a"), nodes: []Node{
				{kind: Document, start: 0, end: 1, link: 2},
				{kind: Document, start: 0, end: 1, link: 3},
				{kind: Text, start: 0, end: 1},
			}}},
			{"rejects a document link after the last node", Tree{src: []byte("a"), nodes: []Node{
				{kind: Document, start: 0, end: 1, link: 3},
				{kind: Text, start: 0, end: 1},
			}}},
			{"rejects a node after the document", Tree{src: []byte("a"), nodes: []Node{
				{kind: Document, start: 0, end: 0, link: 1},
				{kind: Text, start: 0, end: 1},
			}}},
		}},
		{"invariant 5", []badTree{
			{"rejects a leaf with a link", Tree{src: []byte("a"), nodes: []Node{
				{kind: Document, start: 0, end: 1, link: 2},
				{kind: Text, start: 0, end: 1, link: 1},
			}}},
			{"rejects a prefix leaf whose owner is not an ancestor", Tree{src: []byte(">>"), nodes: []Node{
				{kind: Document, start: 0, end: 2, link: 4},
				{kind: BlockQuote, start: 0, end: 1, link: 3},
				{kind: QuoteMarker, start: 0, end: 1, link: 1},
				{kind: QuoteMarker, start: 1, end: 2, link: 1},
			}}},
			{"rejects a prefix leaf whose owner has another kind", Tree{src: []byte(">"), nodes: []Node{
				{kind: Document, start: 0, end: 1, link: 2},
				{kind: QuoteMarker, start: 0, end: 1},
			}}},
		}},
		{"invariant 6", []badTree{
			{"rejects a value that is not a kind", Tree{src: []byte("a"), nodes: []Node{
				{kind: Document, start: 0, end: 1, link: 2},
				{kind: 255, start: 0, end: 1},
			}}},
		}},
		{"invariant 7", []badTree{
			{"rejects a flag outside the kind's mask", Tree{src: []byte("a"), nodes: []Node{
				{kind: Document, start: 0, end: 1, link: 2},
				{kind: Text, flags: 1, start: 0, end: 1},
			}}},
			{"rejects a list flag out of range", Tree{src: []byte("-"), nodes: []Node{
				{kind: Document, start: 0, end: 1, link: 4},
				{kind: List, flags: 2, start: 0, end: 1, link: 4},
				{kind: ListItem, start: 0, end: 1, link: 4},
				{kind: ListMarker, start: 0, end: 1, link: 2},
			}}},
			{"rejects an html block kind out of range", Tree{src: []byte("<p>"), nodes: []Node{
				{kind: Document, start: 0, end: 3, link: 3},
				{kind: HTMLBlock, flags: 8, start: 0, end: 3, link: 3},
				{kind: HTMLText, start: 0, end: 3},
			}}},
			{"rejects an html block kind that its first line does not start", Tree{src: []byte(" <p>"), nodes: []Node{
				{kind: Document, start: 0, end: 4, link: 3},
				{kind: HTMLBlock, flags: 7, start: 0, end: 4, link: 3},
				{kind: HTMLText, start: 0, end: 4},
			}}},
			{"rejects a task item with no task box", Tree{src: []byte("- a"), nodes: []Node{
				{kind: Document, start: 0, end: 3, link: 6},
				{kind: List, start: 0, end: 3, link: 6},
				{kind: ListItem, flags: 1, start: 0, end: 3, link: 6},
				{kind: ListMarker, start: 0, end: 2, link: 2},
				{kind: Paragraph, start: 2, end: 3, link: 6},
				{kind: Text, start: 2, end: 3},
			}}},
			{"rejects a task box in a list item that is not a task", Tree{src: []byte("- [ ] a"), nodes: []Node{
				{kind: Document, start: 0, end: 7, link: 8},
				{kind: List, start: 0, end: 7, link: 8},
				{kind: ListItem, start: 0, end: 7, link: 8},
				{kind: ListMarker, start: 0, end: 2, link: 2},
				{kind: Paragraph, start: 2, end: 7, link: 8},
				{kind: TaskBox, start: 2, end: 5},
				{kind: Whitespace, start: 5, end: 6},
				{kind: Text, start: 6, end: 7},
			}}},
			{"rejects a table cell alignment that its delimiter row cell does not give", Tree{src: []byte("a|\n-|"), nodes: []Node{
				{kind: Document, start: 0, end: 5, link: 9},
				{kind: Table, start: 0, end: 5, link: 9},
				{kind: TableRow, start: 0, end: 3, link: 7},
				{kind: TableCell, flags: 5, start: 0, end: 1, link: 5},
				{kind: Text, start: 0, end: 1},
				{kind: TablePipe, start: 1, end: 2},
				{kind: LineEnding, start: 2, end: 3},
				{kind: TableDelimiter, start: 3, end: 4},
				{kind: TablePipe, start: 4, end: 5},
			}}},
		}},
		{"invariant 8", []badTree{
			{"rejects more than 3 nodes per byte plus 3", Tree{nodes: []Node{
				{kind: Document, link: 4},
				{kind: Document, link: 4},
				{kind: Document, link: 4},
				{kind: Document, link: 4},
			}}},
		}},
	}
	for _, inv := range invariants {
		t.Run(inv.name, func(t *testing.T) {
			t.Parallel()

			for _, bad := range inv.trees {
				t.Run(bad.name, func(t *testing.T) {
					t.Parallel()

					err := bad.tree.Verify()
					if err == nil || !strings.Contains(err.Error(), inv.name+":") {
						t.Fatalf("Verify() = %v, want an error for %s", err, inv.name)
					}
				})
			}
		})
	}
}
