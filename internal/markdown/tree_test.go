package markdown

import (
	"strings"
	"testing"
)

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

	t.Run("rejects a tree whose first node is not a document", func(t *testing.T) {
		t.Parallel()

		for _, tree := range []*Tree{
			{src: []byte("a"), nodes: []Node{{kind: Text, start: 0, end: 1}}},
			{},
		} {
			if err := tree.Verify(); err == nil {
				t.Errorf("Verify() of %+v = nil, want an error", tree.nodes)
			}
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
