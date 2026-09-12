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
		want string
	}{
		{"gives an empty document for empty input", "", `Document{}`},
		{"gives a bom leaf", "\xEF\xBB\xBFa\n", `Document{BOM "\ufeff", Paragraph{Text "a", LineEnding "\n"}}`},
		{"joins lines into a paragraph", "a\r\n  b\rc", `Document{Paragraph{Text "a", LineEnding "\r\n", Indent "  ", Text "b", LineEnding "\r", Text "c"}}`},
		{"keeps trailing spaces in text", "a  \n", `Document{Paragraph{Text "a  ", LineEnding "\n"}}`},
		{"gives the indentation of a first line", "   a", `Document{Paragraph{Indent "   ", Text "a"}}`},
		{"ends a paragraph at a blank line", "a\n\nb", `Document{Paragraph{Text "a", LineEnding "\n"}, BlankLine "\n", Paragraph{Text "b"}}`},
		{"gives one leaf per blank line", "\n \t\r\n", `Document{BlankLine "\n", BlankLine " \t\r\n"}`},
		{"gives a blank line at the end of the input", "a\n  ", `Document{Paragraph{Text "a", LineEnding "\n"}, BlankLine "  "}`},
		{"gives a thematic break", " - - -\t\n", `Document{ThematicBreak{Indent " ", ThematicRun "- - -\t", LineEnding "\n"}}`},
		{"interrupts a paragraph with a thematic break", "a\n***\nb", `Document{Paragraph{Text "a", LineEnding "\n"}, ThematicBreak{ThematicRun "***", LineEnding "\n"}, Paragraph{Text "b"}}`},
		{"needs three markers of one kind", "**\n*-*", `Document{Paragraph{Text "**", LineEnding "\n", Text "*-*"}}`},
		{"gives an atx heading", "## a ##  \n", `Document{Heading{ATXMarker "##", Whitespace " ", Text "a", Whitespace " ", ATXClose "##", Whitespace "  ", LineEnding "\n"}}`},
		{"keeps a closing sequence that follows text", " #\ta#", `Document{Heading{Indent " ", ATXMarker "#", Whitespace "\t", Text "a#"}}`},
		{"gives an empty atx heading", "#\n### ###", `Document{Heading{ATXMarker "#", LineEnding "\n"}, Heading{ATXMarker "###", Whitespace " ", ATXClose "###"}}`},
		{"interrupts a paragraph with an atx heading", "a\n# b", `Document{Paragraph{Text "a", LineEnding "\n"}, Heading{ATXMarker "#", Whitespace " ", Text "b"}}`},
		{"needs one to six markers and a space", "####### a\n#a", `Document{Paragraph{Text "####### a", LineEnding "\n", Text "#a"}}`},
		{"needs a thematic break indented less than four columns", "  \t___", `Document{Paragraph{Indent "  \t", Text "___"}}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tree := Parse([]byte(tt.src))
			if err := tree.Verify(); err != nil {
				t.Fatalf("Parse(%q).Verify() = %v", tt.src, err)
			}
			if got := dump(tree); got != tt.want {
				t.Fatalf("Parse(%q)\n got %s\nwant %s", tt.src, got, tt.want)
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

// testConformance renders the examples of c's sections and compares them with
// the expected HTML, against the examples listed in failing.txt next to c's
// file. The list checks cover the whole corpus, whatever subtests -run
// selects.
func testConformance(t *testing.T, c corpus) {
	examples := slices.DeleteFunc(readExamples(t, c.path), func(ex example) bool {
		return !strings.HasSuffix(ex.section, c.sections)
	})
	failing := readFailing(t, filepath.Join(filepath.Dir(c.path), "failing.txt"), examples)

	got := make([]string, len(examples))
	want := make([]string, len(examples))
	pass := make([]bool, len(examples))
	var unlisted, passing []int
	for i, ex := range examples {
		tree := Parse([]byte(ex.markdown))
		if err := tree.Verify(); err != nil {
			t.Errorf("%s example %d: %v", c.name, ex.id, err)
		}
		got[i] = normalizeHTML(renderHTML(tree))
		want[i] = normalizeHTML(ex.html)
		// cmark-gfm counts an example whose expected HTML is <IGNORE> as passing:
		// it tests only that parsing does not crash.
		pass[i] = got[i] == want[i] || strings.TrimSpace(ex.html) == "<IGNORE>"
		switch {
		case !pass[i] && !failing[ex.id]:
			unlisted = append(unlisted, ex.id)
		case pass[i] && failing[ex.id]:
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

			switch {
			case !pass[i] && !failing[ex.id]:
				t.Errorf("fails and is not in failing.txt (section %s)\nmarkdown: %q\n     got: %q\n    want: %q",
					ex.section, ex.markdown, got[i], want[i])
			case pass[i] && failing[ex.id]:
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

func TestNeedsInlines(t *testing.T) {
	t.Parallel()

	t.Run("leaves 252 of 296 block-section commonmark examples to stage 2", func(t *testing.T) {
		t.Parallel()

		var block, blockOnly int
		for _, ex := range readExamples(t, "testdata/commonmark/spec.txt") {
			if slices.Contains(blockSections, ex.section) {
				block++
				if !needsInlines(ex) {
					blockOnly++
				}
			}
		}
		if block != 296 || blockOnly != 252 {
			t.Fatalf("%d of %d block-section examples do not need inlines, want 252 of 296", blockOnly, block)
		}
	})
}

// blockSections are the CommonMark spec sections about block structure.
var blockSections = []string{
	"Tabs", "Precedence", "Thematic breaks", "ATX headings", "Setext headings",
	"Indented code blocks", "Fenced code blocks", "HTML blocks",
	"Link reference definitions", "Paragraphs", "Blank lines", "Block quotes",
	"List items", "Lists",
}

// needsInlines reports whether a CommonMark example needs the inline phase to
// pass (design 11.2): its expected HTML, outside every <pre> element, has an
// inline element or a character reference other than &quot;, &amp;, &lt; and
// &gt;, or its Markdown has "\" or "&". Stage 2 passes every block-section
// example that does not. Delete it in the commit that passes the stage 3 gate.
func needsInlines(ex example) bool {
	if strings.ContainsAny(ex.markdown, `\&`) {
		return true
	}
	for html := ex.html; ; {
		outside, rest, found := strings.Cut(html, "<pre")
		if hasInlineHTML(outside) {
			return true
		}
		if !found {
			return false
		}
		if _, html, found = strings.Cut(rest, "</pre>"); !found {
			return false
		}
	}
}

// hasInlineHTML reports whether html has an element that the inline phase
// writes, or a character reference other than &quot;, &amp;, &lt; and &gt;.
func hasInlineHTML(html string) bool {
	for _, tag := range []string{"<em", "<strong", "<a", "<img", "<code", "<br"} {
		for rest, found := html, true; found; {
			_, rest, found = strings.Cut(rest, tag)
			if found && rest != "" && strings.IndexByte(" \t\n\r\f\v/>", rest[0]) >= 0 {
				return true
			}
		}
	}
	for rest, found := html, true; found; {
		_, rest, found = strings.Cut(rest, "&")
		name, _, semicolon := strings.Cut(rest, ";")
		if !found || !semicolon || slices.Contains([]string{"quot", "amp", "lt", "gt"}, name) {
			continue
		}
		digits := strings.TrimPrefix(name, "#")
		if digits != "" && strings.TrimFunc(digits, func(r rune) bool {
			return 'a' <= r|0x20 && r|0x20 <= 'z' || '0' <= r && r <= '9'
		}) == "" {
			return true
		}
	}
	return false
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

// dump returns tree as nested kinds with the bytes of each leaf, as in
// Document{Paragraph{Text "a", LineEnding "\n"}, BlankLine "\n"}.
func dump(tree *Tree) string {
	var b strings.Builder
	sep := ""
	c := tree.Walk()
	for e, ok := c.Next(); ok; e, ok = c.Next() {
		k := tree.Kind(e.ID)
		switch {
		case e.Exit:
			b.WriteString("}")
			sep = ", "
		case k.class() == classStructure:
			b.WriteString(sep + k.String() + "{")
			sep = ""
		default:
			b.WriteString(sep + k.String() + " " + strconv.Quote(string(tree.Raw(e.ID))))
			sep = ", "
		}
	}
	return b.String()
}
