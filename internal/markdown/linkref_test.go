package markdown

import "testing"

func TestTree_AppendLabel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  string
		want string
	}{
		{"normalizes a label", "[Foo  BAR]: /u", "foo bar"},
		{"normalizes a label over lines", "[\n  Foo\n ẞ ]: /u 'a\nb'", "foo ss"},
		{"normalizes a label in a block quote", "> [a\n> b]: /u", "a b"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tree := Parse([]byte(tt.src))
			if got := string(tree.AppendLabel(nil, firstOf(tree, LinkReferenceDefinition))); got != tt.want {
				t.Fatalf("AppendLabel of %q = %q, want %q", tt.src, got, tt.want)
			}
		})
	}
}
