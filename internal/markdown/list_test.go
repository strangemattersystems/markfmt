package markdown

import "testing"

func TestTree_ListStart(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		src     string
		start   int
		ordered bool
	}{
		{"reads a bullet list", "- a", 0, false},
		{"reads an ordered list", " 003) a", 3, true},
		{"reads a marker after a split tab", ">\t 1. a", 1, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tree := Parse([]byte(tt.src))
			start, ordered := tree.ListStart(firstOf(tree, List))
			if start != tt.start || ordered != tt.ordered {
				t.Fatalf("ListStart of %q = %d, %v, want %d, %v", tt.src, start, ordered, tt.start, tt.ordered)
			}
		})
	}
}

func TestTree_ListLoose(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  string
		want bool
	}{
		{"reads a tight list", "- a\n- b", false},
		{"reads a loose list", "- a\n\n- b", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tree := Parse([]byte(tt.src))
			if got := tree.ListLoose(firstOf(tree, List)); got != tt.want {
				t.Fatalf("ListLoose of %q = %v, want %v", tt.src, got, tt.want)
			}
		})
	}
}
