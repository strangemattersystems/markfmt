package markdown_test

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/strangemattersystems/markfmt/internal/format"
	"github.com/strangemattersystems/markfmt/internal/markdown"
)

// FuzzFormat checks the formatter against the test HTML, so that Equal is not
// its own oracle (design 10.5).
func FuzzFormat(f *testing.F) {
	for _, src := range markdown.CorpusInputs(f) {
		f.Add(src)
	}
	cases, err := filepath.Glob("../format/testdata/cases/*.in.md")
	if err != nil || len(cases) == 0 {
		f.Fatalf("no cases in ../format/testdata/cases: %v", err)
	}
	for _, path := range cases {
		src, err := os.ReadFile(path)
		if err != nil {
			f.Fatal(err)
		}
		f.Add(src)
	}

	f.Fuzz(func(t *testing.T, src []byte) {
		out, err := format.Source(src)
		if err != nil {
			t.Fatalf("Source(%q) error: %v", src, err)
		}
		again, err := format.Source(out)
		if err != nil {
			t.Fatalf("Source(%q), the output of Source(%q), error: %v", out, src, err)
		}
		if !bytes.Equal(again, out) {
			t.Fatalf("Source is not idempotent\ninput: %q\n once: %q\ntwice: %q", src, out, again)
		}
		if in, got := markdown.RenderTestHTML(markdown.Parse(src)), markdown.RenderTestHTML(markdown.Parse(out)); in != got {
			t.Fatalf("Source(%q) = %q, whose test HTML differs:\n %q\n %q", src, out, in, got)
		}
		if in, got := markdown.Kept(markdown.Parse(src)), markdown.Kept(markdown.Parse(out)); !slices.Equal(in, got) {
			t.Fatalf("Source(%q) = %q, whose kept syntax differs:\n %q\n %q", src, out, in, got)
		}
	})
}
