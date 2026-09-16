// Markfmt formats Markdown files in one canonical style.
//
// Usage:
//
//	markfmt [-check] [path ...]
//
// Markfmt rewrites each path in place. A directory path gives each file below
// it with the extension .md or .markdown, outside directories named testdata,
// vendor or node_modules and hidden directories. With no path, or the path
// "-", it reads standard input and writes standard output.
//
// With -check, markfmt rewrites nothing. It prints each input that is not
// formatted and exits with status 1.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/strangemattersystems/markfmt"
	"github.com/strangemattersystems/markfmt/internal/format"
)

func main() {
	check := flag.Bool("check", false, "print unformatted inputs and exit with status 1; rewrite nothing")
	flag.Parse()

	paths := flag.Args()
	if len(paths) == 0 {
		paths = []string{"-"}
	}

	ins := inputs(paths)
	results := make([]result, len(ins))
	// Peak memory follows the input bytes that are formatted at once, so they
	// stay within the input limit of one file (design 7.2).
	b := newBudget(format.MaxInput)
	var wg sync.WaitGroup
	for i, in := range ins {
		if in.err != nil {
			results[i].err = in.err
			continue
		}
		cost := cost(in.path)
		b.acquire(cost)
		wg.Go(func() {
			defer b.release(cost)
			results[i].changed, results[i].err = run(in.path, *check)
		})
	}
	wg.Wait()

	status := 0
	for i, r := range results {
		switch {
		case r.err != nil:
			fmt.Fprintf(os.Stderr, "markfmt: %s: %v\n", ins[i].path, r.err)
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
// files in lexical order, and any other path is kept as given.
func inputs(paths []string) []input {
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
			case d.IsDir() && path != root && skipDir(d.Name()):
				return filepath.SkipDir
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

// skipDir reports whether a directory below a path holds files that are not
// the project's own: test data, vendored code or hidden tool state.
func skipDir(name string) bool {
	return name == "testdata" || name == "vendor" || name == "node_modules" || strings.HasPrefix(name, ".")
}

func isMarkdown(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".md", ".markdown":
		return true
	}
	return false
}

// cost returns the budget that formatting path takes: its size up to the input
// limit, and no less than a share of the limit for each CPU, which bounds the
// number of files at once.
func cost(path string) int64 {
	size := int64(format.MaxInput)
	if info, err := os.Stat(path); err == nil && path != "-" {
		size = min(info.Size(), size)
	}
	return max(size, int64(format.MaxInput/runtime.NumCPU()))
}

// budget is a count of bytes that goroutines take and give back.
type budget struct {
	mu   sync.Mutex
	cond sync.Cond
	free int64
}

func newBudget(n int64) *budget {
	b := &budget{free: n}
	b.cond.L = &b.mu
	return b
}

// acquire takes n bytes, and waits until they are free.
func (b *budget) acquire(n int64) {
	b.mu.Lock()
	for b.free < n {
		b.cond.Wait()
	}
	b.free -= n
	b.mu.Unlock()
}

func (b *budget) release(n int64) {
	b.mu.Lock()
	b.free += n
	b.mu.Unlock()
	b.cond.Broadcast()
}

// run formats one input and reports whether formatting changed it.
func run(path string, check bool) (bool, error) {
	var src []byte
	var err error
	if path == "-" {
		src, err = io.ReadAll(os.Stdin)
	} else {
		src, err = os.ReadFile(path)
	}
	if err != nil {
		return false, err
	}

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
