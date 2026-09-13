package markdown

import (
	"bytes"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/yuin/goldmark/v2/parser"
	"github.com/yuin/goldmark/v2/renderer/html"
)

var (
	goldmarkParser   = parser.New()
	goldmarkRenderer = html.New(html.WithUnsafe())
)

// FuzzDifferential compares the test HTML of [Parse] with the HTML of
// goldmark v2.0.2 with no extensions (design 11.5).
func FuzzDifferential(f *testing.F) {
	for _, ex := range readExamples(f, "testdata/commonmark/spec.txt") {
		f.Add([]byte(ex.markdown))
	}

	f.Fuzz(func(t *testing.T, src []byte) {
		tree := Parse(src)
		if reason := goldmarkDiffers(tree); reason != "" {
			t.Skip(reason)
		}
		if got, want := normalizeHTML(renderHTML(tree, false)), normalizeHTML(goldmarkHTML(t, src)); got != want {
			t.Fatalf("test HTML of Parse(%q)\n     got %q\ngoldmark %q", src, got, want)
		}
	})
}

func TestGoldmarkHTML(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  string
		want string
	}{
		{"reads cr and crlf as lf", "*\r\n\r- a", "<ul>\n<li></li>\n</ul>\n<ul>\n<li>a</li>\n</ul>\n"},
		{"reads a final line ending", "<div>\nx", "<div>\nx\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := goldmarkHTML(t, []byte(tt.src)); got != tt.want {
				t.Fatalf("goldmarkHTML(%q) = %q, want %q", tt.src, got, tt.want)
			}
		})
	}
}

func TestGoldmarkDiffers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  string
		want string
	}{
		{"gives no reason for commonmark", "*a* `~`\n~~~\nb\n~~~", ""},
		{"skips a table", "| a |\n| - |", "markfmt grammar: Table"},
		{"skips strikethrough", "~a~", "markfmt grammar: Strikethrough"},
		{"skips a tilde that makes emphasis delimiters text", "~a *b~~ c*", "markfmt grammar: ~"},
		{"skips a footnote definition", "[^a]: b", "markfmt grammar: FootnoteDefinition"},
		{"skips a footnote reference", "[^a]", "markfmt grammar: FootnoteReference"},
		{"skips front matter", "---\na: b\n---\n", "markfmt grammar: FrontMatter"},
		{"skips an extended autolink", "www.a.com <https://b.c>", "markfmt grammar: extended autolink"},
		{"skips a task list item", "- [ ] a", "markfmt grammar: task list item"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := goldmarkDiffers(Parse([]byte(tt.src))); got != tt.want {
				t.Fatalf("goldmarkDiffers of %q = %q, want %q", tt.src, got, tt.want)
			}
		})
	}

	for _, ex := range readExamples(t, "testdata/differential/cases.txt") {
		t.Run("checks the verdict of differential case "+strconv.Itoa(ex.id), func(t *testing.T) {
			t.Parallel()

			tree := Parse([]byte(ex.markdown))
			reason := goldmarkDiffers(tree)
			agrees := normalizeHTML(renderHTML(tree, false)) == normalizeHTML(goldmarkHTML(t, []byte(ex.markdown)))
			switch {
			case strings.HasPrefix(ex.section, "goldmark deviates"):
				if agrees {
					t.Errorf("goldmark agrees on %q: remove the case, its predicate and its roadmap row", ex.markdown)
				}
				if reason != ex.section {
					t.Errorf("goldmarkDiffers of %q = %q, want %q", ex.markdown, reason, ex.section)
				}
			case strings.HasPrefix(ex.section, "fixed in markfmt"):
				if !agrees || reason != "" {
					t.Errorf("goldmark disagrees on %q, or goldmarkDiffers gives the reason %q", ex.markdown, reason)
				}
			default:
				t.Errorf("section %q gives no verdict", ex.section)
			}
		})
	}
}

