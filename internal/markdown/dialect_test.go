package markdown

import (
	"maps"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
)

func TestTree_DialectSpans(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  string
		want []string // the kind and the rows of each span in node order
	}{
		{"finds an html block named search and the paragraph it interrupts", "a\n<search>\n*b*", []string{"Paragraph 0", "HTMLBlock 0"}},
		{"finds the paragraph of a block quote that an html block named search closes", "> a\n<SEARCH class=x>", []string{"Paragraph 0", "HTMLBlock 0"}},
		{"finds an html block named search after a blank line alone", "a\n\n</search>", []string{"HTMLBlock 0"}},
		{"finds no span for other html blocks", "a\n<searches>\n\n<div>", nil},
		{"finds a paragraph line that starts with a source tag", "a\n<source src=x", []string{"Paragraph 1"}},
		{"finds a table row that starts with a source tag", "| a |\n| - |\n</SOURCE", []string{"Table 1"}},
		{"finds a definition line that starts with a source tag", "[a]: /u 'b\n<source x'", []string{"LinkReferenceDefinition 1"}},
		{"finds no span for an html block of kind 7 named source", "<source>\n", nil},
		{"finds an html block that starts a declaration with a lowercase letter", "a\n<!doctype html>", []string{"Paragraph 2", "HTMLBlock 2"}},
		{"finds raw html that starts a declaration with a lowercase letter", "| a <!x> |\n| - |", []string{"TableCell 2"}},
		{"finds no span for a declaration with an uppercase letter", "a <!X>\n\n<!DOCTYPE html>", nil},
		{"finds a lazy paragraph line that starts html block kind 7", "> a\n<del>\n*b*", []string{"Paragraph 3"}},
		{"finds no span for a paragraph line that starts html block kind 7 and is not lazy", "> a\n> <del>\n\n- b\n  <del>", nil},
		{"finds raw html comments that github does not read", "a <!--> b <!---> c <!-- -- --> d <!-- - --> e <!--->-->", []string{"Paragraph 4"}},
		{"finds no span for raw html comments that github reads", "a <!-- b --> <!----> <!-- c - d -->", nil},
		{"finds a delimiter run next to a unicode symbol", "£_a_£", []string{"Paragraph 5"}},
		{"finds delimiter runs next to nul, invalid utf-8 and u+fffd", "a\x00*b*\n\nc*d*\xa6\n\ne\ufffd_f_", []string{"Paragraph 5", "Paragraph 5", "Paragraph 5"}},
		{"finds a delimiter run next to an emoji in a table cell", "| a |\n| - |\n| 😀*b* |", []string{"TableCell 5"}},
		{"finds no span for delimiter runs next to ascii punctuation or spaces", "$*a*$ £ _b_ £", nil},
		{"finds a code span after a backtick run that is text", "a `` b `c` d `e`", []string{"Paragraph 6"}},
		{"finds no span for a backtick run after a code span or an escaped backtick", "`c` a `` b\n\na \\` `b`", nil},
		{"finds blocks with brackets, vt or ff in a document with vt or ff", "[a](b\fc)\n\n[a]\n\n[a\v]: /u\n\nd\fe\n\nf", []string{"Paragraph 7 8", "Paragraph 7 8", "LinkReferenceDefinition 7 8", "Paragraph 7 8"}},
		{"finds no span for brackets in a document without vt or ff", "[a](b)", nil},
		{"finds brackets with 1000 bytes between them", "[" + strings.Repeat("a", 1000) + "]", []string{"Paragraph 9"}},
		{"finds a definition whose label has 1000 bytes", "[" + strings.Repeat("é", 500) + "]: /u", []string{"LinkReferenceDefinition 9"}},
		{"finds brackets with 1000 bytes between them across an escaped bracket", "[" + strings.Repeat("a", 998) + "\\]]", []string{"Paragraph 9"}},
		{"finds no span for brackets with 999 bytes between them", "[" + strings.Repeat("a", 999) + "](/u)", nil},
		{"finds a paragraph that starts with an underline after a definition", "[foo]: /url\n---\n\n[bar]: /u\n===", []string{"LinkReferenceDefinition 10", "Paragraph 10", "LinkReferenceDefinition 10", "Paragraph 10"}},
		{"finds a setext heading that starts with an underline after a definition", "[baz]: /url\n-\n-", []string{"LinkReferenceDefinition 10", "Heading 10"}},
		{"finds no span for a paragraph after a blank line or without an underline after a definition", "[foo]: /url\nbar\n\n[a]: /u\n\n===", nil},
		{"finds a paragraph that starts with a quote after a definition without a title", "[foo]: /url\n\"title\"ok\n\n[a]: /u\n(b) c", []string{"LinkReferenceDefinition 11", "Paragraph 11", "LinkReferenceDefinition 11", "Paragraph 11"}},
		{"finds no span for a paragraph that starts with a quote after a definition with a title", "[foo]: /url 't'\n\"x\"", nil},
		{"finds fenced code with vt, ff or a character reference in its info string", "~~~\va\n~~~\n\n``` &#12;b\n```\n\n```c\n```", []string{"CodeBlock 12", "CodeBlock 12"}},
		{"finds a paragraph with a cell pipe escape above its table", "a\\|b\n| x |\n| - |", []string{"Paragraph 13", "Table 13"}},
		{"finds no span for a paragraph without a cell pipe escape above its table", "a|b\n| x |\n| - |", nil},
		{"finds the first block of a line where a list item is block 100", strings.Repeat("- ", 100) + "a\n\n" + strings.Repeat("> ", 99) + "- b", []string{"ListItem 14", "BlockQuote 14"}},
		{"finds no span for a line of 99 list items", strings.Repeat("- ", 99) + "a", nil},
		{"finds a footnote reference whose caret is an escape or an entity reference", "a[\\^1] b[&#94;1]\n\n[^1]: x", []string{"Paragraph 15"}},
		{"finds a footnote reference with a line ending in its label", "a[^b\nc]\n\n[^b]: x", []string{"Paragraph 16"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tree := Parse([]byte(tt.src))
			var got []string
			for _, s := range tree.dialectSpans() {
				span := []string{tree.nodes[s.id].kind.String()}
				for row := range 32 {
					if s.rows&(1<<row) != 0 {
						span = append(span, strconv.Itoa(row))
					}
				}
				got = append(got, strings.Join(span, " "))
			}
			if !slices.Equal(got, tt.want) {
				t.Fatalf("dialectSpans of %q = %q, want %q", tt.src, got, tt.want)
			}
		})
	}

	t.Run("has a predicate for each row of dialect.md", func(t *testing.T) {
		t.Parallel()

		data, err := os.ReadFile("testdata/dialect.md")
		if err != nil {
			t.Fatal(err)
		}
		fixtures := regexp.MustCompile("\\| `([a-z-]+/[a-z0-9-]+)` \\|").FindAllStringSubmatch(string(data), -1)
		if len(fixtures) != len(dialectFixtures) {
			t.Errorf("dialect.md names %d fixtures, and %d have a predicate", len(fixtures), len(dialectFixtures))
		}
		for _, f := range fixtures {
			if _, ok := dialectFixtures[f[1]]; !ok {
				t.Errorf("the row with fixture %s has no predicate", f[1])
			}
		}
	})

	t.Run("finds a span of its row in the github fixture of each row", func(t *testing.T) {
		t.Parallel()

		rows := maps.Clone(dialectFixtures)
		for _, ex := range readExamples(t, "testdata/github/github.txt") {
			row, ok := rows[ex.section]
			if !ok {
				continue
			}
			delete(rows, ex.section)
			if !slices.ContainsFunc(Parse([]byte(ex.markdown)).dialectSpans(), func(s dialectSpan) bool { return s.rows&(1<<row) != 0 }) {
				t.Errorf("%s: no span of row %d in %q", ex.section, row, ex.markdown)
			}
		}
		for section := range rows {
			t.Errorf("no github fixture %s", section)
		}
	})
}

// dialectFixtures maps the GitHub fixture of each row of testdata/dialect.md
// to its row.
var dialectFixtures = map[string]dialectRow{
	"dialect/html-block-search":       rowSearch,
	"dialect/html-block-source":       rowSource,
	"dialect/html-block-declaration":  rowDeclaration,
	"dialect/html-block-lazy-line":    rowLazyKind7,
	"dialect/html-comment":            rowComment,
	"dialect/flanking-symbol":         rowFlanking,
	"dialect/code-span-unmatched-run": rowCodeSpan,
	"dialect/destination-vt-ff":       rowDestinationVTFF,
	"dialect/label-vt-ff":             rowLabelVTFF,
	"dialect/label-length":            rowLabelLength,
	"dialect/definition-underline":    rowDefinitionUnderline,
	"dialect/definition-failed-title": rowFailedTitle,
	"dialect/info-string-vt-ff":       rowInfoVTFF,
	"tables/split-paragraph":          rowSplitParagraph,
	"dialect/list-items-on-one-line":  rowListItems,
	"footnotes/escaped-caret":         rowFootnoteCaret,
	"footnotes/label-line-ending":     rowFootnoteLineEnding,
}
