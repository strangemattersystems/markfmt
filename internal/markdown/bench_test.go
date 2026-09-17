package markdown_test

import (
	"maps"
	"os"
	"slices"
	"testing"

	"github.com/strangemattersystems/markfmt/internal/markdown"
)

// BenchmarkEqual compares the tree of the frozen design document with a second
// tree of the same input, both parsed once.
func BenchmarkEqual(b *testing.B) {
	src, err := os.ReadFile("testdata/bench/design.md")
	if err != nil {
		b.Fatal(err)
	}
	a, c := markdown.Parse(src), markdown.Parse(src)
	b.ReportAllocs()
	b.SetBytes(int64(len(src)))
	for b.Loop() {
		if err := markdown.Equal(a, c); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkPathological parses each input of design 6.8 at one size, so a
// change of the linear-time mechanisms shows as a time, not as a failure.
func BenchmarkPathological(b *testing.B) {
	const n = 1 << 16

	inputs := markdown.PathologicalInputs()
	for _, name := range slices.Sorted(maps.Keys(inputs)) {
		src := inputs[name](n)
		b.Run("input="+name, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(src)))
			for b.Loop() {
				markdown.Parse(src)
			}
		})
	}
}
