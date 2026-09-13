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
| HTML block kind 7 on a lazy candidate line: kind 7 does not start while a paragraph is open, matched or not | `> a⏎<del>` | Block quote with paragraph `a⏎<del>`: the line is lazy | Block quote with paragraph `a`, then an HTML block `<del>` | `TestParse/does_not_start_an_html_block_of_kind_7_on_a_lazy_line` |
| Setext underline after link reference definitions alone: the line is dispatched again as if a paragraph were open | `[foo]: /url⏎---` | Definition, then a thematic break | Paragraph `---` | `TestParse/dispatches_an_underline_after_definitions_alone_as_if_a_paragraph_were_open` |
| Raw HTML comment: `<!--`, text without `-->`, and `-->`; or `<!-->`; or `<!--->` (CM 626) | `a <!--> b --> <!-- c -- d --->` | Paragraph with the raw HTML `<!-->`, the text ` b --> `, and the raw HTML `<!-- c -- d --->` | Paragraph with text only: comment text must not start with `>` or `->`, contain `--`, or end with `-` | `TestParse/gives_raw_html_comments,_processing_instructions,_declarations_and_cdata_sections` |
| Unicode punctuation for flanking: Unicode P and S, and U+FFFD, which is So | `£_a_£` | Paragraph with the emphasis `a` | Paragraph `£_a_£`: only Unicode P and ASCII punctuation count, so `_` is not preceded by punctuation | `TestParse/treats_unicode_symbols_and_u+fffd_as_punctuation_for_flanking` |
| HTML block kind 4 start: `<!` and any ASCII letter | `<!doctype html>` | HTML block | Paragraph with the text `<!doctype html>`: the kind 4 start and the declaration grammar need an uppercase letter | `TestParse/starts_an_html_block_of_kind_4_with_any_ascii_letter` |
