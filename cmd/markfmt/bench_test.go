package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"testing"

	"github.com/strangemattersystems/markfmt"
)

// BenchmarkCheckDir walks a directory of small documents and checks each one,
// through the same walk, budget and goroutines as a --check run.
func BenchmarkCheckDir(b *testing.B) {
	const files = 200

	dir := b.TempDir()
	var bytes int64
	for i := range files {
		sub := filepath.Join(dir, "d"+strconv.Itoa(i%8))
		if err := os.MkdirAll(sub, 0o750); err != nil {
			b.Fatal(err)
		}
		doc := document(i)
		if err := os.WriteFile(filepath.Join(sub, "f"+strconv.Itoa(i)+".md"), doc, 0o600); err != nil {
			b.Fatal(err)
		}
		bytes += int64(len(doc))
	}

	b.ReportAllocs()
	b.SetBytes(bytes)
	for b.Loop() {
		checkDir(b, dir)
	}
}

func checkDir(b *testing.B, dir string) {
	b.Helper()

	budget := newBudget(markfmt.MaxInput)
	procs := runtime.GOMAXPROCS(0)
	var wg sync.WaitGroup
	for _, in := range inputs([]string{dir}, nil) {
		if in.err != nil {
			b.Fatal(in.err)
		}
		src, err := read(in.path)
		if err != nil {
			b.Fatal(err)
		}
		c := cost(len(src), procs)
		budget.acquire(c)
		wg.Go(func() {
			defer budget.release(c)
			if _, err := run(in.path, src, true); err != nil {
				b.Error(err)
			}
		})
	}
	wg.Wait()
}

func document(n int) []byte {
	s := strconv.Itoa(n)
	return []byte("# Document " + s + `

A paragraph with *emphasis*, ` + "`code`" + ` and a [link](https://example.com/` + s + `).

- item one
- item two with **strong** text
- item three

| name | count |
| --- | ---: |
| alpha | ` + s + ` |
| beta | 2 |

> A quote of document ` + s + `.

` + "```go\nfunc main() { println(" + s + ") }\n```" + `

1. first
2. second
`)
}
