package markdown

import (
	"slices"
	"strconv"
	"testing"
)

func TestTree_DialectSpans(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  string
		want []string // the kind and the rows, in binary, of each span in node order
	}{
		{"finds an html block named search and the paragraph it interrupts", "a\n<search>\n*b*", []string{"Paragraph 1", "HTMLBlock 1"}},
		{"finds the paragraph of a block quote that an html block named search closes", "> a\n<SEARCH class=x>", []string{"Paragraph 1", "HTMLBlock 1"}},
		{"finds an html block named search after a blank line alone", "a\n\n</search>", []string{"HTMLBlock 1"}},
		{"finds no span for other html blocks", "a\n<searches>\n\n<div>", nil},
		{"finds a paragraph line that starts with a source tag", "a\n<source src=x", []string{"Paragraph 10"}},
		{"finds a table row that starts with a source tag", "| a |\n| - |\n</SOURCE", []string{"Table 10"}},
		{"finds a definition line that starts with a source tag", "[a]: /u 'b\n<source x'", []string{"LinkReferenceDefinition 10"}},
		{"finds no span for an html block of kind 7 named source", "<source>\n", nil},
		{"finds an html block that starts a declaration with a lowercase letter", "a\n<!doctype html>", []string{"Paragraph 100", "HTMLBlock 100"}},
		{"finds raw html that starts a declaration with a lowercase letter", "| a <!x> |\n| - |", []string{"TableCell 100"}},
		{"finds no span for a declaration with an uppercase letter", "a <!X>\n\n<!DOCTYPE html>", nil},
		{"finds a lazy paragraph line that starts html block kind 7", "> a\n<del>\n*b*", []string{"Paragraph 1000"}},
		{"finds no span for a paragraph line that starts html block kind 7 and is not lazy", "> a\n> <del>\n\n- b\n  <del>", nil},
		{"finds raw html comments that github does not read", "a <!--> b <!---> c <!-- -- --> d <!-- - --> e <!--->-->", []string{"Paragraph 10000"}},
		{"finds no span for raw html comments that github reads", "a <!-- b --> <!----> <!-- c - d -->", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tree := Parse([]byte(tt.src))
			var got []string
			for _, s := range tree.dialectSpans() {
				got = append(got, tree.nodes[s.id].kind.String()+" "+strconv.FormatUint(uint64(s.rows), 2))
			}
			if !slices.Equal(got, tt.want) {
				t.Fatalf("dialectSpans of %q = %q, want %q", tt.src, got, tt.want)
			}
		})
	}

	t.Run("finds a span of its row in the github fixture of each row", func(t *testing.T) {
		t.Parallel()

		rows := map[string]dialectRow{
			"dialect/html-block-search":      rowSearch,
			"dialect/html-block-source":      rowSource,
			"dialect/html-block-declaration": rowDeclaration,
			"dialect/html-block-lazy-line":   rowLazyKind7,
			"dialect/html-comment":           rowComment,
		}
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
