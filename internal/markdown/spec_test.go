package markdown

import (
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
)

type example struct {
	id       int // ordinal in the file
	section  string
	markdown string
	html     string
}

type corpus struct {
	name     string
	path     string
	examples int
}

var corpora = []corpus{
	{"commonmark", "testdata/commonmark/spec.txt", 652},
}

func TestReadExamples(t *testing.T) {
	t.Parallel()

	for _, c := range corpora {
		t.Run("reads "+strconv.Itoa(c.examples)+" "+c.name+" examples", func(t *testing.T) {
			t.Parallel()

			if got := len(readExamples(t, c.path)); got != c.examples {
				t.Fatalf("readExamples(%q) gives %d examples, want %d", c.path, got, c.examples)
			}
		})
	}

	t.Run("reads an example's section, markdown and html", func(t *testing.T) {
		t.Parallel()

		want := example{
			id:       1,
			section:  "Tabs",
			markdown: "\tfoo\tbaz\t\tbim\n",
			html:     "<pre><code>foo\tbaz\t\tbim\n</code></pre>\n",
		}
		if got := readExamples(t, "testdata/commonmark/spec.txt"); len(got) == 0 || got[0] != want {
			t.Fatalf("first example = %+v, want %+v", got, want)
		}
	})
}

// readExamples reads the examples of a file in the spec.txt format, as
// cmark-gfm's spec_tests.py does. An example is an opening fence line, the
// Markdown, a "." line, the HTML, and a closing fence line. An example whose
// opening fence says "disabled" is skipped, but keeps its ordinal. A "→" in
// an example is a tab.
func readExamples(t testing.TB, path string) []example {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	const fence = "````````````````````````````````"
	const opener = fence + " example"
	var (
		examples     []example
		section      string
		id           int
		disabled     bool
		state        int // 0 text, 1 Markdown, 2 HTML
		markdown, ht strings.Builder
	)
	for line := range strings.Lines(string(data)) {
		switch l := strings.TrimSpace(line); {
		case strings.HasPrefix(l, opener):
			state = 1
			disabled = slices.Contains(strings.Fields(l[len(opener):]), "disabled")
		case l == fence:
			state = 0
			id++
			if !disabled {
				examples = append(examples, example{
					id:       id,
					section:  section,
					markdown: strings.ReplaceAll(markdown.String(), "→", "\t"),
					html:     strings.ReplaceAll(ht.String(), "→", "\t"),
				})
			}
			markdown.Reset()
			ht.Reset()
		case l == ".":
			state = 2
		case state == 1:
			markdown.WriteString(line)
		case state == 2:
			ht.WriteString(line)
		case state == 0 && strings.HasPrefix(line, "#") && strings.HasPrefix(strings.TrimLeft(line, "#"), " "):
			section = strings.TrimSpace(strings.TrimLeft(line, "#"))
		}
	}
	return examples
}
