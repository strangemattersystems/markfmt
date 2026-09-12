package markdown

import (
	"bytes"
	"slices"
	"testing"
)

func TestParse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  string
		want []Node
	}{
		{"gives an empty document for empty input", "", []Node{
			{kind: Document, link: 1},
		}},
		{"gives line ending leaves", "a\r\n\nb", []Node{
			{kind: Document, start: 0, end: 5, link: 5},
			{kind: Text, start: 0, end: 1},
			{kind: LineEnding, start: 1, end: 3},
			{kind: LineEnding, start: 3, end: 4},
			{kind: Text, start: 4, end: 5},
		}},
		{"gives a bom leaf", "\xEF\xBB\xBFa\n", []Node{
			{kind: Document, start: 0, end: 5, link: 4},
			{kind: BOM, start: 0, end: 3},
			{kind: Text, start: 3, end: 4},
			{kind: LineEnding, start: 4, end: 5},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := Parse([]byte(tt.src)).nodes; !slices.Equal(got, tt.want) {
				t.Fatalf("Parse(%q) nodes = %+v\nwant %+v", tt.src, got, tt.want)
			}
		})
	}
}

func FuzzParse(f *testing.F) {
	for _, src := range []string{"", "a", "a\nb\r\nc\rd\r\r\n"} {
		f.Add([]byte(src))
	}

	f.Fuzz(func(t *testing.T, src []byte) {
		tree := Parse(src)
		if err := tree.Verify(); err != nil {
			t.Fatalf("Parse(%q).Verify() = %v", src, err)
		}
		var leaves []byte
		for _, n := range tree.nodes {
			if n.kind.class() != classStructure {
				leaves = append(leaves, src[n.start:n.end]...)
			}
		}
		if !bytes.Equal(leaves, src) {
			t.Fatalf("leaves of Parse(%q) = %q", src, leaves)
		}
	})
}
