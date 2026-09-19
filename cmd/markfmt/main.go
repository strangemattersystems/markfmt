// Markfmt formats Markdown files in one canonical style.
//
// Usage:
//
//	markfmt [--check] [--exclude pattern]... [path ...]
//	markfmt --version
//
// Markfmt rewrites each path in place. A directory path gives each file below
// it with the extension .md or .markdown, and skips the files and directories
// below it whose names start with a dot. With no path, or the path "-", it
// reads standard input and writes standard output.
//
// With --check, markfmt rewrites nothing. It prints each input that is not
// formatted and exits with status 1.
//
// With --exclude, markfmt also skips each file and directory below a path whose
// name, or path as markfmt prints it, matches the pattern. The pattern syntax
// is that of [filepath.Match]. The flag can repeat.
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"

	"github.com/strangemattersystems/markfmt"
)

// The release build sets these with -ldflags -X.
var (
	version  = "0.0.0-dev"
	revision = "unknown"
	date     = "unknown"
)

func main() {
	check := flag.Bool("check", false, "print unformatted inputs and exit with status 1; rewrite nothing")
	var exclude []string
	flag.Func("exclude", "skip files and directories below a path whose name or path matches `pattern`; repeatable", func(pattern string) error {
		pattern = filepath.Clean(pattern)
		if _, err := filepath.Match(pattern, ""); err != nil {
			return err
		}
		exclude = append(exclude, pattern)
		return nil
	})
	printVersion := flag.Bool("version", false, "print the version and exit")
	flag.Usage = usage
	flag.Parse()

	if *printVersion {
		// go install builds without the release's -ldflags, but records the
		// module version.
		if info, ok := debug.ReadBuildInfo(); ok && version == "0.0.0-dev" && strings.HasPrefix(info.Main.Version, "v") {
			version = strings.TrimPrefix(info.Main.Version, "v")
		}
		fmt.Printf("markfmt %s (revision %s, built %s)\n", version, revision, date)
		return
	}

	paths := flag.Args()
	if len(paths) == 0 {
		paths = []string{"-"}
	}

	ins := inputs(paths, exclude)
	results := make([]result, len(ins))
	// Peak memory follows the input bytes that are formatted at once, so they
	// stay within the input limit of one file. Each input is read before it
	// takes its cost, so the cost is the bytes read, not a size from os.Stat
	// that can change.
	b := newBudget(markfmt.MaxInput)
	procs := runtime.GOMAXPROCS(0)
	var wg sync.WaitGroup
	for i, in := range ins {
		if in.err != nil {
			results[i].err = in.err
			continue
		}
		src, err := read(in.path)
		if err != nil {
			results[i].err = err
			continue
		}
		cost := cost(len(src), procs)
		b.acquire(cost)
		wg.Go(func() {
			defer b.release(cost)
			results[i].changed, results[i].err = run(in.path, src, *check)
		})
	}
	wg.Wait()

	status := 0
	for i, r := range results {
		switch {
		case r.err != nil:
			fmt.Fprintf(os.Stderr, "markfmt: %s: %v\n", ins[i].path, r.err)
			if ie := (*markfmt.InternalError)(nil); errors.As(r.err, &ie) {
				fmt.Fprintf(os.Stderr, "%s", ie.Stack)
			}
			status = 2
		case r.changed && *check:
			fmt.Println(ins[i].path)
			if status == 0 {
				status = 1
			}
		}
	}
	os.Exit(status)
}

// usage prints the flags with two dashes. The flag package accepts one or two.
func usage() {
	fmt.Fprintln(os.Stderr, "usage: markfmt [--check] [--exclude pattern]... [path ...]")
	fmt.Fprintln(os.Stderr, "       markfmt --version")
	flag.VisitAll(func(f *flag.Flag) {
		name, usage := flag.UnquoteUsage(f)
		if name != "" {
			name = " " + name
		}
		fmt.Fprintf(os.Stderr, "  --%s%s\n    \t%s\n", f.Name, name, usage)
	})
}

// input is a path to format, or the error of finding it.
type input struct {
	path string
	err  error
}

type result struct {
	changed bool
	err     error
}

