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
		{"keeps spaces beyond the indentation of blank lines", "    a\n  \n      \n    b", "a\n\n  \nb\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := string(Parse([]byte(tt.src)).AppendCode(nil, 1)); got != tt.want {
				t.Fatalf("AppendCode of %q = %q, want %q", tt.src, got, tt.want)
			}
		})
	}
}
