# Dialect rows

Each row is a rule where GitHub and CommonMark 0.31.2 give a document a
different meaning (design 2.1). markfmt's grammar follows CommonMark 0.31.2
for core constructs. Each row names an input, both results, and the markfmt
test of the rule. The GitHub fixture of each row lands at stage 4. At stage 6
each row gets a dialect predicate.

| Rule | Input | CommonMark 0.31.2 and markfmt | GitHub | markfmt test |
| --- | --- | --- | --- | --- |
| HTML block kind 6 tag list: `search` is in the list, `source` is not | `a⏎<search>` | Paragraph `a`, then an HTML block `<search>` | Paragraph `a⏎<search>` with raw HTML: `<search>` is kind 7, which cannot interrupt a paragraph | `TestParse/interrupts_a_paragraph_with_a_search_html_block` |
| HTML block kind 6 tag list: `source` is kind 7 | `a⏎<source>` | Paragraph `a⏎<source>` with raw HTML | Paragraph `a`, then an HTML block `<source>` | `TestParse/does_not_interrupt_a_paragraph_with_an_html_block_of_kind_7` |
| HTML block kind 4 start: `<!` and any ASCII letter | `<!doctype html>` | HTML block | Paragraph with the text `<!doctype html>`: the kind 4 start and the declaration grammar need an uppercase letter | `TestParse/starts_an_html_block_of_kind_4_with_any_ascii_letter` |
