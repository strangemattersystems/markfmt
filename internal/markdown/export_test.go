package markdown

import (
	"os"
	"path/filepath"
	"strconv"
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

// NodeFlags returns the flags of node id of t.
func NodeFlags(t *Tree, id NodeID) uint8 {
	return t.nodes[id].flags
}

// LeafNode reports whether node id of t is a leaf.
func LeafNode(t *Tree, id NodeID) bool {
	return t.nodes[id].kind.class() != classStructure
}

// NodeRange returns the input bytes that node id of t covers.
func NodeRange(t *Tree, id NodeID) (start, end int) {
	n := t.nodes[id]
	return int(n.start), int(n.end)
}

// Input is a test input, and the key that names it in a snapshot.
type Input struct {
	Key string
	Src []byte
}

// SnapshotInputs returns every corpus example, pair file and printer case,
// each with a key. A key holds while its file keeps the input, so a snapshot
// diff names what changed.
func SnapshotInputs(tb testing.TB) []Input {
	tb.Helper()

	var inputs []Input
	for _, c := range corpora {
		for _, ex := range readExamples(tb, c.path) {
			inputs = append(inputs, Input{c.name + "/" + strconv.Itoa(ex.id), []byte(ex.markdown)})
		}
	}
	for _, pattern := range []string{"testdata/pairs/*.md", "../format/testdata/cases/*.in.md"} {
		paths, err := filepath.Glob(pattern)
		if err != nil {
			tb.Fatal(err)
		}
		if len(paths) == 0 {
			tb.Fatalf("no files match %s", pattern)
		}
		for _, path := range paths {
			src, err := os.ReadFile(path)
			if err != nil {
				tb.Fatal(err)
			}
			inputs = append(inputs, Input{filepath.ToSlash(path), src})
		}
	}
	return inputs
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
