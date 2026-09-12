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

func TestBuilder_Flag(t *testing.T) {
	t.Parallel()

	t.Run("sets the flags of the innermost open node", func(t *testing.T) {
		t.Parallel()

		b := newBuilder([]byte("<p>"))
		b.open(Document)
		b.open(HTMLBlock)
		b.flag(6)
		b.leaf(HTMLText, 3)
		b.close()
		b.close()
		if got := b.finish().nodes[1].flags; got != 6 {
			t.Fatalf("flags = %d, want 6", got)
		}
	})

	testPanics(t, []panicTest{
		{"panics on flags out of range for the kind", "", func(b *builder) {
			b.open(Document)
			b.flag(1)
		}},
		{"panics with no open node", "", func(b *builder) {
			b.flag(1)
		}},
	})
}

func TestBuilder_Leaf(t *testing.T) {
	t.Parallel()

	t.Run("gives the next leaf the columns left of a split tab", func(t *testing.T) {
		t.Parallel()

		b := newBuilder([]byte("\ta"))
		b.open(Document)
		b.split = 2
		b.leaf(Text, 1)
		b.leaf(Text, 2)
		b.close()
		want := []Node{
			{kind: Document, start: 0, end: 2, link: 3},
			{kind: Text, virt: 2, start: 0, end: 1},
			{kind: Text, start: 1, end: 2},
		}
		if got := b.finish().nodes; !slices.Equal(got, want) {
			t.Fatalf("nodes = %+v, want %+v", got, want)
		}
	})

	testPanics(t, []panicTest{
		{"panics on virt for a leaf that does not start with a tab", "a", func(b *builder) {
			b.open(Document)
			b.split = 1
			b.leaf(Text, 1)
		}},
		{"panics on virt above 3", "\t", func(b *builder) {
			b.open(Document)
			b.split = 4
			b.leaf(Text, 1)
		}},
		{"panics on an interior kind", "a", func(b *builder) {
			b.open(Document)
			b.leaf(Document, 1)
		}},
		{"panics on a value that is not a kind", "a", func(b *builder) {
			b.open(Document)
			b.leaf(255, 1)
		}},
		{"panics on a prefix kind", ">", func(b *builder) {
			b.open(Document)
			b.open(BlockQuote)
			b.leaf(QuoteMarker, 1)
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

func TestBuilder_Prefix(t *testing.T) {
	t.Parallel()

	t.Run("appends a leaf with its owner", func(t *testing.T) {
		t.Parallel()

		b := newBuilder([]byte(">"))
		b.open(Document)
		b.open(BlockQuote)
		b.prefix(QuoteMarker, 1, 1)
		b.close()
		b.close()
		want := []Node{
			{kind: Document, start: 0, end: 1, link: 3},
			{kind: BlockQuote, start: 0, end: 1, link: 3},
			{kind: QuoteMarker, start: 0, end: 1, link: 1},
		}
		if got := b.finish().nodes; !slices.Equal(got, want) {
			t.Fatalf("nodes = %+v, want %+v", got, want)
		}
	})

	testPanics(t, []panicTest{
		{"panics on a kind that is not a prefix kind", "a", func(b *builder) {
			b.open(Document)
			b.prefix(Text, 1, 0)
		}},
		{"panics on an owner of another kind", ">", func(b *builder) {
			b.open(Document)
			b.prefix(QuoteMarker, 1, 0)
		}},
		{"panics on a closed owner", ">", func(b *builder) {
			b.open(Document)
			b.open(BlockQuote)
			b.close()
			b.prefix(QuoteMarker, 1, 1)
		}},
		{"panics on an owner after the last node", ">", func(b *builder) {
			b.open(Document)
			b.prefix(QuoteMarker, 1, 5)
		}},
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
