package markdown

import (
	"slices"
	"testing"
)

func TestLines_Next(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  string
		want []line
	}{
		{"splits at lf", "a\nb", []line{{0, 1, 2}, {2, 3, 3}}},
		{"splits at cr", "a\rb", []line{{0, 1, 2}, {2, 3, 3}}},
		{"splits at crlf", "a\r\nb", []line{{0, 1, 3}, {3, 4, 4}}},
		{"reads cr cr lf as two line endings", "a\r\r\nb", []line{{0, 1, 2}, {2, 2, 4}, {4, 5, 5}}},
		{"reads empty lines", "\n\r\n", []line{{0, 0, 1}, {1, 1, 3}}},
		{"gives the last line no line ending", "a", []line{{0, 1, 1}}},
		{"ends at a final line ending", "a\n", []line{{0, 1, 2}}},
		{"gives no line for empty input", "", nil},
		{"starts after a bom", "\xEF\xBB\xBFa\n", []line{{3, 4, 5}}},
		{"gives no line for a bom alone", "\xEF\xBB\xBF", nil},
		{"reads a partial bom as content", "\xEF\xBBa", []line{{0, 3, 3}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var got []line
			it := newLines([]byte(tt.src))
			for l, ok := it.next(); ok; l, ok = it.next() {
				got = append(got, l)
			}
			if !slices.Equal(got, tt.want) {
				t.Fatalf("lines of %q = %v, want %v", tt.src, got, tt.want)
			}
		})
	}
}
