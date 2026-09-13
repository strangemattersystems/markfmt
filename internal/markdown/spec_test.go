package markdown

import (
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

type example struct {
	id       int // ordinal in the file
	section  string
	info     string // the words after "example" on the opening fence
	markdown string
	html     string
}

type corpus struct {
	name      string
	path      string
	examples  int    // in the file
	sections  string // suffix of the names of the sections that run
	tagFilter bool   // upstream renders every example with the GFM tag filter
	gitHub    bool   // the expected HTML comes from the GitHub Markdown API
}

var corpora = []corpus{
	{"commonmark", "testdata/commonmark/spec.txt", 652, "", false, false},
	{"gfm", "testdata/gfm/spec.txt", 670, " (extension)", false, false},
	{"cmark-gfm-extensions", "testdata/cmark-gfm-extensions/extensions.txt", 30, "", true, false},
	{"cmark-gfm-regression", "testdata/cmark-gfm-regression/regression.txt", 26, "", false, false},
	{"commonmark-js-regression", "testdata/commonmark-js-regression/regression.txt", 32, "", false, false},
	{"markfmt", "testdata/markfmt/grammar.txt", 17, "", false, false},
	{"github", "testdata/github/github.txt", 71, "", true, true},
	{"differential", "testdata/differential/cases.txt", 40, "", false, false},
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

	t.Run("reads a ␀ in an example's markdown as nul", func(t *testing.T) {
		t.Parallel()

		const fence = "````````````````````````````````"
		path := filepath.Join(t.TempDir(), "spec.txt")
		if err := os.WriteFile(path, []byte(fence+" example\na␀→b\n.\n<p>␀</p>\n"+fence+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if got := readExamples(t, path); len(got) != 1 || got[0].markdown != "a\x00\tb\n" || got[0].html != "<p>␀</p>\n" {
			t.Fatalf("readExamples(%q) = %+v", path, got)
		}
	})

	t.Run("reads the words of an example's opening fence", func(t *testing.T) {
		t.Parallel()

		for _, ex := range readExamples(t, "testdata/gfm/spec.txt") {
			if ex.id == 198 && ex.info != "table" {
				t.Fatalf("info of gfm example 198 = %q, want %q", ex.info, "table")
			}
		}
	})
}

// readExamples reads the examples of a file in the spec.txt format, as
// cmark-gfm's spec_tests.py does. An example is an opening fence line, the
// Markdown, a "." line, the HTML, and a closing fence line. An example whose
// opening fence says "disabled" is skipped, but keeps its ordinal. A "→" in
// the Markdown of an example is a tab, and a "␀" is NUL.
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
		info         string
		disabled     bool
		state        int // 0 text, 1 Markdown, 2 HTML
		markdown, ht strings.Builder
	)
	for line := range strings.Lines(string(data)) {
		switch l := strings.TrimSpace(line); {
		case strings.HasPrefix(l, opener):
			state = 1
			info = strings.TrimSpace(l[len(opener):])
			disabled = slices.Contains(strings.Fields(info), "disabled")
		case l == fence:
			state = 0
			id++
			if !disabled {
				examples = append(examples, example{
					id:       id,
					section:  section,
					info:     info,
					markdown: strings.NewReplacer("→", "\t", "␀", "\x00").Replace(markdown.String()),
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
