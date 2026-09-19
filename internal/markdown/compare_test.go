package markdown

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestEqual(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("testdata/pairs/pairs.txt")
	if err != nil {
		t.Fatal(err)
	}
	listed := make(map[string]bool)
	for line := range strings.Lines(string(data)) {
		if strings.HasPrefix(line, "#") || strings.TrimSpace(line) == "" {
			continue
		}
		name, rest, _ := strings.Cut(strings.TrimSpace(line), " ")
		verdict, reason, _ := strings.Cut(rest, " ")
		listed[name] = true
		t.Run(strings.ReplaceAll(name, "-", " ")+" is "+verdict, func(t *testing.T) {
			t.Parallel()

			a, b := readPair(t, name)
			err := Equal(a, b)
			htmlA, htmlB := normalizeHTML(renderHTML(a, false)), normalizeHTML(renderHTML(b, false))
			switch verdict {
			case "equal":
				if err != nil {
					t.Errorf("Equal = %v, want nil: %s", err, reason)
				}
				if htmlA != htmlB {
					t.Errorf("the pair renders different test HTML, so it cannot be equal:\n%q\n%q", htmlA, htmlB)
				}
			case "different":
				if err == nil {
					t.Errorf("Equal = nil, want a difference: %s", reason)
				}
				if htmlA == htmlB && !strings.Contains(reason, "(github ") {
					t.Errorf("the pair renders equal test HTML %q, so it needs a GitHub fixture to be different", htmlA)
				}
			default:
				t.Fatalf("verdict %q is not equal or different", verdict)
			}
		})
	}

	t.Run("rejects a line break against no line break", func(t *testing.T) {
		t.Parallel()

		// No pair holds this case: the test HTML normalizes the line ending
		// to a space.
		if err := Equal(Parse([]byte("a\nb")), Parse([]byte("ab"))); err == nil {
			t.Fatal("Equal = nil, want a difference")
		}
	})

	t.Run("rejects a soft break against a space", func(t *testing.T) {
		t.Parallel()

		// No pair holds this case: the test HTML normalizes the line ending
		// to a space (design 8.4).
		if err := Equal(Parse([]byte("a\nb")), Parse([]byte("a b"))); err == nil {
			t.Fatal("Equal = nil, want a difference")
		}
	})

	t.Run("rejects links of different forms", func(t *testing.T) {
		t.Parallel()

		// No pair holds these cases: the forms render equal test HTML, but the
		// form is in the key of a link (design 8.4).
		for _, pair := range [][2]string{
			{"[a](/u)\n\n[a]: /u", "[a]\n\n[a]: /u"},
			{"[a][]\n\n[a]: /u", "[a]\n\n[a]: /u"},
			{"[a][a]\n\n[a]: /u", "[a][]\n\n[a]: /u"},
		} {
			if err := Equal(Parse([]byte(pair[0])), Parse([]byte(pair[1]))); err == nil {
				t.Errorf("Equal of %q and %q = nil, want a difference", pair[0], pair[1])
			}
		}
	})

	t.Run("rejects an angle autolink against an extended autolink", func(t *testing.T) {
		t.Parallel()

		// No pair holds this case: both render equal test HTML, but the form is
		// in the key of an autolink (design 10.2).
		if err := Equal(Parse([]byte("<http://a.b>")), Parse([]byte("http://a.b"))); err == nil {
			t.Fatal("Equal = nil, want a difference")
		}
	})

	t.Run("rejects a table cell beyond the header count against no cell", func(t *testing.T) {
		t.Parallel()

		// No pair holds this case: the test HTML writes no cell beyond the header
		// count, but such cells are content (design 8.4).
		if err := Equal(Parse([]byte("| a |\n| - |\n| b | c |")), Parse([]byte("| a |\n| - |\n| b |"))); err == nil {
			t.Fatal("Equal = nil, want a difference")
		}
	})

	t.Run("compares the bytes and the lines of dialect spans", func(t *testing.T) {
		t.Parallel()

		// No pair holds these cases: the paragraph that an HTML block named
		// search interrupts is a dialect span (github 17), which renders equal
		// test HTML in both forms.
		for _, pair := range [][2]string{
			{"a  \n<search>", "a\n<search>"},
			{"> a\n> b\n<search>", "> a\nb\n<search>"},
			{"£~!a~", "£~~!a~~"},
			// GitHub opens no block after 99 on a line, so it reads the 100th
			// marker as text (dialect.md).
			{strings.Repeat(">", 99) + "*", strings.Repeat("> ", 99) + "-\n"},
			{strings.Repeat("- ", 100) + "a", strings.Repeat("- ", 99) + "* a"},
			// A span right after a span: the paragraph that a table split off,
			// and a second line of 100 markers.
			{"a\\|b\n| x |\n| - |\n", "a\\|b\n|x|\n|-|\n"},
			{strings.Repeat(strings.Repeat("- ", 100)+"a\n", 2), strings.Repeat("- ", 100) + "a\n" + strings.Repeat("- ", 99) + "* a\n"},
			// Indentation that GitHub reads: an HTML block starts after at most
			// 3 columns, and past 99 blocks a line of 4 more columns is code.
			{"a\n    <source>\n", "a\n   <source>\n"},
			{"> a\n    <b x=1>\n", "> a\n  <b x=1>\n"},
			{strings.Repeat("- ", 100) + "a\n\n" + strings.Repeat(" ", 202) + "b\n", strings.Repeat("- ", 100) + "a\n\n" + strings.Repeat(" ", 200) + "b\n"},
		} {
			if err := Equal(Parse([]byte(pair[0])), Parse([]byte(pair[1]))); err == nil {
				t.Errorf("Equal of %q and %q = nil, want a difference", pair[0], pair[1])
			}
		}
		for _, pair := range [][2]string{
			{"> a\r\n> b\r\n<search>", "> a\n> b\n<search>"},
			{"- a\n\tb\n<search>", "- a\n    b\n<search>"},
			{"[x]: /u\n'", "[x]: /u\n'\n"},
			{" ```\f\n\t0", " ```\f\n    0"},
			{" ```\v\n ", " ```\v\n \n"},
			// A span that ends where the span holding it ends.
			{"\\|\n0\n -|\n0*\x80", "\\|\n0\n -|\n0*\x80\n"},
			// A tab and spaces of equal columns, in the lines of a span.
			{strings.Repeat("- ", 100) + "a\n\n" + strings.Repeat("\t", 3) + "b\n", strings.Repeat("- ", 100) + "a\n\n" + strings.Repeat(" ", 12) + "b\n"},
		} {
			if err := Equal(Parse([]byte(pair[0])), Parse([]byte(pair[1]))); err != nil {
				t.Errorf("Equal of %q and %q = %v, want nil", pair[0], pair[1], err)
			}
		}
	})

	t.Run("lists every pair", func(t *testing.T) {
		t.Parallel()

		files, err := filepath.Glob("testdata/pairs/*.a.md")
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range files {
			if name := strings.TrimSuffix(filepath.Base(f), ".a.md"); !listed[name] {
				t.Errorf("pair %q is not in pairs.txt", name)
			}
		}
	})
}

