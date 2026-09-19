package format

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/strangemattersystems/markfmt/internal/markdown"
)

type benchInput struct {
	name string
	docs [][]byte
}

func (in benchInput) size() int {
	n := 0
	for _, doc := range in.docs {
		n += len(doc)
	}
	return n
}

// benchInputs returns the benchmark inputs by name: the frozen design
// document, that document formatted, which is what a check of a formatted
// project reads, every printer case, and the frozen small document.
//
// The cases stay separate documents. Joined into one, they hold a block that
// fails the check, so the benchmark would measure the retries of [Source].
func benchInputs(b *testing.B) []benchInput {
	b.Helper()

	read := func(path string) []byte {
		src, err := os.ReadFile(path)
		if err != nil {
			b.Fatal(err)
		}
		return src
	}
	paths, err := filepath.Glob("testdata/cases/*.in.md")
	if err != nil {
		b.Fatal(err)
	}
	if len(paths) == 0 {
		b.Fatal("no files match testdata/cases/*.in.md")
	}
	cases := make([][]byte, 0, len(paths))
	for _, path := range paths {
		cases = append(cases, read(path))
	}
	design := read("../markdown/testdata/bench/design.md")
	formatted, err := Source(design)
	if err != nil {
		b.Fatal(err)
	}
	return []benchInput{
		{"design", [][]byte{design}},
		{"formatted", [][]byte{formatted}},
		{"cases", cases},
		{"small", [][]byte{read("../markdown/testdata/bench/small.md")}},
	}
}

// benchmarkBytes runs f once per iteration over an input of n bytes, and
// reports throughput and the bytes allocated per input byte.
func benchmarkBytes(b *testing.B, n int, f func()) {
	b.Helper()

	b.ReportAllocs()
	b.SetBytes(int64(n))
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	iters := 0
	for b.Loop() {
		f()
		iters++
	}
	// b.Loop stops the timer when it returns false, so this read is untimed.
	runtime.ReadMemStats(&after)
	b.ReportMetric(float64(after.TotalAlloc-before.TotalAlloc)/float64(iters)/float64(n), "alloc-bytes/byte")
}

// BenchmarkSource formats each input end to end: both parses, the printer and
// the runtime check.
func BenchmarkSource(b *testing.B) {
	for _, in := range benchInputs(b) {
		b.Run("input="+in.name, func(b *testing.B) {
			for _, doc := range in.docs {
				if _, err := Source(doc); err != nil {
					// task bench-compare runs these benchmarks on the base's
					// code, which can fail on a case that the head adds.
					b.Skipf("Source cannot format the input: %v", err)
				}
			}
			benchmarkBytes(b, in.size(), func() {
				for _, doc := range in.docs {
					if _, err := Source(doc); err != nil {
						b.Fatal(err)
					}
				}
			})
		})
	}
}

// BenchmarkPrinter times the printer alone, over trees parsed once.
func BenchmarkPrinter(b *testing.B) {
	for _, in := range benchInputs(b) {
		trees := make([]*markdown.Tree, len(in.docs))
		for i, doc := range in.docs {
			trees[i] = markdown.Parse(doc)
		}
		b.Run("input="+in.name, func(b *testing.B) {
			benchmarkBytes(b, in.size(), func() {
				for _, tree := range trees {
					p := printer{tree: tree, max: MaxOutput}
					p.document()
				}
			})
		})
	}
}
