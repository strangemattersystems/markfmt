package markdown

import (
	"slices"
	"testing"
)

func TestContainerWalk_Matched(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  string
		want []int
	}{
		{"counts no container on a lazy line of a block quote", "> a\nb", []int{0}},
		{"counts a block quote", "> a\n> b", []int{1}},
		{"counts a list item", "- a\n  b", []int{1}},
		{"counts no container on a lazy line of a list item", "- a\nb", []int{0}},
		{"counts a block quote whose list item does not match", "> - a\n> b", []int{1}},
		{"counts a block quote and its list item", "> - a\n>   b", []int{2}},
		{"counts a list item that consumes part of a tab", "- a\n\tb", []int{1}},
		{"counts two list items that consume one tab", "- - a\n\t\tb", []int{2}},
		{"counts the list item whose indentation matches", "- - a\n   b", []int{1}},
		{"counts a list item after the optional space of a block quote in a tab", "> - a\n>\t b", []int{2}},
		{"counts a list item that consumes part of a tab after another list item", "> - - a\n>\t\t b", []int{3}},
		{"counts no list item that a tab after a block quote marker is too narrow for", "> 1. a\n>\tb", []int{1}},
		{"counts no list item after a list item that consumes part of a tab", "1. - b\n \tc", []int{1}},
		{"counts a footnote definition", "[^a]: x\n    y", []int{1}},
		{"counts a footnote definition that consumes a tab", "[^a]: x\n\ty", []int{1}},
		{"counts no container on a lazy line of a footnote definition", "[^a]: x\ny", []int{0}},
		{"counts a list item whose first line is blank", "-\n  a\n  b", []int{1}},
		{"counts a list item whose first line is blank in a tab", "-\n\ta\n\tb", []int{1}},
		{"counts each line", "> a\n> b\nc", []int{1, 0}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			parser, walk, parserEnds, walkEnds := matchedLines([]byte(tt.src))
			if !slices.Equal(parser, tt.want) || !slices.Equal(walk, tt.want) || !slices.Equal(parserEnds, walkEnds) {
				t.Fatalf("matched containers of %q: parser %v, walk %v, want %v; prefix ends: parser %v, walk %v", tt.src, parser, walk, tt.want, parserEnds, walkEnds)
			}
		})
	}

	t.Run("agrees with the parser on every corpus example", func(t *testing.T) {
		t.Parallel()

		for _, c := range corpora {
			for _, ex := range readExamples(t, c.path) {
				if parser, walk, parserEnds, walkEnds := matchedLines([]byte(ex.markdown)); !slices.Equal(parser, walk) || !slices.Equal(parserEnds, walkEnds) {
					t.Errorf("%s example %d: matched containers of %q: parser %v, walk %v; prefix ends: parser %v, walk %v", c.name, ex.id, ex.markdown, parser, walk, parserEnds, walkEnds)
				}
			}
		}
	})
}

func TestTree_ItemIndent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  string
		want int
	}{
		{"gives the marker and a space", "- a", 2},
		{"gives the indentation, the marker and the padding", " 10.  a", 6},
		{"gives 1 column of padding after a marker at the end of a line", "-\n", 2},
		{"gives 1 column of padding after a marker at the end of the input", "-", 2},
		{"gives 1 column of padding after a marker and spaces at the end of a line", "-   \n", 2},
		{"gives 1 column of padding before indented code", "-     a", 2},
		{"gives the columns of a tab after the marker", "-\ta", 4},
		{"gives the columns of a tab that the padding splits", "-\t    a", 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tree := Parse([]byte(tt.src))
			item := slices.IndexFunc(tree.nodes, func(n Node) bool { return n.kind == ListItem })
			if got := tree.itemIndent(NodeID(count(item)), 0); got != tt.want {
				t.Fatalf("itemIndent of %q = %d, want %d", tt.src, got, tt.want)
			}
		})
	}
}

// matchedLines returns, for each paragraph continuation line of src in
// order, the containers that the parser matched and the containers that a
// [containerWalk] counts. For each line that starts or continues a
// paragraph, it also returns the column where the parser's container
// prefixes end and the column that the walk gives.
func matchedLines(src []byte) (parser, walk, parserEnds, walkEnds []int) {
	type traced struct {
		start        uint32
		continuation bool
	}
	var lines []traced
	tree := parse(src, func(l line, matched, end int, continuation bool) {
		lines = append(lines, traced{start: l.start, continuation: continuation})
		parserEnds = append(parserEnds, end)
		if continuation {
			parser = append(parser, matched)
		}
	})
	w := containerWalk{t: tree}
	j := 0
	for i, n := range tree.nodes {
		if j < len(lines) && n.kind.class() != classStructure && n.start == lines[j].start {
			matched, end := w.matched(uint32(i))
			if lines[j].continuation {
				walk = append(walk, matched)
			}
			walkEnds = append(walkEnds, end)
			j++
		}
		w.visit(uint32(i))
	}
	return parser, walk, parserEnds, walkEnds
}

func TestInterruptsParagraph(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		line string
		lazy bool
		want bool
	}{
		{"starts a block quote", "> a", false, true},
		{"starts a thematic break", "* * *", false, true},
		{"starts an atx heading", "# a", false, true},
		{"starts a fenced code block", "~~~", false, true},
		{"starts an html block of kind 6", "<div>", false, true},
		{"starts no html block of kind 7", "<del>", false, false},
		{"starts a footnote definition", "[^a]: b", false, true},
		{"starts a bullet list item with content", "- a", false, true},
		{"starts no empty list item", "*", false, false},
		{"starts no ordered list item that does not start at 1", "2. a", false, false},
		{"starts an ordered list item at 1", "1) a", false, true},
		{"is a setext underline", "===", false, true},
		{"is a table delimiter row", "| - | :-: |", false, true},
		{"continues a paragraph with text", "a # b", false, false},
		{"starts an empty list item on a lazy line", "-", true, true},
		{"starts an ordered list item that does not start at 1 on a lazy line", "2. a", true, true},
		{"is no setext underline on a lazy line", "===", true, false},
		{"is no table delimiter row on a lazy line", "| - |", true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := InterruptsParagraph([]byte(tt.line), tt.lazy); got != tt.want {
				t.Fatalf("InterruptsParagraph(%q, %t) = %t, want %t", tt.line, tt.lazy, got, tt.want)
			}
		})
	}
}

func TestStartsBlock(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		line string
		want bool
	}{
		{"starts an atx heading", "# a", true},
		{"starts an html block of kind 7", "<del>", true},
		{"starts an ordered list item that does not start at 1", "2. a", true},
		{"starts an empty list item", "-", true},
		{"is no setext underline", "===", false},
		{"is no table delimiter row", "| - |", false},
		{"is text", "a <del>", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := StartsBlock([]byte(tt.line)); got != tt.want {
				t.Fatalf("StartsBlock(%q) = %t, want %t", tt.line, got, tt.want)
			}
		})
	}
}
