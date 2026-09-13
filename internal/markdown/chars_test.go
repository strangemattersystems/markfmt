package markdown

import (
	"testing"
	"unicode/utf8"
)

func TestDecodeRune(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		b    string
		r    rune
		n    int
	}{
		{"decodes ascii", "ab", 'a', 1},
		{"decodes a multibyte character", "ẞx", 'ẞ', 3},
		{"keeps an encoded replacement character", "�", utf8.RuneError, 3},
		{"replaces nul", "\x00a", utf8.RuneError, 1},
		{"replaces a truncated sequence as one character", "\xE2\x82a", utf8.RuneError, 2},
		{"replaces a lead byte whose next byte is out of range", "\xF0\x80\x80\x80", utf8.RuneError, 1},
		{"replaces a byte that is never valid", "\xC0\x80", utf8.RuneError, 1},
		{"replaces a lone continuation byte", "\x80", utf8.RuneError, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if r, n := decodeRune([]byte(tt.b)); r != tt.r || n != tt.n {
				t.Fatalf("decodeRune(%q) = %q, %d, want %q, %d", tt.b, r, n, tt.r, tt.n)
			}
		})
	}
}

func TestLabelFolder_Write(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		pieces []string
		want   string
	}{
		{"folds case", []string{"Foo BAR"}, "foo bar"},
		{"uses full case folding", []string{"ẞ ǅ ﬀ"}, "ss ǆ ff"},
		{"collapses and trims spaces, tabs and line endings", []string{" \ta \r\n", "\n b\t "}, "a b"},
		{"keeps other whitespace", []string{"a b"}, "a b"},
		{"keeps escapes as written", []string{`a\!`}, `a\!`},
		{"replaces invalid utf-8", []string{"a\xE2\x82b"}, "a�b"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := labelFolder{dst: []byte("x"), start: 1}
			for _, p := range tt.pieces {
				f.write([]byte(p))
			}
			if got := string(f.dst); got != "x"+tt.want {
				t.Fatalf("labelFolder of %q = %q, want %q", tt.pieces, got, "x"+tt.want)
			}
		})
	}
}