// readPair parses the files of pair name and verifies both trees.
func readPair(t *testing.T, name string) (*Tree, *Tree) {
	t.Helper()

	var trees [2]*Tree
	for i, suffix := range []string{".a.md", ".b.md"} {
		src, err := os.ReadFile(filepath.Join("testdata/pairs", name+suffix))
		if err != nil {
			t.Fatal(err)
		}
		trees[i] = Parse(src)
		if err := trees[i].Verify(); err != nil {
			t.Fatalf("%s%s: %v", name, suffix, err)
		}
	}
	return trees[0], trees[1]
}

func FuzzEqual(f *testing.F) {
	for _, src := range []string{
		"- a\n- b\n\n1. c\n2. d\n",
		"> ```\n> a\n> ```\n>\n> b\r\n",
		"    code\n\n\nb\n\n~~~ info\nx\n~~~\n",
		"# h\n\n<div>\n\n[a]: /u 't'\n---\n",
		">     code\n>\n> - x\n>\n>   y\n",
		"a  \nb \\\n> c \nd  \n",
		"\\# a \\* b\\\\\n\\- c\n",
		"&amp; &ast;a&#42; &#x2d; b\n",
		"`a` `` b\n` `` ``` c`",
		"<https://a.b> <a@b.c>\n",
		"a <b\n  c='d'> <!-- e -->\n",
		"*a* __b__ ***c*** _d*\n",
		"[a *b*](<c> \"d\") ![e](f\n'g')\n",
		"[a][Bc] [b][] ![c]\n\n[b]: /u\n[bc]: /v\n[c]: /w\n",
		"~a~ ~~b~~ *~c~* ~~d~\n",
		"| a | b |\n|:-|-:|\n| c |\nd | e | f\n\n> x | y\n> --- | ---\n",
		"a\\|b\n| c\\|d | `e\\|` [f](g\\\\|h) |\n| - | - |\n",
		"- [ ] a\n- [x] b\n- [X]\tc\n\n1. [ ] d\n",
		"www.a.com http://b.c/(d) *www.e.f* HTTPS://g.h.\n",
		"a\\_b@c.de mailto:x@y.zz &#104;@i.jj [e@f.gg](/u)\n",
		"a[^1] b[^x] ![^1] [^*c* [^d]]\n\n[^1]: e\n    f\n",
	} {
		for op := range byte(mutations) {
			f.Add([]byte(src), op)
		}
	}

	f.Fuzz(func(t *testing.T, src []byte, op byte) {
		a := Parse(src)
		mutated := mutateSyntax(a, op)
		b := Parse(mutated)
		if err := b.Verify(); err != nil {
			t.Fatalf("Parse(%q).Verify() = %v", mutated, err)
		}
		if Equal(a, b) != nil {
			return
		}
		if htmlA, htmlB := normalizeHTML(renderHTML(a, false)), normalizeHTML(renderHTML(b, false)); htmlA != htmlB {
			t.Fatalf("Equal accepts %q and its mutation %q, whose test HTML differs:\n%q\n%q", src, mutated, htmlA, htmlB)
		}
	})
}

