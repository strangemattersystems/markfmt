package markdown_test

import (
	"bytes"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/strangemattersystems/markfmt"
	"github.com/strangemattersystems/markfmt/internal/format"
	"github.com/strangemattersystems/markfmt/internal/markdown"
)

// TestFormatSource does not call t.Parallel: its pathological and long
// subtests measure time and memory.
func TestFormatSource(t *testing.T) {
	t.Run("keeps each corpus output within 3 times the size of its input", func(t *testing.T) {
		t.Parallel()

		inputs := markdown.CorpusInputs(t)
		cases, err := filepath.Glob("../format/testdata/cases/*.in.md")
		if err != nil {
			t.Fatal(err)
		}
		for _, path := range cases {
			src, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			inputs = append(inputs, src)
		}
		largest, largestIn := 0.0, []byte(nil)
		for _, src := range inputs {
			out, err := format.Strict(src)
			if err != nil {
				t.Fatalf("Strict(%q) error: %v", src, err)
			}
			if ratio := float64(len(out)) / float64(max(len(src), 1)); ratio > largest {
				largest, largestIn = ratio, src
			}
		}
		t.Logf("largest output-to-input ratio: %.2f, for %q", largest, largestIn)
		if largest > 3 {
			t.Errorf("Source(%q) gives %.2f times its size, want at most 3", largestIn, largest)
		}
	})

	t.Run("pathological", func(t *testing.T) {
		if testing.Short() {
			t.Skip("the pathological subtests measure time, which takes about 20 s")
		}

		inputs := formatInputs()
		for _, name := range slices.Sorted(maps.Keys(inputs)) {
			if name == "empty" {
				continue
			}
			t.Run(name, func(t *testing.T) {
				// The 10n run takes at least 50 ms, so the n run is long enough to
				// time.
				n := 1000
				large := timeSource(inputs[name](10 * n))
				for large < 50*time.Millisecond && 10*n < markfmt.DefaultMaxInput {
					n *= 2
					large = timeSource(inputs[name](10 * n))
				}
				small := timeSource(inputs[name](n))
				if ratio := float64(large) / float64(max(small, 1)); ratio > 30 {
					t.Errorf("%d bytes take %v and %d bytes take %v: ratio %.0f, want at most 30", n, small, 10*n, large, ratio)
				}
			})
		}
	})

	t.Run("long", func(t *testing.T) {
		if spec := os.Getenv("MARKFMT_FORMAT_CHILD"); spec != "" {
			formatChild(t, spec)
			return
		}
		if markdown.RaceEnabled {
			t.Skip("the race detector makes times and memory unlike those of a normal build")
		}
		if os.Getenv("MARKFMT_LONG") != "1" {
			t.Skip("set MARKFMT_LONG=1 to run")
		}
		empty := runFormatChild(t, "empty", 0)
		names := slices.Sorted(maps.Keys(formatInputs()))
		for _, name := range names {
			if name == "empty" {
				continue
			}
			t.Run(name, func(t *testing.T) {
				small := runFormatChild(t, name, markfmt.DefaultMaxInput/10)
				large := runFormatChild(t, name, markfmt.DefaultMaxInput)
				t.Logf("%d bytes: %v; %d bytes: %v, %d bytes of output, %d bytes of memory, error %q",
					small.bytes, small.time, large.bytes, large.time, large.output, large.maxrss-empty.maxrss, large.err)
				if ratio := float64(large.time) / float64(max(small.time, 1)); ratio > 30 {
					t.Errorf("%d bytes take %v and %d bytes take %v: ratio %.0f, want at most 30", small.bytes, small.time, large.bytes, large.time, ratio)
				}
				if large.maxrss > 0 && empty.maxrss > 0 && large.maxrss-empty.maxrss > formatMemoryBound {
					t.Errorf("%d bytes take %d bytes of memory, want at most %d", large.bytes, large.maxrss-empty.maxrss, int64(formatMemoryBound))
				}
			})
		}
	})
}

// formatMemoryBound is the peak memory budget of format.Source at the input
// limit.
const formatMemoryBound = 4 << 30

// timeSource returns the best of 3 times of format.Source on src.
func timeSource(src []byte) time.Duration {
	best := time.Duration(1<<63 - 1)
	for range 3 {
		start := time.Now()
		_, _ = format.Source(src)
		best = min(best, time.Since(start))
	}
	return best
}

