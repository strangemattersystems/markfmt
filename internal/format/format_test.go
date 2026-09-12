package format

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSource formats each testdata/cases/NAME.in.md file and compares the
// result with testdata/cases/NAME.out.md. A case without an .out.md file
// expects an error.
func TestSource(t *testing.T) {
	t.Parallel()

	inputs, err := filepath.Glob("testdata/cases/*.in.md")
	if err != nil {
		t.Fatal(err)
	}
	if len(inputs) == 0 {
		t.Fatal("no test cases in testdata")
	}

	for _, input := range inputs {
		base := strings.TrimSuffix(input, ".in.md")
		t.Run(strings.ReplaceAll(filepath.Base(base), "-", " "), func(t *testing.T) {
			t.Parallel()

			in, err := os.ReadFile(input)
			if err != nil {
				t.Fatal(err)
			}
			got, formatErr := Source(in)

			want, err := os.ReadFile(base + ".out.md")
			if errors.Is(err, fs.ErrNotExist) {
				if formatErr == nil {
					t.Fatalf("Source(%s) = %q, want an error", input, got)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}

			if formatErr != nil {
				t.Fatalf("Source(%s) error: %v", input, formatErr)
			}
			if string(got) != string(want) {
				t.Fatalf("Source(%s)\n got: %q\nwant: %q", input, got, want)
			}
			again, err := Source(got)
			if err != nil {
				t.Fatalf("Source(%q) error: %v", got, err)
			}
			if string(again) != string(got) {
				t.Fatalf("not idempotent\n once: %q\ntwice: %q", got, again)
			}
		})
	}
}