const mutations = 20

// mutateSyntax returns the source of tree with one kind of block syntax
// changed everywhere, chosen by op: bullet characters, line endings, ordered
// delimiters, fence characters, the number of blank lines, the space after a
// block quote marker, trailing spaces, the backslash of escapes, entity
// references as their characters, the length of code span fences, the
// emphasis character, the quotes of titles, the case of labels, the number of
// tildes of strikethrough, the outer pipes of table rows, the spaces of
// Whitespace leaves, the dashes of delimiter row cells, the backslash before a
// cell pipe escape, the case of the x of task boxes, or the case of footnote
// labels (design 10.5). A mutation may change meaning.
func mutateSyntax(tree *Tree, op byte) []byte {
	var out []byte
	for i, n := range tree.nodes {
		if n.kind.class() == classStructure {
			continue
		}
		b := tree.src[n.start:n.end]
		switch op % mutations {
		case 0:
			if n.kind == ListMarker {
				b = swapBytes(b, "-*+", "*+-")
			}
		case 1:
			if n.kind == LineEnding || n.kind == VerbatimLineEnding || n.kind == BlankLine {
				b = swapLineEnding(b)
			}
		case 2:
			if n.kind == ListMarker {
				b = swapBytes(b, ".)", ").")
			}
		case 3:
			if n.kind == FenceMarker {
				b = swapBytes(b, "`~", "~`")
			}
		case 4:
			if n.kind == BlankLine {
				b = bytes.Repeat(b, 2)
			}
		case 5:
			if n.kind == QuoteMarker {
				if trimmed, ok := bytes.CutSuffix(b, []byte("> ")); ok {
					b = append(bytes.Clone(trimmed), '>')
				} else {
					b = append(bytes.Clone(b), ' ')
				}
			}
		case 6:
			if n.kind == TrailingSpace {
				continue
			}
		case 7:
			if n.kind == Escape {
				b = b[1:]
			}
		case 8:
			if n.kind == EntityRef {
				b = tree.AppendValue(nil, NodeID(i))
			}
		case 9:
			if n.kind == CodeFence {
				b = bytes.Repeat(b, 2)
			}
		case 10:
			if n.kind == Delimiter {
				b = swapBytes(b, "*_", "_*")
			}
		case 11:
			if n.kind == TitleQuote {
				b = swapBytes(b, "\"'", "'\"")
			}
		case 12:
			if n.kind == LinkLabel {
				b = bytes.ToUpper(b)
			}
		case 13:
			if n.kind == Delimiter && b[0] == '~' {
				b = []byte("~~")[:3-len(b)]
			}
		case 14:
			if n.kind == TablePipe && (lineEdge(tree, i, -1) || lineEdge(tree, i, 1)) {
				continue
			}
		case 15:
			if n.kind == Whitespace {
				b = bytes.Repeat(b, 2)
			}
		case 19:
			if n.kind == FootnoteLabel {
				b = bytes.ToUpper(b)
			}
		case 18:
			if n.kind == TaskBox {
				b = swapBytes(b, "xX", "Xx")
			}
		case 17:
			if n.kind == CellPipeEscape {
				b = []byte(`\\|`)[len(b)-2:]
			}
		case 16:
			if n.kind == TableDelimiter {
				j := bytes.IndexByte(b, '-')
				b = append(append(bytes.Clone(b[:j]), '-'), b[j:]...)
			}
		}
		out = append(out, b...)
	}
	return out
}

