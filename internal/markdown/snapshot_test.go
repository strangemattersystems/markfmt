package markdown_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/strangemattersystems/markfmt/internal/format"
	"github.com/strangemattersystems/markfmt/internal/markdown"
)

// parseSnapshot records the tree of every input of markdown.SnapshotInputs
// as kinds, flags and byte ranges, so that a change of structure shows in
// review. It
// holds no node layout, so a change of representation leaves it as it is.
const parseSnapshot = "testdata/snapshot/parse.txt"

// formatSnapshot records the output of format.Source for every input of
// markdown.SnapshotInputs, so that a change of output shows in review.
const formatSnapshot = "testdata/snapshot/format.txt"

func TestFormatSnapshot(t *testing.T) {
	t.Parallel()

	var b strings.Builder
	for _, in := range markdown.SnapshotInputs(t) {
		out, err := format.Source(in.Src)
		record := strconv.Quote(string(out))
		if err != nil {
			record = "error " + strconv.Quote(err.Error())
		}
		fmt.Fprintf(&b, "%s\t%s\n", in.Key, record)
	}
	compareSnapshot(t, formatSnapshot, b.String())
}

// compareSnapshot fails the test when got differs from the file at path. With
// MARKFMT_UPDATE=1 it writes the file instead.
func compareSnapshot(t *testing.T, path, got string) {
	t.Helper()

	if os.Getenv("MARKFMT_UPDATE") == "1" {
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o600); err != nil {
			t.Fatal(err)
		}
		t.Logf("wrote %s", path)
		return
	}

	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v; run task snapshot to write it", err)
	}
	if got != string(want) {
		t.Fatalf("the record of %s in %s differs; run task snapshot to update it, and review the diff", firstDiff(string(want), got), path)
	}
}

func TestParseSnapshot(t *testing.T) {
	t.Parallel()

	var b strings.Builder
	for _, in := range markdown.SnapshotInputs(t) {
		fmt.Fprintf(&b, "%s\t%s\n", in.Key, dumpTree(markdown.Parse(in.Src)))
	}
	compareSnapshot(t, parseSnapshot, b.String())
}

// dumpTree returns the nodes of tree on one line: the kind of each node, with
// its flags after a slash when it has any, the byte range of each leaf, and
// parentheses around the children of an interior node.
func dumpTree(tree *markdown.Tree) string {
	var b strings.Builder
	c := tree.Walk()
	for e, ok := c.Next(); ok; e, ok = c.Next() {
		if e.Exit {
			b.WriteString(")")
			continue
		}
		fmt.Fprintf(&b, " %v", tree.Kind(e.ID))
		if flags := markdown.NodeFlags(tree, e.ID); flags != 0 {
			fmt.Fprintf(&b, "/%#x", flags)
		}
		if markdown.LeafNode(tree, e.ID) {
			start, end := markdown.NodeRange(tree, e.ID)
			fmt.Fprintf(&b, "[%d:%d]", start, end)
		} else {
			b.WriteString("(")
		}
	}
	return strings.TrimSpace(b.String())
}

// firstDiff returns the key of the first record of got that want does not
// hold, for a failure message.
func firstDiff(want, got string) string {
	w := strings.Split(want, "\n")
	for i, line := range strings.Split(got, "\n") {
		if i >= len(w) || w[i] != line {
			key, _, _ := strings.Cut(line, "\t")
			if key == "" && i < len(w) {
				key, _, _ = strings.Cut(w[i], "\t")
			}
			return strconv.Quote(key)
		}
	}
	return "no record"
}
