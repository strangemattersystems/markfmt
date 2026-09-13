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
