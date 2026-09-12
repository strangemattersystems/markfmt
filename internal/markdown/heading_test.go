package markdown

import "testing"

func TestTree_HeadingLevel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  string
		want int
	}{
		{"reads an atx heading", "### a", 3},
		{"reads an indented atx heading", "  ###### a", 6},
		{"reads a setext heading", "a\n= \n", 1},
		{"reads a setext heading of level 2", "a\nb\n--", 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := Parse([]byte(tt.src)).HeadingLevel(1); got != tt.want {
				t.Fatalf("HeadingLevel of %q = %d, want %d", tt.src, got, tt.want)
			}
		})
	}
}
