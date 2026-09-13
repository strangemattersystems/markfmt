package markdown

import "testing"

func TestTree_AppendHTML(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  string
		want string
	}{
		{"writes a line feed for each line ending", "<div>\r\n  a\n", "<div>\n  a\n"},
		{"adds a line feed after a last line without one", "<!--\n-->", "<!--\n-->\n"},
		{"writes the columns left of a split tab as spaces", "> <!--\n>\t\n> -->", "<!--\n  \n-->\n"},
		{"writes u+fffd for nul and each maximal invalid utf-8 subsequence", "<div>\x00a\xe1\x80b", "<div>\ufffda\ufffdb\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tree := Parse([]byte(tt.src))
			if got := string(tree.AppendHTML(nil, firstOf(tree, HTMLBlock))); got != tt.want {
				t.Fatalf("AppendHTML of %q = %q, want %q", tt.src, got, tt.want)
			}
		})
	}
}

func TestTree_HTMLBlockClosed(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  string
		want bool
	}{
		{"closes a comment on the line of its end condition", "<!-- a\nb -->\n\nc", true},
		{"leaves a comment without its end condition open", "<!--\na\n\n", false},
		{"leaves open a comment whose container closes", "> <!--\n> a\n\nb", false},
		{"closes a block of kind 6 at a blank line", "<div>\n\na", true},
		{"closes a block of kind 7 at the end of the input", "<del>", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tree := Parse([]byte(tt.src))
			if got := tree.HTMLBlockClosed(firstOf(tree, HTMLBlock)); got != tt.want {
				t.Fatalf("HTMLBlockClosed of %q = %t, want %t", tt.src, got, tt.want)
			}
		})
	}
}
