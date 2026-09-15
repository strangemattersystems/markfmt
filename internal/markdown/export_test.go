package markdown

import (
	"os"
	"path/filepath"
	"testing"
)

// RenderTestHTML returns the test HTML of tree, normalized, for the tests in
// package markdown_test.
func RenderTestHTML(tree *Tree) string {
	return normalizeHTML(renderHTML(tree, false))
}

// RaceEnabled reports whether the test binary has the race detector.
const RaceEnabled = raceEnabled

// MaxrssBytes returns the peak resident set size of the process that ps
// describes, in bytes, and whether the platform reports it.
func MaxrssBytes(ps *os.ProcessState) (int64, bool) {
	return maxrssBytes(ps)
}

// PathologicalInputs returns the builders of the inputs of design 6.8, by
// name. Each builds an input of about n bytes.
func PathologicalInputs() map[string]func(n int) []byte {
	inputs := make(map[string]func(n int) []byte, len(pathologicalInputs))
	for _, in := range pathologicalInputs {
		inputs[in.name] = in.build
	}
	return inputs
}

// Kept returns the kept syntax of tree (design 12).
func Kept(tree *Tree) []string {
	return kept(tree)
}

// CorpusInputs returns the Markdown of every example of every corpus, and of
// every file of the pair corpus.
func CorpusInputs(tb testing.TB) [][]byte {
	tb.Helper()

	var inputs [][]byte
	for _, c := range corpora {
		for _, ex := range readExamples(tb, c.path) {
			inputs = append(inputs, []byte(ex.markdown))
		}
	}
	pairs, err := filepath.Glob("testdata/pairs/*.md")
	if err != nil {
		tb.Fatal(err)
	}
	for _, path := range pairs {
		src, err := os.ReadFile(path)
		if err != nil {
			tb.Fatal(err)
		}
		inputs = append(inputs, src)
	}
	return inputs
}
