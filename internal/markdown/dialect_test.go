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
			"dialect/html-block-search": rowSearch,
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