// inputs returns the inputs of paths in order: a directory gives its Markdown
// files in lexical order, less those that [skip] reports, and any other path is
// kept as given.
func inputs(paths, exclude []string) []input {
	var ins []input
	for _, root := range paths {
		info, err := os.Stat(root)
		if root == "-" || err == nil && !info.IsDir() {
			ins = append(ins, input{path: root})
			continue
		}
		if err != nil {
			ins = append(ins, input{path: root, err: err})
			continue
		}
		err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			switch {
			case err != nil:
				ins = append(ins, input{path: path, err: err})
			case path == root:
			case skip(path, d.Name(), exclude):
				if d.IsDir() {
					return filepath.SkipDir
				}
			case d.Type().IsRegular() && isMarkdown(path):
				ins = append(ins, input{path: path})
			}
			return nil
		})
		if err != nil {
			ins = append(ins, input{path: root, err: err})
		}
	}
	return ins
}

// skip reports whether a walk leaves out the entry at path: a hidden entry, or
// one whose name or path matches an exclude pattern.
func skip(path, name string, exclude []string) bool {
	if strings.HasPrefix(name, ".") {
		return true
	}
	for _, pattern := range exclude {
		// flag.Parse rejects a malformed pattern, so Match returns no error.
		if m, _ := filepath.Match(pattern, name); m {
			return true
		}
		if m, _ := filepath.Match(pattern, path); m {
			return true
		}
	}
	return false
}

func isMarkdown(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".md", ".markdown":
		return true
	}
	return false
}

// read returns the contents of path, or standard input for "-". It stops one
// byte past [markfmt.MaxInput], so an input that [markfmt.Format] rejects costs
// no more memory than one it accepts.
func read(path string) ([]byte, error) {
	f := os.Stdin
	if path != "-" {
		var err error
		if f, err = os.Open(path); err != nil {
			return nil, err
		}
		defer func() { _ = f.Close() }()
	}
	return io.ReadAll(io.LimitReader(f, markfmt.MaxInput+1))
}

// cost returns the budget that formatting n input bytes takes: n up to the
// input limit, and no less than the limit divided by procs, which bounds the
// files at once to procs.
func cost(n, procs int) int {
	return max(min(n, markfmt.MaxInput), markfmt.MaxInput/procs)
}

// budget is a count of bytes that goroutines take and give back.
type budget struct {
	mu   sync.Mutex
	cond sync.Cond
	free int
}

func newBudget(n int) *budget {
	b := &budget{free: n}
	b.cond.L = &b.mu
	return b
}

// acquire takes n bytes, and waits until they are free.
func (b *budget) acquire(n int) {
	b.mu.Lock()
	for b.free < n {
		b.cond.Wait()
	}
	b.free -= n
	b.mu.Unlock()
}

func (b *budget) release(n int) {
	b.mu.Lock()
	b.free += n
	b.mu.Unlock()
	b.cond.Broadcast()
}

// run formats src, the contents of path, and reports whether formatting
// changed it.
func run(path string, src []byte, check bool) (bool, error) {
	var out bytes.Buffer
	if err := markfmt.Format(&out, bytes.NewReader(src)); err != nil {
		return false, err
	}
	changed := !bytes.Equal(src, out.Bytes())

	switch {
	case check:
		return changed, nil
	case path == "-":
		_, err := os.Stdout.Write(out.Bytes())
		return changed, err
	case changed:
		return true, writeFile(path, out.Bytes())
	}
	return false, nil
}

// writeFile replaces the file at path with data. It renames a temporary file
// over the target, so an interrupted write leaves the original file intact.
func writeFile(path string, data []byte) error {
	// Resolve symlinks so the rename replaces the target, not the link.
	path, err := filepath.EvalSymlinks(path)
	if err != nil {
		return err
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".markfmt-*")
	if err != nil {
		return err
	}

	_, err = tmp.Write(data)
	if err == nil {
		err = tmp.Chmod(info.Mode().Perm())
	}
	if err == nil {
		err = tmp.Sync()
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(tmp.Name(), path)
	}
	if err != nil {
		_ = os.Remove(tmp.Name())
	}
	return err
}
