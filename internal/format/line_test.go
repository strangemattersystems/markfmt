package format

import (
	"testing"

	"github.com/strangemattersystems/markfmt/internal/markdown"
)

func TestPrinter_Write(t *testing.T) {
	t.Parallel()

	t.Run("stops at the output limit", func(t *testing.T) {
		t.Parallel()

		p := printer{max: 4}
		p.write([]byte("abc"))
		p.write([]byte("de"))
		p.write([]byte("f"))
		if string(p.out) != "abc" || !p.full {
			t.Fatalf("out = %q, full = %t, want \"abc\", true", p.out, p.full)
		}
	})
}

func TestPrinter_PrefixLazyLine(t *testing.T) {
	t.Parallel()

	t.Run("stops at the output limit", func(t *testing.T) {
		t.Parallel()

		// "> a\nbbbbbbbb" is 12 bytes, and its lazy line takes a prefix of 2.
		p := printer{tree: markdown.Parse([]byte("> a\nbbbbbbbb\n")), max: 13}
		p.document()
		if len(p.out) > p.max || !p.full {
			t.Fatalf("out = %q, full = %t, want at most %d bytes and full", p.out, p.full, p.max)
		}
	})
}