// formatInputs returns the inputs of the pathological and long subtests, by
// name: the pathological inputs of the parser; tab-indented code in list
// items, which has the largest output-to-input ratio of the corpora; and
// inputs that deep nesting makes pathological for the printer.
func formatInputs() map[string]func(n int) []byte {
	inputs := markdown.PathologicalInputs()
	inputs["empty"] = func(int) []byte { return nil }
	inputs["tab-indented code in list items"] = func(n int) []byte { return bytes.Repeat([]byte("-\t\tfoo\n"), n/7) }
	inputs["nested block quotes then lazy lines"] = func(n int) []byte {
		return []byte(strings.Repeat(">", n/2) + "a\n" + strings.Repeat("b\n", n/4))
	}
	inputs["nested list items then an HTML block of one long line"] = func(n int) []byte {
		return []byte(strings.Repeat("- ", n/4) + "a\n\n<div " + strings.Repeat("a", n/2))
	}
	inputs["escapes before emphasis"] = func(n int) []byte {
		return []byte(strings.Repeat("\\!", n/2) + "*a*")
	}
	inputs["nested block quotes then blank quote lines"] = func(n int) []byte {
		return []byte(strings.Repeat(">", n/2) + "a\n" + strings.Repeat(">\n", n/4))
	}
	inputs["nested strong emphasis"] = func(n int) []byte {
		return []byte(strings.Repeat("**", n/4) + "a" + strings.Repeat("**", n/4))
	}
	inputs["emphasis in nested emphasis"] = func(n int) []byte {
		return []byte(strings.Repeat("*a ", n/6) + strings.Repeat("*b* ", n/8) + strings.Repeat("a* ", n/6))
	}
	inputs["strikethrough in nested emphasis"] = func(n int) []byte {
		return []byte(strings.Repeat("*a ", n/6) + strings.Repeat("~~b~~ ", n/12) + strings.Repeat("a* ", n/6))
	}
	return inputs
}

type formatResult struct {
	time          time.Duration
	bytes, output int
	err           string
	maxrss        int64 // peak resident set size in bytes, or 0
}

// runFormatChild runs format.Source on the input called name at size bytes
// in a child process of the test binary.
func runFormatChild(t *testing.T, name string, size int) formatResult {
	t.Helper()

	cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestFormatSource$/^long$") //nolint:gosec // The command is the test binary itself.
	cmd.Env = append(os.Environ(), "MARKFMT_FORMAT_CHILD="+name+","+strconv.Itoa(size))
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("child for %s at %d bytes: %v\n%s", name, size, err, out)
	}
	for line := range strings.Lines(string(out)) {
		var r formatResult
		if _, err := fmt.Sscanf(line, "markfmt-format-long %d %d %d %q", &r.time, &r.bytes, &r.output, &r.err); err == nil {
			r.maxrss, _ = markdown.MaxrssBytes(cmd.ProcessState)
			return r
		}
	}
	t.Fatalf("child for %s at %d bytes gave no result:\n%s", name, size, out)
	return formatResult{}
}

// formatChild builds the input that spec names, "name,size", and prints the
// time of format.Source on it, the input and output sizes, and its error.
func formatChild(t *testing.T, spec string) {
	name, size, _ := strings.Cut(spec, ",")
	n, err := strconv.Atoi(size)
	if err != nil {
		t.Fatal(err)
	}
	build, ok := formatInputs()[name]
	if !ok {
		t.Fatalf("no input %q", name)
	}
	// A builder can pass n by a few bytes, and Source rejects an input above
	// the limit. A cut input can be a different input, so a smaller one is
	// built.
	src := build(n)
	for m := n; len(src) > n; {
		m -= len(src) - n
		src = build(m)
	}
	start := time.Now()
	out, err := format.Source(src)
	d := time.Since(start)
	msg := ""
	if err != nil {
		msg = err.Error()
	}
	fmt.Printf("markfmt-format-long %d %d %d %q\n", d, len(src), len(out), msg)
}

// FuzzSource checks what users run: format.Source keeps the bytes of a block
// that it cannot format, so it never fails below the input limit, never
// changes the test HTML or the kept syntax, and gives the same output again.
func FuzzSource(f *testing.F) {
	for _, src := range markdown.CorpusInputs(f) {
		f.Add(src)
	}
	f.Add([]byte("* 0\r--\n  |-"))

	f.Fuzz(func(t *testing.T, src []byte) {
		if len(src) > markfmt.DefaultMaxInput {
			return
		}
		out, err := format.Source(src)
		if err != nil {
			t.Fatalf("Source(%q) error: %v", src, err)
		}
		if in, got := markdown.RenderTestHTML(markdown.Parse(src)), markdown.RenderTestHTML(markdown.Parse(out)); in != got {
			t.Fatalf("Source(%q) = %q, whose test HTML differs:\n %q\n %q", src, out, in, got)
		}
		if in, got := markdown.Kept(markdown.Parse(src)), markdown.Kept(markdown.Parse(out)); !slices.Equal(in, got) {
			t.Fatalf("Source(%q) = %q, whose kept syntax differs:\n %q\n %q", src, out, in, got)
		}
		if again, err := format.Source(out); err != nil || !bytes.Equal(again, out) {
			t.Fatalf("Source is not idempotent\ninput: %q\n once: %q\ntwice: %q, %v", src, out, again, err)
		}
	})
}

// FuzzFormat checks the formatter against the test HTML, so that Equal is not
// its own oracle.
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
		out, err := format.Strict(src)
		if err != nil {
			t.Fatalf("Strict(%q) error: %v", src, err)
		}
		again, err := format.Strict(out)
		if err != nil {
			t.Fatalf("Strict(%q), the output of Strict(%q), error: %v", out, src, err)
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
