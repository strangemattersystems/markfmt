package markdown

import (
	"slices"
	"testing"
)

func TestBuilder_Open(t *testing.T) {
	t.Parallel()

	testPanics(t, []panicTest{
		{"panics on a leaf kind", "", func(b *builder) {
			b.open(Text)
		}},
		{"panics on a first node that is not a document", "", func(b *builder) {
			b.open(255)
		}},
		{"panics after the document closes", "", func(b *builder) {
			b.open(Document)
			b.close()
			b.open(Document)
		}},
	})
}

func TestBuilder_Leaf(t *testing.T) {
	t.Parallel()

	testPanics(t, []panicTest{
		{"panics on an interior kind", "a", func(b *builder) {
			b.open(Document)
			b.leaf(Document, 1)
		}},
		{"panics on a value that is not a kind", "a", func(b *builder) {
			b.open(Document)
			b.leaf(255, 1)
		}},
		{"panics outside the document", "a", func(b *builder) {
			b.leaf(Text, 1)
		}},
		{"panics on an empty leaf", "a", func(b *builder) {
			b.open(Document)
			b.leaf(Text, 0)
		}},
		{"panics past the input", "a", func(b *builder) {
			b.open(Document)
			b.leaf(Text, 2)
		}},
		{"panics above the node limit", "a", func(b *builder) {
			for range 6 {
				b.open(Document)
			}
			b.leaf(Text, 1)
		}},
	})
}

func TestBuilder_LeafIf(t *testing.T) {
	t.Parallel()

	t.Run("appends nothing for an empty leaf", func(t *testing.T) {
		t.Parallel()

		b := newBuilder([]byte("a"))
		b.open(Document)
		b.leafIf(Text, 0)
		b.leafIf(Text, 1)
		b.leafIf(Text, 1)
		b.close()
		want := []Node{
			{kind: Document, start: 0, end: 1, link: 2},
			{kind: Text, start: 0, end: 1},
		}
		if got := b.finish().nodes; !slices.Equal(got, want) {
			t.Fatalf("nodes = %+v, want %+v", got, want)
		}
	})
}

func TestBuilder_Close(t *testing.T) {
	t.Parallel()

	t.Run("closes the innermost open node", func(t *testing.T) {
		t.Parallel()

		b := newBuilder([]byte("ab"))
		b.open(Document)
		b.open(Document)
		b.leaf(Text, 1)
		b.close()
		b.leaf(Text, 2)
		b.close()
		want := []Node{
			{kind: Document, start: 0, end: 2, link: 4},
			{kind: Document, start: 0, end: 1, link: 3},
			{kind: Text, start: 0, end: 1},
			{kind: Text, start: 1, end: 2},
		}
		if got := b.finish().nodes; !slices.Equal(got, want) {
			t.Fatalf("nodes = %+v, want %+v", got, want)
		}
	})

	testPanics(t, []panicTest{
		{"panics with no open node", "", func(b *builder) {
			b.close()
		}},
	})
}

func TestBuilder_Finish(t *testing.T) {
	t.Parallel()

	testPanics(t, []panicTest{
		{"panics with an open node", "", func(b *builder) {
			b.open(Document)
			b.finish()
		}},
		{"panics without a document", "", func(b *builder) {
			b.finish()
		}},
		{"panics before the end of the input", "ab", func(b *builder) {
			b.open(Document)
			b.leaf(Text, 1)
			b.close()
			b.finish()
		}},
		{"panics above the node limit", "", func(b *builder) {
			for range 4 {
				b.open(Document)
			}
			for range 4 {
				b.close()
			}
			b.finish()
		}},
	})
}

type panicTest struct {
	name  string
	src   string
	build func(b *builder)
}

// testPanics runs each build on a new builder for its src, in a subtest that
// fails unless the build panics.
func testPanics(t *testing.T, tests []panicTest) {
	t.Helper()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			defer func() {
				if recover() == nil {
					t.Fatal("no panic")
				}
			}()
			tt.build(newBuilder([]byte(tt.src)))
		})
	}
}
