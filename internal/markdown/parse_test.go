package markdown

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  string
		want []Node
	}{
		{"gives an empty document for empty input", "", []Node{
			{kind: Document, link: 1},
		}},
		{"gives line ending leaves", "a\r\n\nb", []Node{
			{kind: Document, start: 0, end: 5, link: 5},
			{kind: Text, start: 0, end: 1},
			{kind: LineEnding, start: 1, end: 3},
			{kind: LineEnding, start: 3, end: 4},
			{kind: Text, start: 4, end: 5},
		}},
		{"gives a bom leaf", "\xEF\xBB\xBFa\n", []Node{
			{kind: Document, start: 0, end: 5, link: 4},
			{kind: BOM, start: 0, end: 3},
			{kind: Text, start: 3, end: 4},
			{kind: LineEnding, start: 4, end: 5},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := Parse([]byte(tt.src)).nodes; !slices.Equal(got, tt.want) {
				t.Fatalf("Parse(%q) nodes = %+v\nwant %+v", tt.src, got, tt.want)
			}
		})
	}

	for _, c := range corpora {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			testConformance(t, c)
		})
	}
}

// testConformance renders every example of c and compares it with the
// expected HTML, against the examples listed in failing.txt next to c's file.
// The list checks cover the whole corpus, whatever subtests -run selects.
func testConformance(t *testing.T, c corpus) {
	examples := readExamples(t, c.path)
	failing := readFailing(t, filepath.Join(filepath.Dir(c.path), "failing.txt"), examples)

	got := make([]string, len(examples))
	want := make([]string, len(examples))
	var unlisted, passing []int
	for i, ex := range examples {
		tree := Parse([]byte(ex.markdown))
		if err := tree.Verify(); err != nil {
			t.Errorf("%s example %d: %v", c.name, ex.id, err)
		}
		got[i] = normalizeHTML(renderHTML(tree))
		want[i] = normalizeHTML(ex.html)
		switch pass := got[i] == want[i]; {
		case !pass && !failing[ex.id]:
			unlisted = append(unlisted, ex.id)
		case pass && failing[ex.id]:
			passing = append(passing, ex.id)
		}
	}
	if len(unlisted) > 0 {
		t.Errorf("%d examples fail and are not in failing.txt: %v", len(unlisted), unlisted)
	}
	if len(passing) > 0 {
		t.Errorf("%d examples in failing.txt pass: %v", len(passing), passing)
	}

	for i, ex := range examples {
		t.Run(c.name+" example "+strconv.Itoa(ex.id), func(t *testing.T) {
			t.Parallel()

			switch pass := got[i] == want[i]; {
			case !pass && !failing[ex.id]:
				t.Errorf("fails and is not in failing.txt (section %s)\nmarkdown: %q\n     got: %q\n    want: %q",
					ex.section, ex.markdown, got[i], want[i])
			case pass && failing[ex.id]:
				t.Error("passes: remove it from failing.txt")
			}
		})
	}
}

// readFailing reads the IDs in a failing.txt file: one example ID per line,
// in increasing order, with "#" comments. It reports an entry that is not an
// ID, that is a duplicate or out of order, or that names no example.
func readFailing(t *testing.T, path string, examples []example) map[int]bool {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	ids := make(map[int]bool, len(examples))
	for _, ex := range examples {
		ids[ex.id] = true
	}
	failing := make(map[int]bool)
	last, n := 0, 0
	for line := range strings.Lines(string(data)) {
		n++
		entry, _, _ := strings.Cut(line, "#")
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		id, err := strconv.Atoi(entry)
		switch {
		case err != nil:
			t.Errorf("%s:%d: %q is not an example ID", path, n, entry)
			continue
		case id <= last:
			t.Errorf("%s:%d: %d is a duplicate or out of order", path, n, id)
		case !ids[id]:
			t.Errorf("%s:%d: %d names no example", path, n, id)
		}
		failing[id] = true
		last = max(last, id)
	}
	return failing
}

func FuzzParse(f *testing.F) {
	for _, src := range []string{"", "a", "a\nb\r\nc\rd\r\r\n"} {
		f.Add([]byte(src))
	}

	f.Fuzz(func(t *testing.T, src []byte) {
		tree := Parse(src)
		if err := tree.Verify(); err != nil {
			t.Fatalf("Parse(%q).Verify() = %v", src, err)
		}
		var leaves []byte
		for _, n := range tree.nodes {
			if n.kind.class() != classStructure {
				leaves = append(leaves, src[n.start:n.end]...)
			}
		}
		if !bytes.Equal(leaves, src) {
			t.Fatalf("leaves of Parse(%q) = %q", src, leaves)
		}
	})
}