// lineEdge reports whether only structure, Whitespace, Indent and prefix
// leaves are between leaf i and the start of its line, for dir -1, or the end
// of its line, for dir 1.
func lineEdge(tree *Tree, i, dir int) bool {
	for j := i + dir; j >= 0 && j < len(tree.nodes); j += dir {
		switch k := tree.nodes[j].kind; {
		case k.class() == classStructure, k == Whitespace, k == Indent:
		case k == LineEnding:
			return true
		default:
			_, prefix := k.owner()
			return prefix && dir < 0
		}
	}
	return true
}

// swapBytes returns a copy of b with each byte in from replaced by the byte at
// the same index in to.
func swapBytes(b []byte, from, to string) []byte {
	out := bytes.Clone(b)
	for i, c := range out {
		if j := strings.IndexByte(from, c); j >= 0 {
			out[i] = to[j]
		}
	}
	return out
}

// swapLineEnding returns b with its line ending swapped: CRLF becomes LF, and
// LF or CR becomes CRLF.
func swapLineEnding(b []byte) []byte {
	switch {
	case bytes.HasSuffix(b, []byte("\r\n")):
		return append(bytes.Clone(b[:len(b)-2]), '\n')
	case bytes.HasSuffix(b, []byte("\n")), bytes.HasSuffix(b, []byte("\r")):
		return append(bytes.Clone(b[:len(b)-1]), '\r', '\n')
	}
	return b
}

// TestComparer_Compare covers the guards of compare that no pair of trees
// reaches: Equal stops at an earlier difference before a projection can run
// out of events or give two content runs of different groups.
func TestComparer_Compare(t *testing.T) {
	t.Parallel()

	tree := Parse([]byte("a\n"))
	c := comparer{a: tree, b: tree}

	t.Run("reports that one tree has more events", func(t *testing.T) {
		t.Parallel()

		enter := event{op: enterEvent}
		if err := c.compare(enter, true, enter, false); err == nil {
			t.Fatal("compare gives no error where one tree has no more events")
		}
	})

	t.Run("reports different content groups", func(t *testing.T) {
		t.Parallel()

		content := event{op: contentEvent, group: Paragraph}
		other := content
		other.group = Heading
		if err := c.compare(content, true, other, true); err == nil {
			t.Fatal("compare gives no error for content runs of different groups")
		}
	})
}

func TestKept(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  string
		want []string
	}{
		{"gives escapes, entity references and cell pipe escapes with their bytes", "a \\* &amp;\n\n| b\\|c |\n| - |", []string{"Escape \\*", "EntityRef &amp;", "CellPipeEscape \\|"}},
		{"gives the raw bytes of link labels without prefixes and with lf line endings", "> [Foo\r\n> bar] [x][Y] [z][]\n\n[foo bar]: /u\n[y]: /v\n[z]: /w", []string{"label Foo\nbar", "label Y", "label z", "label foo bar", "label y", "label z"}},
		{"gives the raw labels of footnote references and definitions", "a[^B]\n\n[^B]: x", []string{"label B", "label B"}},
		{"gives each nul and invalid utf-8 sequence in content", "a\x00b\xa6\xe0\xa0c", []string{"invalid \x00", "invalid \xa6", "invalid \xe0\xa0"}},
		{"gives an ordered list that starts at 1 and whose second item is 1", "1. a\n1. b\n\n- c\n\n3) d\n1) e\n\n1. f\n2. g", []string{"lazy numbering"}},
		{"gives the rows and the blank lines around each dialect span", "a\n<search>\n\n\nb", []string{"span 1, blank lines 0 and 0", "span 1, blank lines 0 and 2"}},
		{"gives no blank line for the blank rest of a list marker line", "*\n\xf1*", []string{"span 100000, blank lines 0 and 0", "invalid \xf1"}},
		{"gives no blank lines at the start of a container or at the end of the input", "\n<search>\n\n\n\n> \n> <search>\n>\n", []string{"span 1, blank lines 0 and 3", "span 1, blank lines 0 and 0"}},
		{"gives no blank lines after the label line of a footnote definition", "[^0]:\n\n    <search>\n", []string{"label 0", "span 1, blank lines 0 and 0"}},
		{"gives nothing for text, emphasis and an inline link", "a *b* [c](/u)", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := Kept(Parse([]byte(tt.src))); !slices.Equal(got, tt.want) {
				t.Fatalf("Kept(%q) = %q, want %q", tt.src, got, tt.want)
			}
		})
	}
}
