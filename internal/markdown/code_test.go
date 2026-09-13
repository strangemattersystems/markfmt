package markdown

import "testing"

func TestTree_AppendCode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  string
		want string
	}{
		{"writes a line feed for each line ending", "    a\r\n\t b\r    c\n", "a\n b\nc\n"},
		{"adds a line feed after a last line without one", "    a", "a\n"},
		{"adds a line feed after a last fenced line without one", "```\nx", "x\n"},
		{"gives no content for fences alone", "```\n```", ""},
		{"gives no content for an opening fence at the end of the input", "```js\n", ""},
		{"removes up to the fence indentation", "  ```\n a\n   b\n```", "a\n b\n"},
		{"keeps spaces beyond the indentation of blank lines", "    a\n  \n      \n    b", "a\n\n  \nb\n"},
		{"writes the columns left of a split tab as spaces", ">\t\tfoo\n", "  foo\n"},
		{"keeps a split tab alone on the last line", "> ```\n>\t", "  \n"},
		{"writes u+fffd for nul and each maximal invalid utf-8 subsequence", ">\t\t\x00a\xe1\x80b", "  \ufffda\ufffdb\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tree := Parse([]byte(tt.src))
			if got := string(tree.AppendCode(nil, firstOf(tree, CodeBlock))); got != tt.want {
				t.Fatalf("AppendCode of %q = %q, want %q", tt.src, got, tt.want)
			}
		})
	}
}
