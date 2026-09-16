package main

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestInputs(t *testing.T) {
	t.Parallel()

	t.Run("walks a directory for markdown files in order", func(t *testing.T) {
		t.Parallel()

		root := tree(t, "b.md", "a.markdown", "sub/c.MD", "d.txt", "sub/e.mdx")
		if got, want := paths(t, inputs([]string{root}), root), []string{"a.markdown", "b.md", "sub/c.MD"}; !slices.Equal(got, want) {
			t.Fatalf("inputs = %q, want %q", got, want)
		}
	})

	t.Run("skips testdata, hidden and vendored directories", func(t *testing.T) {
		t.Parallel()

		root := tree(t, "a.md", "testdata/b.md", ".github/c.md", "vendor/d.md", "node_modules/e.md", "sub/testdata/f.md", "sub/.x.md")
		if got, want := paths(t, inputs([]string{root}), root), []string{"a.md", "sub/.x.md"}; !slices.Equal(got, want) {
			t.Fatalf("inputs = %q, want %q", got, want)
		}
	})

	t.Run("walks a root that a skip would match", func(t *testing.T) {
		t.Parallel()

		root := filepath.Join(tree(t, "testdata/a.md"), "testdata")
		if got, want := paths(t, inputs([]string{root}), root), []string{"a.md"}; !slices.Equal(got, want) {
			t.Fatalf("inputs = %q, want %q", got, want)
		}
	})

	t.Run("keeps a file and stdin as given", func(t *testing.T) {
		t.Parallel()

		root := tree(t, "notes.txt")
		file := filepath.Join(root, "notes.txt")
		got := inputs([]string{file, "-"})
		if len(got) != 2 || got[0].path != file || got[1].path != "-" || got[0].err != nil || got[1].err != nil {
			t.Fatalf("inputs = %v, want %s and - without errors", got, file)
		}
	})

	t.Run("gives an error for a path that does not exist", func(t *testing.T) {
		t.Parallel()

		missing := filepath.Join(t.TempDir(), "missing")
		if got := inputs([]string{missing}); len(got) != 1 || got[0].err == nil {
			t.Fatalf("inputs = %v, want one input with an error", got)
		}
	})
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
