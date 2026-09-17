package main

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/strangemattersystems/markfmt/internal/format"
)

func TestInputs(t *testing.T) {
	t.Parallel()

	t.Run("walks a directory for markdown files in order", func(t *testing.T) {
		t.Parallel()

		root := tree(t, "b.md", "a.markdown", "sub/c.MD", "d.txt", "sub/e.mdx")
		if got, want := paths(t, inputs([]string{root}, nil), root), []string{"a.markdown", "b.md", "sub/c.MD"}; !slices.Equal(got, want) {
			t.Fatalf("inputs = %q, want %q", got, want)
		}
	})

	t.Run("walks testdata and vendored directories", func(t *testing.T) {
		t.Parallel()

		root := tree(t, "testdata/a.md", "vendor/b.md", "node_modules/c.md")
		if got, want := paths(t, inputs([]string{root}, nil), root), []string{"node_modules/c.md", "testdata/a.md", "vendor/b.md"}; !slices.Equal(got, want) {
			t.Fatalf("inputs = %q, want %q", got, want)
		}
	})

	t.Run("skips hidden files and directories", func(t *testing.T) {
		t.Parallel()

		root := tree(t, "a.md", ".github/b.md", "sub/.c.md")
		if got, want := paths(t, inputs([]string{root}, nil), root), []string{"a.md"}; !slices.Equal(got, want) {
			t.Fatalf("inputs = %q, want %q", got, want)
		}
	})

	t.Run("skips excluded files and directories by name or path", func(t *testing.T) {
		t.Parallel()

		root := tree(t, "a.md", "b.markdown", "testdata/c.md", "sub/testdata/d.md", "docs/e.md", "docs/f.md")
		exclude := []string{"testdata", "*.markdown", filepath.Join(root, "docs", "e.md")}
		if got, want := paths(t, inputs([]string{root}, exclude), root), []string{"a.md", "docs/f.md"}; !slices.Equal(got, want) {
			t.Fatalf("inputs = %q, want %q", got, want)
		}
	})

	t.Run("walks a root that a skip would match", func(t *testing.T) {
		t.Parallel()

		root := filepath.Join(tree(t, ".github/a.md", "testdata/b.md"), ".github")
		if got, want := paths(t, inputs([]string{root}, nil), root), []string{"a.md"}; !slices.Equal(got, want) {
			t.Fatalf("inputs = %q, want %q", got, want)
		}
		root = filepath.Join(filepath.Dir(root), "testdata")
		if got, want := paths(t, inputs([]string{root}, []string{"testdata"}), root), []string{"b.md"}; !slices.Equal(got, want) {
			t.Fatalf("inputs = %q, want %q", got, want)
		}
	})

	t.Run("keeps a file and stdin as given", func(t *testing.T) {
		t.Parallel()

		root := tree(t, "notes.txt")
		file := filepath.Join(root, "notes.txt")
		got := inputs([]string{file, "-"}, nil)
		if len(got) != 2 || got[0].path != file || got[1].path != "-" || got[0].err != nil || got[1].err != nil {
			t.Fatalf("inputs = %v, want %s and - without errors", got, file)
		}
	})

	t.Run("gives an error for a path that does not exist", func(t *testing.T) {
		t.Parallel()

		missing := filepath.Join(t.TempDir(), "missing")
		if got := inputs([]string{missing}, nil); len(got) != 1 || got[0].err == nil {
			t.Fatalf("inputs = %v, want one input with an error", got)
		}
	})
}

func TestRead(t *testing.T) {
	t.Parallel()

	t.Run("reads a file", func(t *testing.T) {
		t.Parallel()

		root := tree(t, "a.md")
		if src, err := read(filepath.Join(root, "a.md")); err != nil || string(src) != "a\n" {
			t.Fatalf("read = %q, %v, want %q", src, err, "a\n")
		}
	})

	t.Run("stops one byte past the input limit", func(t *testing.T) {
		t.Parallel()

		path := filepath.Join(t.TempDir(), "large.md")
		if err := os.WriteFile(path, make([]byte, 2*format.MaxInput), 0o600); err != nil {
			t.Fatal(err)
		}
		if src, err := read(path); err != nil || len(src) != format.MaxInput+1 {
			t.Fatalf("read = %d bytes, %v, want %d bytes", len(src), err, format.MaxInput+1)
		}
	})

	t.Run("gives an error for a file that does not exist", func(t *testing.T) {
		t.Parallel()

		if _, err := read(filepath.Join(t.TempDir(), "missing")); err == nil {
			t.Fatal("read gives no error")
		}
	})
}

func TestCost(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name     string
		n, procs int
		want     int
	}{
		{"counts a small input at the share of one proc", 1, 4, format.MaxInput / 4},
		{"counts a larger input at its size", format.MaxInput / 2, 4, format.MaxInput / 2},
		{"caps an input at the input limit", format.MaxInput + 1, 4, format.MaxInput},
		{"counts every input at the limit with one proc", 1, 1, format.MaxInput},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := cost(tt.n, tt.procs); got != tt.want {
				t.Fatalf("cost(%d, %d) = %d, want %d", tt.n, tt.procs, got, tt.want)
			}
		})
	}
}

func TestBudget_Acquire(t *testing.T) {
	t.Parallel()

	t.Run("waits until the bytes are released", func(t *testing.T) {
		t.Parallel()

		b := newBudget(10)
		b.acquire(10)
		done := make(chan struct{})
		go func() {
			b.acquire(1)
			close(done)
		}()
		select {
		case <-done:
			t.Fatal("acquire returned while no bytes were free")
		default:
		}
		b.release(10)
		<-done
	})
}

// tree creates the files at the slash-separated paths below a new directory,
// and returns the directory.
func tree(t *testing.T, files ...string) string {
	t.Helper()

	root := t.TempDir()
	for _, f := range files {
		path := filepath.Join(root, filepath.FromSlash(f))
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("a\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// paths returns the paths of ins relative to root, with slashes, and fails the
// test on an input with an error.
func paths(t *testing.T, ins []input, root string) []string {
	t.Helper()

	var out []string
	for _, in := range ins {
		if in.err != nil {
			t.Fatal(in.err)
		}
		rel, err := filepath.Rel(root, in.path)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, filepath.ToSlash(rel))
	}
	return out
}
