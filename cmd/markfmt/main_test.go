package main

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/strangemattersystems/markfmt"
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
		if err := os.WriteFile(path, make([]byte, 2*markfmt.DefaultMaxInput), 0o600); err != nil {
			t.Fatal(err)
		}
		if src, err := read(path); err != nil || len(src) != markfmt.DefaultMaxInput+1 {
			t.Fatalf("read = %d bytes, %v, want %d bytes", len(src), err, markfmt.DefaultMaxInput+1)
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
		{"counts a small input at the share of one proc", 1, 4, markfmt.DefaultMaxInput / 4},
		{"counts a larger input at its size", markfmt.DefaultMaxInput / 2, 4, markfmt.DefaultMaxInput / 2},
		{"caps an input at the input limit", markfmt.DefaultMaxInput + 1, 4, markfmt.DefaultMaxInput},
		{"counts every input at the limit with one proc", 1, 1, markfmt.DefaultMaxInput},
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

// mainEnv is the environment variable that makes the test binary run main in
// place of the tests.
const mainEnv = "MARKFMT_MAIN"

const (
	formatted   = "# Title\n\n- item\n"
	unformatted = "#  Title\n\n\n*  item\n"
)

// TestMain runs main in the child process that runMarkfmt starts, and the
// tests in every other process.
func TestMain(m *testing.M) {
	if os.Getenv(mainEnv) == "1" {
		// main returns only for --version; its other paths call os.Exit.
		main()
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestMarkfmt(t *testing.T) {
	t.Parallel()

	t.Run("prints nothing for a formatted file with --check", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		write(t, filepath.Join(dir, "a.md"), formatted, 0o600)
		got := runMarkfmt(t, dir, "", "--check", "a.md")
		if got.status != 0 || got.stdout != "" || got.stderr != "" {
			t.Fatalf("markfmt --check a.md = %+v, want status 0 and no output", got)
		}
	})

	t.Run("prints an unformatted file with --check and leaves it alone", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		path := filepath.Join(dir, "a.md")
		write(t, path, unformatted, 0o600)
		got := runMarkfmt(t, dir, "", "--check", "a.md")
		if got.status != 1 || got.stdout != "a.md\n" {
			t.Fatalf("markfmt --check a.md = %+v, want status 1 and %q", got, "a.md\n")
		}
		if src := readFile(t, path); src != unformatted {
			t.Fatalf("a.md = %q, want it unchanged as %q", src, unformatted)
		}
	})

	t.Run("rewrites an unformatted file and keeps its mode", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		path := filepath.Join(dir, "a.md")
		write(t, path, unformatted, 0o640)
		// Windows keeps only a read-only bit, so the test compares the mode
		// with the one that the file has, not with 0o640.
		before, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		got := runMarkfmt(t, dir, "", "a.md")
		if got.status != 0 || got.stdout != "" || got.stderr != "" {
			t.Fatalf("markfmt a.md = %+v, want status 0 and no output", got)
		}
		if src := readFile(t, path); src != formatted {
			t.Fatalf("a.md = %q, want %q", src, formatted)
		}
		after, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if after.Mode().Perm() != before.Mode().Perm() {
			t.Fatalf("a.md mode = %v, want %v", after.Mode().Perm(), before.Mode().Perm())
		}
	})

	t.Run("reports a path that does not exist", func(t *testing.T) {
		t.Parallel()

		got := runMarkfmt(t, t.TempDir(), "", "missing.md")
		if got.status != 2 || !strings.Contains(got.stderr, "missing.md") {
			t.Fatalf("markfmt missing.md = %+v, want status 2 and the path on stderr", got)
		}
	})

	t.Run("formats standard input with no path", func(t *testing.T) {
		t.Parallel()

		got := runMarkfmt(t, t.TempDir(), unformatted)
		if got.status != 0 || got.stdout != formatted {
			t.Fatalf("markfmt = %+v, want status 0 and %q", got, formatted)
		}
	})

	t.Run("formats standard input for the path -", func(t *testing.T) {
		t.Parallel()

		got := runMarkfmt(t, t.TempDir(), unformatted, "-")
		if got.status != 0 || got.stdout != formatted {
			t.Fatalf("markfmt - = %+v, want status 0 and %q", got, formatted)
		}
	})

	t.Run("rewrites the markdown below a directory and skips hidden files", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		for _, name := range []string{"a.md", "sub/b.markdown", ".hidden/c.md", "d.txt"} {
			write(t, filepath.Join(dir, filepath.FromSlash(name)), unformatted, 0o600)
		}
		got := runMarkfmt(t, dir, "", ".")
		if got.status != 0 || got.stderr != "" {
			t.Fatalf("markfmt . = %+v, want status 0 and no output", got)
		}
		for name, want := range map[string]string{
			"a.md":           formatted,
			"sub/b.markdown": formatted,
			".hidden/c.md":   unformatted,
			"d.txt":          unformatted,
		} {
			if src := readFile(t, filepath.Join(dir, filepath.FromSlash(name))); src != want {
				t.Errorf("%s = %q, want %q", name, src, want)
			}
		}
	})

	t.Run("skips an excluded directory and file", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		for _, name := range []string{"a.md", "skipme/b.md", "c.md"} {
			write(t, filepath.Join(dir, filepath.FromSlash(name)), unformatted, 0o600)
		}
		got := runMarkfmt(t, dir, "", "--exclude", "skipme", "--exclude", "c.md", ".")
		if got.status != 0 || got.stderr != "" {
			t.Fatalf("markfmt --exclude = %+v, want status 0 and no output", got)
		}
		for name, want := range map[string]string{
			"a.md":        formatted,
			"skipme/b.md": unformatted,
			"c.md":        unformatted,
		} {
			if src := readFile(t, filepath.Join(dir, filepath.FromSlash(name))); src != want {
				t.Errorf("%s = %q, want %q", name, src, want)
			}
		}
	})

	t.Run("rewrites the target of a symlink and keeps the link", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		target := filepath.Join(dir, "target.md")
		write(t, target, unformatted, 0o600)
		link := filepath.Join(dir, "link.md")
		if err := os.Symlink("target.md", link); err != nil {
			t.Fatal(err)
		}
		got := runMarkfmt(t, dir, "", "link.md")
		if got.status != 0 || got.stderr != "" {
			t.Fatalf("markfmt link.md = %+v, want status 0 and no output", got)
		}
		if src := readFile(t, target); src != formatted {
			t.Fatalf("target.md = %q, want %q", src, formatted)
		}
		info, err := os.Lstat(link)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode()&os.ModeSymlink == 0 {
			t.Fatalf("link.md mode = %v, want a symlink", info.Mode())
		}
	})

	t.Run("prints the version", func(t *testing.T) {
		t.Parallel()

		got := runMarkfmt(t, t.TempDir(), "", "--version")
		if got.status != 0 || !strings.HasPrefix(got.stdout, "markfmt ") {
			t.Fatalf("markfmt --version = %+v, want status 0 and output starting %q", got, "markfmt ")
		}
	})

	t.Run("prints the usage for an unknown flag", func(t *testing.T) {
		t.Parallel()

		got := runMarkfmt(t, t.TempDir(), "", "--nope")
		if got.status != 2 || !strings.Contains(got.stderr, "usage: markfmt") {
			t.Fatalf("markfmt --nope = %+v, want status 2 and the usage on stderr", got)
		}
	})

	t.Run("reports an input above the input limit", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "large.md"), make([]byte, markfmt.DefaultMaxInput+1), 0o600); err != nil {
			t.Fatal(err)
		}
		got := runMarkfmt(t, dir, "", "large.md")
		if got.status != 2 || !strings.Contains(got.stderr, strconv.Itoa(markfmt.DefaultMaxInput)) {
			t.Fatalf("markfmt large.md = %+v, want status 2 and %d on stderr", got, markfmt.DefaultMaxInput)
		}
	})
}

type output struct {
	stdout, stderr string
	status         int
}

// runMarkfmt runs main in a child process with args, dir as its working
// directory and stdin on its standard input. main ends the process with
// [os.Exit], so a child process is what gives a status to observe.
func runMarkfmt(t *testing.T, dir, stdin string, args ...string) output {
	t.Helper()

	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(t.Context(), exe, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), mainEnv+"=1")
	cmd.Stdin = strings.NewReader(stdin)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr

	var exit *exec.ExitError
	if err := cmd.Run(); err != nil && !errors.As(err, &exit) {
		t.Fatal(err)
	}
	return output{stdout.String(), stderr.String(), cmd.ProcessState.ExitCode()}
}

// write creates the file at path, and the directories above it, with mode.
func write(t *testing.T, path, data string, mode os.FileMode) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), mode); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()

	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(src)
}
