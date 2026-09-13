package format

import "testing"

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
