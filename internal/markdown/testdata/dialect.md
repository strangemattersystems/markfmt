# Dialect rows

Each row is a rule where GitHub and CommonMark 0.31.2 give a document a
different meaning (design 2.1). markfmt follows CommonMark 0.31.2 for core
constructs, and GitHub where a GFM construct decides. Each row names an input,
markfmt's result, GitHub's result, the GitHub fixture in `github/github.txt`
that shows GitHub's result, and the markfmt test of the rule. Where markfmt
and GitHub differ, `github/grammar-differs.txt` lists the fixture with a case
of markfmt's result (design 11.4). At stage 6 each row gets a dialect
predicate.

| Rule | Input | markfmt | GitHub | GitHub fixture | markfmt test |
| --- | --- | --- | --- | --- | --- |
| HTML block kind 6 tag list: `search` is in the list, `source` is not | `a⏎<search>⏎*b*` | Paragraph `a`, then an HTML block `<search>⏎*b*` | Paragraph `a⏎<search>⏎*b*` with raw HTML and emphasis: `<search>` is kind 7, which cannot interrupt a paragraph | `dialect/html-block-search` | `TestParse/interrupts_a_paragraph_with_a_search_html_block` |
| HTML block kind 6 tag list: `source` is kind 7 | `a⏎<source>⏎*b*` | Paragraph `a⏎<source>⏎*b*` with raw HTML and emphasis | Paragraph `a`, then an HTML block `<source>⏎*b*` | `dialect/html-block-source` | `TestParse/does_not_interrupt_a_paragraph_with_an_html_block_of_kind_7` |
| HTML block kind 7 on a lazy candidate line: kind 7 does not start while a paragraph is open, matched or not | `> a⏎<del>⏎*b*` | Block quote with paragraph `a⏎<del>⏎*b*`: the lines are lazy | Block quote with paragraph `a`, then an HTML block `<del>⏎*b*` | `dialect/html-block-lazy-line` | `TestParse/does_not_start_an_html_block_of_kind_7_on_a_lazy_line` |
| Setext underline after link reference definitions alone: the line is dispatched again as if a paragraph were open | `[foo]: /url⏎---` | Definition, then a thematic break | Paragraph `---` | `dialect/definition-underline` | `TestParse/dispatches_an_underline_after_definitions_alone_as_if_a_paragraph_were_open` |
| Raw HTML comment: `<!--`, text without `-->`, and `-->`; or `<!-->`; or `<!--->` (CM 626) | `a <!--> b --> <!-- c -- d --->` | Paragraph with the raw HTML `<!-->`, the text ` b --> `, and the raw HTML `<!-- c -- d --->` | Paragraph with the raw HTML `<!-->`, then the text ` b --> <!-- c -- d --->`: comment text must not contain `--` | `dialect/html-comment` | `TestParse/gives_raw_html_comments,_processing_instructions,_declarations_and_cdata_sections` |
| Unicode punctuation for flanking: Unicode P and S, and U+FFFD, which is So | `£_a_£` | Paragraph with the emphasis `a` | Paragraph `£_a_£`: only Unicode P and ASCII punctuation count, so `_` is not preceded by punctuation | `dialect/flanking-symbol` | `TestParse/treats_unicode_symbols_and_u+fffd_as_punctuation_for_flanking` |
| HTML block kind 4 start: `<!` and any ASCII letter | `<!doctype html>` | HTML block | Paragraph with the text `<!doctype html>`: the kind 4 start and the declaration grammar need an uppercase letter | `dialect/html-block-declaration` | `TestParse/starts_an_html_block_of_kind_4_with_any_ascii_letter` |
| Paragraph split off above a table: its lines get the cell pipe rule of the table (design 5.4). CommonMark 0.31.2, which has no tables, gives the text `a\|b c\\|d` and the code span `e\\|f` | ``a\\|b c\\\|d `e\\|f`⏎\| x \|⏎\| - \|`` | Paragraph with the text `a\|b c\|d` and the code span `e\|f`, then a table | The same as markfmt | `tables/split-paragraph` | `TestParse/gives_cell_pipe_escapes_in_text,_a_code_span,_a_destination_and_the_paragraph_above_a_table` |
| List items that start on one line: cmark 0.31.1 opens every one, and cmark-gfm opens at most 99 blocks on a line (`MAX_LIST_DEPTH`) | `- `×100 `a` | 100 nested lists, the last item with the paragraph `a` | 99 nested lists, the last item with the paragraph `- a` | `dialect/list-items-on-one-line` | `TestParse/opens_every_list_item_that_starts_on_a_line` |