// goldmarkHTML returns goldmark's HTML for src. goldmark reads src with LF
// line endings and a final line ending: CommonMark gives both forms the same
// meaning, and goldmark does not.
func goldmarkHTML(t testing.TB, src []byte) string {
	t.Helper()

	src = bytes.ReplaceAll(bytes.ReplaceAll(src, []byte("\r\n"), []byte("\n")), []byte("\r"), []byte("\n"))
	if len(src) > 0 && src[len(src)-1] != '\n' {
		src = append(src, '\n')
	}
	var b bytes.Buffer
	if err := goldmarkRenderer.Render(&b, src, goldmarkParser.Parse(src)); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

// goldmarkDiffers returns the reason why goldmark's HTML for the source of
// tree can differ from its test HTML, or "" (design 11.5).
func goldmarkDiffers(tree *Tree) string {
	for i, n := range tree.nodes {
		switch id := NodeID(i); n.kind {
		case Table, Strikethrough, FootnoteDefinition, FootnoteReference, FrontMatter:
			return "markfmt grammar: " + n.kind.String()
		case Autolink:
			if !tree.AutolinkAngle(id) {
				return "markfmt grammar: extended autolink"
			}
		case ListItem:
			if task, _ := tree.ListItemTask(id); task {
				return "markfmt grammar: task list item"
			}
		case Text, Delimiter:
			// A ~ run can make emphasis delimiters text with no
			// Strikethrough node (design 6.4).
			if bytes.IndexByte(tree.Raw(id), '~') >= 0 {
				return "markfmt grammar: ~"
			}
		}
	}
	for _, d := range goldmarkDeviations {
		if d.match(tree) {
			return d.section
		}
	}
	return ""
}

// goldmarkDeviations are the rows of the roadmap's Known goldmark deviations
// that show in HTML, each with the section of its case in
// testdata/differential/cases.txt.
var goldmarkDeviations = []struct {
	section string
	match   func(*Tree) bool
}{
	{"goldmark deviates, spec sections 4.6 and 6.6: a declaration starts with `<!` and an ASCII letter", func(t *Tree) bool {
		return lowercaseDeclaration.Match(t.src)
	}},
	{"goldmark deviates, spec section 4.6: `meta` is not in the tag list of HTML block kind 6", func(t *Tree) bool {
		return metaTag.Match(t.src)
	}},
	{"goldmark deviates, spec section 4.6: a closing tag of `pre`, `script` or `style` starts HTML block kind 7", func(t *Tree) bool {
		return kind1ClosingTag.Match(t.src)
	}},
	{"goldmark deviates, spec sections 4.7 and 6.3: an angle destination contains no unescaped `<`", func(t *Tree) bool {
		return angleDestinationLT.Match(t.src)
	}},
	{"goldmark deviates, spec sections 4.3 and 4.7: a setext underline after link reference definitions alone is paragraph text", func(t *Tree) bool {
		// Only this rule gives a paragraph whose first line is a thematic
		// break of "-".
		for i, n := range t.nodes {
			if n.kind != Paragraph {
				continue
			}
			line := t.Raw(NodeID(i))
			if end := bytes.IndexAny(line, "\r\n"); end >= 0 {
				line = line[:end]
			}
			if line = bytes.Trim(line, " \t"); len(line) >= 3 && len(bytes.Trim(line, "-")) == 0 {
				return true
			}
		}
		return false
	}},
	{"goldmark deviates, spec section 5.3: a blank line at the end of a code block in a list item leaves the list tight", func(t *Tree) bool {
		var lists []NodeID // the open lists, innermost last
		c := t.Walk()
		for e, ok := c.Next(); ok; e, ok = c.Next() {
			switch k := t.Kind(e.ID); {
			case k == List && e.Exit:
				lists = lists[:len(lists)-1]
			case k == List:
				lists = append(lists, e.ID)
			case e.Exit || len(lists) == 0 || t.ListLoose(lists[len(lists)-1]):
			case k == CodeBlock && bytes.HasSuffix(t.AppendCode(nil, e.ID), []byte("\n\n")),
				k == HTMLBlock && bytes.HasSuffix(t.AppendHTML(nil, e.ID), []byte("\n\n")):
				return true
			}
		}
		return false
	}},
	{"goldmark deviates, spec section 4.5: the class of an info word that starts with `language-` gets no second prefix", func(t *Tree) bool {
		for i, n := range t.nodes {
			if n.kind == CodeBlock && bytes.HasPrefix(t.AppendInfo(nil, NodeID(i)), []byte("language-")) {
				return true
			}
		}
		return false
	}},
}

var (
	lowercaseDeclaration = regexp.MustCompile(`<![a-z]`)
	metaTag              = regexp.MustCompile(`(?i)</?meta([ \t\r\n/>]|$)`)
	kind1ClosingTag      = regexp.MustCompile(`(?i)</(pre|script|style)`)
	angleDestinationLT   = regexp.MustCompile(`\](\(|:)[ \t\r\n]*<[^>\r\n]*<`)
)
