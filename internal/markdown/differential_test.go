package markdown

import (
	"bytes"
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
	return ""
}
