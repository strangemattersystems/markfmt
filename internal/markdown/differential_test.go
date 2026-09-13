package markdown

import (
	"bytes"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

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
	for _, c := range corpora {
		for _, ex := range readExamples(f, c.path) {
			f.Add([]byte(ex.markdown))
		}
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
		{"reads invalid utf-8 as u+fffd", "[\xa6]: /u\n\n[\ufffd]", "<p><a href=\"/u\">\ufffd</a></p>\n"},
		{"reads nul as u+fffd", "[a](/\x00)", "<p><a href=\"/%EF%BF%BD\">a</a></p>\n"},
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
		{"skips a tab after a nested list marker that a split tab starts", "* 0\n\t* \t*", "goldmark deviates, spec sections 2.2 and 5.2: a tab after the prefix of a list item line stops at a column counted from the start of the line"},
		{"skips a tab in the indentation after a list item prefix", "* 0\n  \t -", "goldmark deviates, spec sections 2.2 and 5.2: a tab after the prefix of a list item line stops at a column counted from the start of the line"},
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
// line endings, a final line ending, and U+FFFD for NUL and for each maximal
// invalid UTF-8 subsequence. CommonMark gives the line ending forms the same
// meaning and replaces NUL, and markfmt reads invalid UTF-8 as U+FFFD (design
// 8.4). goldmark does not.
func goldmarkHTML(t testing.TB, src []byte) string {
	t.Helper()

	src = bytes.ReplaceAll(bytes.ReplaceAll(src, []byte("\r\n"), []byte("\n")), []byte("\r"), []byte("\n"))
	if len(src) > 0 && src[len(src)-1] != '\n' {
		src = append(src, '\n')
	}
	if !utf8.Valid(src) || bytes.IndexByte(src, 0) >= 0 {
		var valid []byte
		for b := src; len(b) > 0; {
			r, n := decodeRune(b)
			if r == utf8.RuneError {
				valid = append(valid, "\ufffd"...)
			} else {
				valid = append(valid, b[:n]...)
			}
			b = b[n:]
		}
		src = valid
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
	{"goldmark deviates, spec section 6.6: an attribute value can contain U+FFFD", func(t *Tree) bool {
		for i, n := range t.nodes {
			if n.kind != RawHTML {
				continue
			}
			if v := t.AppendRawHTML(nil, NodeID(i)); len(v) > 1 && isASCIILetter(v[1]) && bytes.Contains(v, replacement) {
				return true
			}
		}
		return false
	}},
	{"goldmark deviates, spec sections 2.1 and 6.2: a vertical tab, U+0085, U+2028 and U+2029 are not Unicode whitespace", func(t *Tree) bool {
		return bytes.ContainsFunc(t.src, func(r rune) bool {
			return unicode.IsSpace(r) && !isUnicodeSpace(r)
		})
	}},
	{"goldmark deviates, spec sections 4.7 and 6.3: a parenthesis in a destination is escaped or in a balanced pair", func(t *Tree) bool {
		// A destination after "](" or "]:" whose open parentheses do not all
		// close before a space or a line ending.
		src := t.src
		for i := 0; i+1 < len(src); i++ {
			if src[i] != ']' || src[i+1] != '(' && src[i+1] != ':' {
				continue
			}
			j := i + 2
			for j < len(src) && (src[j] == ' ' || src[j] == '\t' || src[j] == '\n' || src[j] == '\r') {
				j++
			}
			depth := 0
			for ; j < len(src) && src[j] > ' ' && depth >= 0; j++ {
				switch src[j] {
				case '\\':
					j++
				case '(':
					depth++
				case ')':
					depth--
				}
			}
			if depth > 0 {
				return true
			}
		}
		return false
	}},
	{"goldmark deviates, spec section 4.5: a content line of spaces in fenced code loses the fence and container indentation only", func(t *Tree) bool {
		for i, n := range t.nodes {
			if n.kind != CodeBlock {
				continue
			}
			leaves := t.nodes[i+1 : n.link]
			if !slices.ContainsFunc(leaves, func(m Node) bool { return m.kind == FenceMarker }) {
				continue
			}
			// A content line has only spaces and tabs when its CodeIndent and
			// CodeText leaves have bytes and no other byte.
			spaces, other := false, false
			for _, m := range leaves {
				switch m.kind {
				case CodeIndent:
					spaces = true
				case CodeText:
					spaces = true
					other = other || len(bytes.Trim(t.src[m.start:m.end], " \t")) > 0
				case VerbatimLineEnding, LineEnding, FenceMarker:
					if m.kind == VerbatimLineEnding && spaces && !other {
						return true
					}
					spaces, other = false, false
				}
			}
			if spaces && !other {
				return true
			}
		}
		return false
	}},
	{"goldmark deviates, spec sections 2.2 and 5.2: a tab after the prefix of a list item line stops at a column counted from the start of the line", func(t *Tree) bool {
		src := t.src
		containers := 0             // the open list items and block quotes
		prefix, lead := false, true // the line has a list item prefix leaf, and only indentation after it
		c := t.Walk()
		for e, ok := c.Next(); ok; e, ok = c.Next() {
			n := t.nodes[e.ID]
			switch n.kind {
			case ListItem, BlockQuote:
				if e.Exit {
					containers--
				} else {
					containers++
				}
				continue
			case LineEnding, VerbatimLineEnding, BlankLine:
				prefix, lead = false, true
			case ListMarker:
				// A tab after a nested marker is in the marker leaf.
				j := n.start
				for j < n.end && (src[j] == ' ' || src[j] == '\t') {
					j++
				}
				for j < n.end && '0' <= src[j] && src[j] <= '9' {
					j++
				}
				for j++; containers >= 2 && int(j) < len(src) && (src[j] == ' ' || src[j] == '\t'); j++ {
					if src[j] == '\t' {
						return true
					}
				}
				prefix = true
			case ItemIndent:
				prefix = true
			case Indent, CodeIndent, Whitespace:
				if prefix && lead && bytes.IndexByte(src[n.start:n.end], '\t') >= 0 {
					return true
				}
			default:
				if n.kind.class() != classStructure {
					lead = false
				}
			}
		}
		return false
	}},
	{"goldmark deviates, spec section 5.2: a list item that starts with a blank line takes a bullet list item indented to its content", func(t *Tree) bool {
		for i, n := range t.nodes {
			if n.kind != ListItem {
				continue
			}
			blank := false
			j := i + 1
			for ; j < int(n.link); j++ {
				k := t.nodes[j].kind
				blank = blank || k == BlankLine
				if k != ListMarker && k != BlankLine && k != ItemIndent && k != QuoteMarker && k != FootnoteIndent {
					break
				}
			}
			if blank && j < int(n.link) && t.nodes[j].kind == List {
				return true
			}
		}
		return false
	}},
	{"goldmark deviates, spec sections 4.7 and 6.3: a form feed ends a destination", func(t *Tree) bool {
		return formFeedDestination.Match(t.src)
	}},
	{"goldmark deviates, spec section 4.6: HTML block kind 1 needs a space, a tab, `>` or the end of the line after the tag name", func(t *Tree) bool {
		return kind1SlashTag.Match(t.src)
	}},
	{"goldmark deviates, spec section 4.5: an info string loses VT and FF at its ends, as cmark trims it", func(t *Tree) bool {
		for i, n := range t.nodes {
			if n.kind != CodeBlock {
				continue
			}
			line := t.Raw(NodeID(i))
			if end := bytes.IndexAny(line, "\r\n"); end >= 0 {
				line = line[:end]
			}
			if bytes.ContainsAny(line, "\v\f") {
				return true
			}
		}
		return false
	}},
	{"goldmark deviates, spec section 4.5: the first word of an info string ends at a tab", func(t *Tree) bool {
		for i, n := range t.nodes {
			if n.kind != CodeBlock {
				continue
			}
			word := t.AppendInfo(nil, NodeID(i))
			if end := bytes.IndexByte(word, ' '); end >= 0 {
				word = word[:end]
			}
			if bytes.ContainsAny(word, "\t\n\r\v\f") {
				return true
			}
		}
		return false
	}},
	{"goldmark deviates, spec section 4.5: an info string loses whitespace at its ends after its entity references decode, as cmark trims it", func(t *Tree) bool {
		for _, n := range t.nodes {
			if n.kind != InfoString {
				continue
			}
			r := t.newValueReader(n)
			if v := appendPieces(nil, &r); len(bytes.Trim(v, " \t\n\v\f\r")) != len(v) {
				return true
			}
		}
		return false
	}},
	{"goldmark deviates, spec section 5.2: a blank line indented to the content of an empty list item continues it", func(t *Tree) bool {
		// An empty item keeps a second blank line only when the line has
		// its indentation.
		for i, n := range t.nodes {
			if n.kind != ListItem {
				continue
			}
			blanks := 0
			for _, m := range t.nodes[i+1 : n.link] {
				if m.kind.class() == classStructure {
					break
				}
				if m.kind == BlankLine {
					blanks++
				}
			}
			if blanks >= 2 {
				return true
			}
		}
		return false
	}},
	{"goldmark deviates, spec sections 2.4 and 6.7: a backslash after an escaped backslash at the end of a line is a hard line break", func(t *Tree) bool {
		for i := 1; i+1 < len(t.nodes); i++ {
			if t.nodes[i].kind == HardBreak && t.nodes[i-1].kind == Escape && t.nodes[i+1].kind == HardBreakMarker &&
				string(t.Raw(NodeID(i-1))) == "\\\\" && string(t.Raw(NodeID(i+1))) == "\\" {
				return true
			}
		}
		return false
	}},
	{"goldmark deviates, spec section 4.7: a definition with its destination on a later line ends at the destination when the next line is not a title", func(t *Tree) bool {
		for i, n := range t.nodes {
			if n.kind != LinkReferenceDefinition {
				continue
			}
			var lineEnding, destination, title bool
			for _, m := range t.nodes[i+1 : n.link] {
				lineEnding = lineEnding || m.kind == LineEnding && !destination
				destination = destination || m.kind == Destination
				title = title || m.kind == Title
			}
			next := int(n.link)
			for next < len(t.nodes) && (t.nodes[next].kind == QuoteMarker || t.nodes[next].kind == ListMarker || t.nodes[next].kind == ItemIndent || t.nodes[next].kind == FootnoteIndent) {
				next++
			}
			if !lineEnding || !destination || title || next == len(t.nodes) || t.nodes[next].kind != Paragraph {
				continue
			}
			if rest := bytes.TrimLeft(t.src[t.nodes[next].start:], " \t"); len(rest) > 0 && strings.IndexByte(`"'(`, rest[0]) >= 0 {
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
	formFeedDestination  = regexp.MustCompile(`\](\(|:)[ \t\r\n]*[^ \t\r\n]*\f`)
	kind1SlashTag        = regexp.MustCompile(`(?i)<(pre|script|style|textarea)/`)
)
