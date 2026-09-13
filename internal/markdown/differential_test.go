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
	goldmarkRenderer = html.New(html.WithUnsafe(), html.WithXHTML())
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
		{"writes void elements as xhtml, as the test renderer does", "a\\\nb", "<p>a<br />\nb</p>\n"},
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
		{"skips a thematic break that starts like a list item after an empty list item", "*\n  - --", "goldmark deviates, spec section 5.2: a list item that starts with a blank line takes a line indented to its content that starts like a bullet list item"},
		{"skips an open parenthesis after a nul in a destination", "[0]:0\x00(", "goldmark deviates, spec sections 4.7 and 6.3: a parenthesis in a destination is escaped or in a balanced pair"},
		{"skips a tab before a nested list marker after a list item prefix", "0) 0\n   \t* 0", "goldmark deviates, spec sections 2.2 and 5.2: a tab after the prefix of a list item line stops at a column counted from the start of the line"},
		{"skips a control character in an unquoted value after whitespace", "<A A0000= \x01>0", "goldmark deviates, spec section 6.6: an unquoted attribute value takes ASCII control characters"},
		{"skips a paragraph after an empty nested list item that follows a paragraph", "0) 0\n\n   0)\n\n   0", "goldmark deviates, spec section 5.2: a blank line after an empty nested list item continues the outer list item"},
		{"skips a tab and text after a kind 6 tag name", "</td\t0", "goldmark deviates, spec section 4.6: a tab after the tag name starts HTML block kind 6"},
		{"skips a definition whose label spans lines and whose title fails", "[0\n]:0\n\"\"0", "goldmark deviates, spec section 4.7: a definition over several lines ends at its destination when the next line is not a title"},
		{"skips a code block of one blank line at the end of a list item", "- 00000\n  ```\n\n-", "goldmark deviates, spec section 5.3: a blank line at the end of a code block in a list item leaves the list tight"},
		{"skips a paragraph after lists that end in an empty item two levels down", "* 0)   *  \n\n  0", "goldmark deviates, spec section 5.2: a blank line after an empty nested list item continues the outer list item"},
		{"skips an open parenthesis before a backslash and a space", "[]((\\ )", "goldmark deviates, spec sections 4.7 and 6.3: a parenthesis in a destination is escaped or in a balanced pair"},
		{"skips a control character in an unquoted value after a quoted value with >", "<A A='>' A=\x15>", "goldmark deviates, spec section 6.6: an unquoted attribute value takes ASCII control characters"},
		{"skips a single dash after definitions alone", "[0]:0\n-", "goldmark deviates, spec sections 4.3 and 4.7: a setext underline after link reference definitions alone is paragraph text"},
		{"skips a nested list item after an empty one and a blank line", "* -\n \n  -", "goldmark deviates, spec section 5.2: a blank line after an empty nested list item continues the outer list item"},
		{"skips an escape after punctuation on the line after a backslash and a hard break", "\\  \n*\\!", "goldmark deviates, spec sections 2.4 and 6.7: a backslash escape after a backslash and a hard line break of spaces decodes"},
		{"skips a setext heading of a dash after definitions alone", "[0]:0\n-\n-", "goldmark deviates, spec sections 4.3 and 4.7: a setext underline after link reference definitions alone is paragraph text"},
		{"skips raw html in an image description", "![<A>]()", "goldmark deviates, spec section 6.4: the alt text of an image is the plain text of its description, as cmark writes it"},
		{"skips a form feed after a line ending in an html tag", "<A\n\f>", "goldmark deviates, spec section 6.6: FF is whitespace in an HTML tag, as cmark reads it"},
		{"skips a form feed after the tag of an html block of kind 7", "<A>\f", "goldmark deviates, spec section 4.6: a tab or FF after the tag that starts HTML block kind 7 is whitespace"},
		{"skips a link after open brackets split by text", strings.Repeat("[", 500) + "dddd" + strings.Repeat("[", 496) + "a](b)", "goldmark deviates, spec section 6.3: a link forms after any number of open brackets"},
		{"skips a setext heading after a definition over several lines", "[0]:\n0\n''0\n-", "goldmark deviates, spec section 4.7: a definition over several lines ends at its destination when the next line is not a title"},
		{"skips a link after open brackets that span 1000 bytes with nul as u+fffd", strings.Repeat("[", 497) + "\x00\x00\x00" + strings.Repeat("[", 496) + "a](b)", "goldmark deviates, spec section 6.3: a link forms after any number of open brackets"},
		{"skips an escape after a line of punctuation after a backslash and a hard break", "\\  \n*\n\\!", "goldmark deviates, spec sections 2.4 and 6.7: a backslash escape after a backslash and a hard line break of spaces decodes"},
		{"skips a tab before the end of a tag after a quoted >", "<A A=\">\"\t>", "goldmark deviates, spec section 6.6: a tab before the `>` of the tag that starts an HTML block is whitespace"},
		{"skips a tab after a kind 7 tag with a quoted >", "<A A=\">\">\t", "goldmark deviates, spec section 4.6: a tab or FF after the tag that starts HTML block kind 7 is whitespace"},
		{"skips a form feed in a tag after a quoted >", "z <j k=\">\"\f>", "goldmark deviates, spec section 6.6: FF is whitespace in an HTML tag, as cmark reads it"},
		{"skips a blank line of an html block in a list item", "*\n\t<!A\n\t", "goldmark deviates, spec sections 4.4, 4.6 and 5.2: a blank line of indented code or of an HTML block in a list item keeps the spaces beyond the indentation"},
		{"skips an escape after a backslash hard break after a backslash and a hard break", "\\  \n\\\n\\!", "goldmark deviates, spec sections 2.4 and 6.7: a backslash escape after a backslash and a hard line break of spaces decodes"},
		{"skips a paragraph after a block quote that ends with an empty list item", "* >+\n  >\n  0", "goldmark deviates, spec section 5.2: a blank line after an empty nested list item continues the outer list item"},
		{"skips a setext underline after a block quote marker space and a tab", ">00\n> \t=", "goldmark deviates, spec sections 2.2 and 5.2: a tab after the space of a block quote marker stops at a column counted from the start of the line"},
		{"skips a setext heading after a definition whose title fails", "[0]:0\n\"\"[0]:0\n-", "goldmark deviates, spec section 4.7: a title that other characters follow on its line is not the title of the definition"},
		{"skips an escape after a code span after a backslash and a hard break", "\\  \n``0``\\!", "goldmark deviates, spec sections 2.4 and 6.7: a backslash escape after a backslash and a hard line break of spaces decodes"},
		{"skips a tab in the indentation of an html block after a list item prefix", "*\n  \t<!A", "goldmark deviates, spec sections 2.2 and 5.2: a tab after the prefix of a list item line stops at a column counted from the start of the line"},
		{"skips a code block that ends blank in a loose list in a tight list", "* * 0\n\n    ```\n\n  0", "goldmark deviates, spec section 5.3: a blank line at the end of a code block in a list item leaves the list tight"},
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
		// Only this rule gives a paragraph or a setext heading whose first
		// line is "-", which goldmark reads as an empty list item, or a
		// thematic break of "-".
		for i, n := range t.nodes {
			if n.kind != Paragraph && n.kind != Heading {
				continue
			}
			line := t.Raw(NodeID(i))
			if end := bytes.IndexAny(line, "\r\n"); end >= 0 {
				line = line[:end]
			}
			if line = bytes.Trim(line, " \t"); (len(line) == 1 || len(line) >= 3) && len(bytes.Trim(line, "-")) == 0 {
				return true
			}
		}
		return false
	}},
	{"goldmark deviates, spec section 5.3: a blank line at the end of a code block in a list item leaves the list tight", func(t *Tree) bool {
		// endsBlank reports whether the last line of a block value is blank.
		endsBlank := func(v []byte) bool {
			return string(v) == "\n" || bytes.HasSuffix(v, []byte("\n\n"))
		}
		var lists []NodeID // the open lists, innermost last
		c := t.Walk()
		for e, ok := c.Next(); ok; e, ok = c.Next() {
			switch k := t.Kind(e.ID); {
			case k == List && e.Exit:
				lists = lists[:len(lists)-1]
			case k == List:
				lists = append(lists, e.ID)
			case e.Exit || !slices.ContainsFunc(lists, func(l NodeID) bool { return !t.ListLoose(l) }):
				// goldmark can make any enclosing tight list loose.
			case k == CodeBlock && endsBlank(t.AppendCode(nil, e.ID)), k == HTMLBlock && endsBlank(t.AppendHTML(nil, e.ID)):
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
		// close before a space, a tab, a line ending, VT or FF (design 8.4).
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
			for ; j < len(src) && strings.IndexByte(" \t\n\r\v\f", src[j]) < 0 && depth >= 0; j++ {
				switch src[j] {
				case '\\':
					if j+1 < len(src) && isASCIIPunct(src[j+1]) {
						j++
					}
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
				// A tab before or after a nested marker is in the marker leaf.
				j := n.start
				for j < n.end && (src[j] == ' ' || src[j] == '\t') {
					j++
				}
				if prefix && bytes.IndexByte(src[n.start:j], '\t') >= 0 {
					return true
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
			default:
				// An HTML block holds its indentation in its first leaf.
				if n.kind.class() != classStructure && prefix && lead {
					b := src[n.start:n.end]
					if bytes.IndexByte(b[:len(b)-len(bytes.TrimLeft(b, " \t"))], '\t') >= 0 {
						return true
					}
				}
				if n.kind.class() != classStructure && n.kind != Indent && n.kind != CodeIndent && n.kind != Whitespace {
					lead = false
				}
			}
		}
		return false
	}},
	{"goldmark deviates, spec section 5.2: a list item that starts with a blank line takes a line indented to its content that starts like a bullet list item", func(t *Tree) bool {
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
			if !blank || j == int(n.link) {
				continue
			}
			switch m := t.nodes[j]; m.kind {
			case List:
				return true
			case ThematicBreak:
				// goldmark reads "- - -" and "* * *" as a list item first.
				if run := bytes.TrimLeft(t.src[m.start:m.end], " \t"); len(run) > 1 && (run[0] == '-' || run[0] == '*') && (run[1] == ' ' || run[1] == '\t') {
					return true
				}
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
	{"goldmark deviates, spec section 5.2: a blank line after an empty nested list item continues the outer list item", func(t *Tree) bool {
		// firstChild returns the first structure node in [from, to), or -1.
		firstChild := func(from, to int) int {
			for j := from; j < to; j++ {
				if t.nodes[j].kind.class() == classStructure {
					return j
				}
			}
			return -1
		}
		// lastChild returns the last structure child of node id, or -1.
		lastChild := func(id int) int {
			last := -1
			for j := id + 1; j < int(t.nodes[id].link); {
				if m := t.nodes[j]; m.kind.class() == classStructure {
					last = j
					j = int(m.link)
				} else {
					j++
				}
			}
			return last
		}
		// endsEmpty reports whether node id, a list item, a list or a block
		// quote, ends with an empty list item through its last children.
		endsEmpty := func(id int) bool {
			for {
				switch t.nodes[id].kind {
				case ListItem:
					child := lastChild(id)
					if child < 0 {
						return true
					}
					id = child
				case List, BlockQuote:
					if id = lastChild(id); id < 0 {
						return false
					}
				default:
					return false
				}
			}
		}
		for i, n := range t.nodes {
			if n.kind != ListItem {
				continue
			}
			// An item of a child list that ends empty, with an item or a child
			// of the outer item after it, or a child block quote that ends
			// empty, with a child after it.
			for c := firstChild(i+1, int(n.link)); c >= 0; c = firstChild(int(t.nodes[c].link), int(n.link)) {
				later := firstChild(int(t.nodes[c].link), int(n.link)) >= 0
				switch t.nodes[c].kind {
				case List:
					for item := firstChild(c+1, int(t.nodes[c].link)); item >= 0; {
						next := firstChild(int(t.nodes[item].link), int(t.nodes[c].link))
						if (next >= 0 || later) && endsEmpty(item) {
							return true
						}
						item = next
					}
				case BlockQuote:
					if later && endsEmpty(c) {
						return true
					}
				}
			}
		}
		return false
	}},
	{"goldmark deviates, spec section 6.2: a delimiter run at the start of a line in a block quote follows a line ending", func(t *Tree) bool {
		return quoteMarkerDelimiter.Match(t.src)
	}},
	{"goldmark deviates, spec section 6.6: the `?` of `<?` does not start the `?>` of a processing instruction", func(t *Tree) bool {
		return emptyInstruction.Match(t.src)
	}},
	{"goldmark deviates, spec section 6.6: an unquoted attribute value takes ASCII control characters", func(t *Tree) bool {
		return unquotedControl.Match(t.src)
	}},
	{"goldmark deviates, spec section 6.6: a closing tag has no whitespace after `</`", func(t *Tree) bool {
		return spacedClosingTag.Match(t.src)
	}},
	{"goldmark deviates, spec section 4.6: a tab after the tag name starts HTML block kind 6", func(t *Tree) bool {
		return tabAfterTagName.Match(t.src)
	}},
	{"goldmark deviates, spec section 6.4: the alt text of an image is the plain text of its description, as cmark writes it", func(t *Tree) bool {
		for i, n := range t.nodes {
			if n.kind != Image {
				continue
			}
			for _, m := range t.nodes[i+1 : n.link] {
				switch m.kind {
				case SoftBreak, HardBreak, CodeSpan, Autolink, RawHTML:
					return true
				}
			}
		}
		return false
	}},
	{"goldmark deviates, spec section 4.7: a definition over several lines ends at its destination when the next line is not a title", func(t *Tree) bool {
		for i, n := range t.nodes {
			if n.kind != LinkReferenceDefinition {
				continue
			}
			var lineEnding, destination, title bool
			for _, m := range t.nodes[i+1 : n.link] {
				lineEnding = lineEnding || (m.kind == LineEnding || m.kind == VerbatimLineEnding) && !destination
				destination = destination || m.kind == Destination
				title = title || m.kind == Title
			}
			next := int(n.link)
			for next < len(t.nodes) && (t.nodes[next].kind == QuoteMarker || t.nodes[next].kind == ListMarker || t.nodes[next].kind == ItemIndent || t.nodes[next].kind == FootnoteIndent) {
				next++
			}
			if !lineEnding || !destination || title || next == len(t.nodes) || t.nodes[next].kind != Paragraph && t.nodes[next].kind != Heading {
				continue
			}
			if rest := bytes.TrimLeft(t.src[t.nodes[next].start:], " \t"); len(rest) > 0 && strings.IndexByte(`"'(`, rest[0]) >= 0 {
				return true
			}
		}
		return false
	}},
	{"goldmark deviates, spec section 4.7: a title that other characters follow on its line is not the title of the definition", func(t *Tree) bool {
		for i, n := range t.nodes {
			if n.kind != LinkReferenceDefinition || slices.ContainsFunc(t.nodes[i+1:n.link], func(m Node) bool { return m.kind == Title }) {
				continue
			}
			next := int(n.link)
			for next < len(t.nodes) && (t.nodes[next].kind == QuoteMarker || t.nodes[next].kind == ListMarker || t.nodes[next].kind == ItemIndent || t.nodes[next].kind == FootnoteIndent) {
				next++
			}
			if next == len(t.nodes) || t.nodes[next].kind != Paragraph && t.nodes[next].kind != Heading {
				continue
			}
			if rest := bytes.TrimLeft(t.src[t.nodes[next].start:], " \t"); len(rest) > 0 && strings.IndexByte(`"'(`, rest[0]) >= 0 {
				return true
			}
		}
		return false
	}},
	{"goldmark deviates, spec sections 4.7 and 6.3: a link label has at most 999 characters", func(t *Tree) bool {
		// A bracket text of more than 999 bytes.
		start := -1
		for i, c := range t.src {
			switch c {
			case '[':
				start = i
			case ']':
				if start >= 0 && i-start-1 > 999 {
					return true
				}
				start = -1
			}
		}
		return false
	}},
	{"goldmark deviates, spec section 5.3: a link reference definition in a list item does not make the list loose", func(t *Tree) bool {
		for i, n := range t.nodes {
			if n.kind != ListItem {
				continue
			}
			definition := false
			for j := i + 1; j < int(n.link); j++ {
				switch m := t.nodes[j]; m.kind {
				case LinkReferenceDefinition:
					definition = true
				case Paragraph:
					if definition {
						return true
					}
				}
				if m := t.nodes[j]; m.kind.class() == classStructure {
					j = int(m.link) - 1
				}
			}
		}
		return false
	}},
	{"goldmark deviates, spec sections 2.4 and 6.7: a backslash escape after a backslash and a hard line break of spaces decodes", func(t *Tree) bool {
		// goldmark writes an escape after such a break as it is in some
		// positions only, so every escape later in the block is skipped.
		for i := 1; i+1 < len(t.nodes); i++ {
			if t.nodes[i].kind != HardBreak || t.nodes[i-1].kind != Text || t.nodes[i+1].kind != HardBreakMarker ||
				!bytes.HasSuffix(t.Raw(NodeID(i-1)), []byte("\\")) || bytes.Contains(t.Raw(NodeID(i+1)), []byte("\\")) {
				continue
			}
			block := i - 1
			for block > 0 && (t.nodes[block].kind.class() != classStructure || int(t.nodes[block].link) <= i) {
				block--
			}
			if slices.ContainsFunc(t.nodes[t.nodes[i].link:t.nodes[block].link], func(m Node) bool { return m.kind == Escape }) {
				return true
			}
		}
		return false
	}},
	{"goldmark deviates, spec section 4.6: a tab or FF after the tag that starts HTML block kind 7 is whitespace", func(t *Tree) bool {
		return tabAfterTag.Match(t.src)
	}},
	{"goldmark deviates, spec section 6.3: a link forms after any number of open brackets", func(t *Tree) bool {
		// goldmark forms no link when about 1000 bytes of its input lie
		// between the first and the last open bracket of a block before it.
		// goldmark reads each NUL and invalid UTF-8 sequence as U+FFFD, 3
		// bytes.
		for i, n := range t.nodes {
			if n.kind != Paragraph && n.kind != Heading {
				continue
			}
			raw := t.Raw(NodeID(i))
			first, last := bytes.IndexByte(raw, '['), bytes.LastIndexByte(raw, '[')
			if first < 0 {
				continue
			}
			span := 0
			for b := raw[first:last]; len(b) > 0; {
				r, n := decodeRune(b)
				if span += n; r == utf8.RuneError {
					span += 3 - n
				}
				b = b[n:]
			}
			if span >= 999 {
				return true
			}
		}
		return false
	}},
	{"goldmark deviates, spec section 6.3: whitespace separates a link title from an angle destination", func(t *Tree) bool {
		return angleTitle.Match(t.src)
	}},
	{"goldmark deviates, spec section 6.6: a closing tag has no `/` before its `>`", func(t *Tree) bool {
		return slashClosingTag.Match(t.src)
	}},
	{"goldmark deviates, spec section 6.6: FF is whitespace in an HTML tag, as cmark reads it", func(t *Tree) bool {
		src := t.src
		for i := range src {
			j := i + 1
			if src[i] != '<' || j == len(src) {
				continue
			}
			if src[j] == '/' {
				j++
			}
			k := j
			for k < len(src) && (isASCIIAlphanumeric(src[k]) || src[k] == '-') {
				k++
			}
			if k == j || !isASCIILetter(src[j]) {
				continue
			}
			// goldmark takes FF right after a kind 1 tag name.
			if k < len(src) && src[k] == '\f' && slices.Contains(kind1Names, strings.ToLower(string(src[j:k]))) {
				continue
			}
			// Whitespace in a tag holds at most one line ending.
			lines := 0
		scan:
			for ; k < len(src) && src[k] != '<' && src[k] != '>'; k++ {
				switch src[k] {
				case '\f':
					return true
				case '"', '\'':
					// A quoted value can hold '>' and FF.
					if e := bytes.IndexByte(src[k+1:], src[k]); e >= 0 {
						k += e + 1
					}
				case '\r', '\n':
					if src[k] == '\r' && k+1 < len(src) && src[k+1] == '\n' {
						k++
					}
					if lines++; lines > 1 {
						break scan
					}
				}
			}
		}
		return false
	}},
	{"goldmark deviates, spec section 4.7: FF inside a link label is whitespace, and a label of FF alone is blank, as cmark reads them", func(t *Tree) bool {
		// A bracket text with FF inside its trimmed bytes, or with FF and
		// nothing but whitespace.
		start := -1
		for i, c := range t.src {
			switch c {
			case '[':
				start = i
			case ']':
				if start >= 0 {
					text := t.src[start+1 : i]
					trimmed := bytes.Trim(text, " \t\r\n\v\f")
					if bytes.IndexByte(trimmed, '\f') >= 0 || len(trimmed) == 0 && bytes.IndexByte(text, '\f') >= 0 {
						return true
					}
				}
				start = -1
			}
		}
		return false
	}},
	{"goldmark deviates, spec sections 2.2 and 5.2: a tab after the space of a block quote marker stops at a column counted from the start of the line", func(t *Tree) bool {
		for i, n := range t.nodes {
			if n.kind != QuoteMarker || !bytes.HasSuffix(t.Raw(NodeID(i)), []byte(" ")) {
				continue
			}
			// The next leaf on the line, whose indentation holds the tab.
			for j := i + 1; j < len(t.nodes); j++ {
				if m := t.nodes[j]; m.kind.class() != classStructure {
					lead := t.src[m.start:m.end]
					if bytes.IndexByte(lead[:len(lead)-len(bytes.TrimLeft(lead, " \t"))], '\t') >= 0 {
						return true
					}
					break
				}
			}
		}
		return false
	}},
	{"goldmark deviates, spec sections 4.4, 4.6 and 5.2: a blank line of indented code or of an HTML block in a list item keeps the spaces beyond the indentation", func(t *Tree) bool {
		items := 0 // the open list items
		c := t.Walk()
		for e, ok := c.Next(); ok; e, ok = c.Next() {
			switch n := t.nodes[e.ID]; {
			case n.kind == ListItem && e.Exit:
				items--
			case n.kind == ListItem:
				items++
			case (n.kind == CodeBlock || n.kind == HTMLBlock) && !e.Exit && items > 0:
				value := t.AppendHTML(nil, e.ID)
				if n.kind == CodeBlock {
					if slices.ContainsFunc(t.nodes[e.ID+1:n.link], func(m Node) bool { return m.kind == FenceMarker }) {
						continue
					}
					value = t.AppendCode(nil, e.ID)
				}
				for line := range bytes.Lines(value) {
					if l := bytes.TrimSuffix(line, []byte("\n")); len(l) > 0 && len(bytes.Trim(l, " \t")) == 0 {
						return true
					}
				}
			}
		}
		return false
	}},
	{"goldmark deviates, spec section 6.5: the scheme of an absolute URI has at most 32 characters", func(t *Tree) bool {
		return longScheme.Match(t.src)
	}},
	{"goldmark deviates, spec section 6.6: a tab before the `>` of the tag that starts an HTML block is whitespace", func(t *Tree) bool {
		return tabBeforeTagEnd.Match(t.src)
	}},
}

var (
	lowercaseDeclaration = regexp.MustCompile(`<![a-z]`)
	metaTag              = regexp.MustCompile(`(?i)</?meta([ \t\r\n/>]|$)`)
	kind1ClosingTag      = regexp.MustCompile(`(?i)</(pre|script|style)`)
	angleDestinationLT   = regexp.MustCompile(`\](\(|:)[ \t\r\n]*<[^>\r\n]*<`)
	formFeedDestination  = regexp.MustCompile(`\](\(|:)[ \t\r\n]*[^ \t\r\n]*\f`)
	kind1SlashTag        = regexp.MustCompile(`(?i)<(pre|script|style|textarea)/`)
	quoteMarkerDelimiter = regexp.MustCompile(`>[*_]`)
	emptyInstruction     = regexp.MustCompile(`<\?>`)
	unquotedControl      = regexp.MustCompile(`<[A-Za-z](?:[^<>"']|"[^"]*"|'[^']*')*=[ \t\r\n]*[^ \t\r\n"'=<>\x60]*[\x01-\x08\x0b\x0c\x0e-\x1f\x7f]`)
	spacedClosingTag     = regexp.MustCompile(`</[ \t\r\n]+[A-Za-z]`)
	tabAfterTagName      = regexp.MustCompile(`</?[A-Za-z][A-Za-z0-9]*\t`)
	tabAfterTag          = regexp.MustCompile(`<[A-Za-z/](?:[^<>"'\r\n]|"[^"]*"|'[^']*')*>[ \t\f]*[\t\f][ \t\f]*(?:\r|\n|$)`)
	angleTitle           = regexp.MustCompile(`\]\([ \t\r\n]*<[^<>\r\n]*>["'(]`)
	slashClosingTag      = regexp.MustCompile(`</[A-Za-z][A-Za-z0-9-]*[ \t\r\n]*/`)
	longScheme           = regexp.MustCompile(`<[A-Za-z][A-Za-z0-9+.-]{32,}:`)
	tabBeforeTagEnd      = regexp.MustCompile(`<[/]?[A-Za-z](?:[^<>"'\r\n]|"[^"]*"|'[^']*')*\t[ \t]*/?>`)
)
