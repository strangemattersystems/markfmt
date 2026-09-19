# markfmt roadmap

Read this document first when you resume work on markfmt. It records the
product rules, the current state, the decisions and the reasons for them, and
the plan with its gates. Update it in the same commit as the work it tracks.

The parser design is `docs/design/parser.md`. Read it before stage 1 work. Its
section numbers are cited below as "design 5.4".

## Product rules

1. markfmt reads Markdown and writes Markdown. It has no other output format.
2. markfmt has one canonical style with minimal optional configuration. The
   defaults must suit at least 95% of users, who never set an option.
3. Add an option only for a real need that the default cannot meet. Record
   the reason for each option in Decisions. There are no options yet.
4. markfmt canonicalizes syntax and preserves content: prose line breaks,
   code, raw HTML, front matter and link destinations.
5. Formatting is idempotent: `format(format(x)) == format(x)`.
6. Formatting never changes the meaning of a document. If markfmt cannot
   show that the meaning is unchanged, it returns an error and writes nothing.
7. Output has LF line endings, as gofmt does. Input CRLF, CR and LF are all
   line endings, as CommonMark defines.
8. The end state has zero dependencies in the root `go.mod`.

## Working rules

- Commit when a piece of work is done. Ask before every push to `main`.
- Use Conventional Commits. Never put a session link in a commit or on GitHub.
- Run `task ci` before a commit. Use `task lint`, not the `golangci-lint` on
  `PATH` (that one is v1 and cannot read the v2 config).
- Fix root causes, not symptoms. For each bug, from fuzzing or elsewhere:
  1. Classify it: parser conformance, printer, or tree comparison.
  2. Add a failing case to the component that owns the bug.
  3. Fix it where the rule lives. Do not add special cases to the block loop
     of the printer. Do not normalize output to hide a difference.
  4. Name the spec rule or invariant that the fix restores in the commit
     message. A fix that cannot name one is probably a symptom fix.

## Current state

Last updated: 2026-09-19. v0.0.1 is the first release. `main` is pushed and
holds the CLI changes after v0.0.1. `feat/phase0-gates` is not merged or
pushed.

| Commit | Content |
| --- | --- |
| `ff7de5b` | README |
| `fac76fe` | Formatter skeleton on goldmark v2.0.2, CLI, golangci-lint, Taskfile, CI, Dependabot |
| `bc99d39` | Spec corpus test, `FuzzSource`, fixes for 10 corpus and fuzz findings |
| `3898ac7` | LF line endings |
| `62af5e4` to `5123f12` | This roadmap |
| `1f63d60`, `f32fd7e` | Parser design, after five review rounds |
| `104566e` | `.scratch/` is gitignored |
| `59e930f` onwards | Stage 1: tree, builder, `Verify`, line iterator, test HTML renderer, conformance runner, five corpora |
| `1604b94` onwards | Stage 2: paragraphs, blank lines, thematic breaks, ATX and setext headings, indented and fenced code, HTML blocks, block quotes, tabs, lists, link reference definitions, front matter, label normalization, `Equal` for block kinds, pathological inputs and `task long` |
| `1cca948` onwards | Stage 3: the inline phase, line breaks, backslash escapes, entity references, code spans, autolinks, raw HTML, emphasis, links and images, pass 1 and reference links, `BenchmarkParse` |
| `6110977` onwards | Stage 4: the GFM tag filter, GitHub fixtures and the GitHub normalizer, strikethrough, tables, cell pipe escapes, task list items, extended www, URL and email autolinks, footnote definitions and references, the indentation memo |
| `f51a4c6` onwards | Stage 5: `FuzzDifferential` against goldmark v2.0.2, the differential cases and their predicates, fixes for NUL, control characters, VT and FF, kind 7 tag names and code span closers |
| `dfc151f` onwards | Stage 6: the canonical style survey, `Source` on the new parser, `FuzzFormat`, dialect predicates and spans in `Equal`, a printer for every node kind, display width from Unicode `EastAsianWidth.txt`, aligned tables, the GitHub printer fixtures and their API check, output bounds, fixes for the `FuzzFormat` findings, and the kept marker rule |
| `1d28ee8` onwards | Stage 8: CLI directory walking, the goreleaser release and the Homebrew cask, v0.0.1 |
| The commit after `5d4e86a` | Stage 7: goldmark, `differential_test.go` and `go.sum` removed; the root `go.mod` has no requirements |
| `4e7fbc0`, `b10d309` (`main`, after v0.0.1) | CLI: `--exclude` replaces the built-in directory skips, double-dash flags, gofmt-style paths, a read limit per input, concurrency from `GOMAXPROCS` |
| `8322491` onwards (`feat/phase0-gates`) | Parse and format snapshots, pathological inputs from the reviews, benchmarks and `task bench-compare`, the printer split by file, the runtime `Kept` check, pass-through of a block that fails its check, a block with a dialect span printed as written, `FuzzSource`, the exported limits and errors |

Layout:

- `cmd/markfmt`: the CLI. `markfmt [--check] [--exclude pattern]... [path ...]` and `markfmt --version`.
  A directory path gives its Markdown files. With no path or `-`, it reads
  stdin and writes stdout. It writes files atomically.
- `.goreleaser.yaml`, `Dockerfile.goreleaser`, `.github/workflows/release.yml`:
  the release, run on a pushed `v` tag.
- `markfmt.go`: the public API: `Format(w io.Writer, r io.Reader) error`,
  `MaxInput`, `MaxOutput`, `ErrTooLarge` and `*InternalError`.
- `internal/markdown`: the new parser and its tree (stage 1 onwards).
- `internal/markdown/testdata/<corpus>`: the conformance corpora `commonmark`,
  `gfm`, `cmark-gfm-extensions`, `cmark-gfm-regression` and
  `commonmark-js-regression`, each with a README and `failing.txt`.
- `internal/markdown/testdata/dialect.md`: the rules where GitHub and
  CommonMark 0.31.2 differ, one row each (design 2.1).
- `internal/markdown/testdata/github`: GitHub Markdown API fixtures, their
  inputs and the capture loop (design 11.4). `github.txt` runs as conformance
  after the GitHub normalizer. `printer.txt` holds math, alert and plain text
  fixtures, each with a printer case, and `printer-output.txt` the HTML of
  each case output.
- `internal/markdown/testdata/markfmt/grammar.txt`: markfmt's own cases, with
  the expected HTML of each example that a `grammar-differs.txt` lists
  (design 11.2). `testdata/commonmark/grammar-differs.txt` lists CM 96 and 98
  (front matter). `testdata/commonmark-js-regression/grammar-differs.txt`
  lists regression example 25, where cmark and commonmark.js disagree on list
  looseness and markfmt follows cmark.
- `internal/markdown/testdata/differential/cases.txt`: the disagreements that
  `FuzzDifferential` found, each marked "fixed in markfmt" or "goldmark
  deviates" (design 11.5).
- `internal/markdown/testdata/pairs`: the pair corpus of `TestEqual`, each
  pair equal or different with a reason, checked against the test HTML
  (design 10.5).
- `internal/markdown/testdata/unicode/CaseFolding.txt`: the source of the
  generated full case folding table.
- `internal/markdown/testdata/entities/entities.json`: the source of the
  generated entity table.
- `internal/format`: the formatter on the new parser. `Source` parses the
  input, prints it in the canonical style (`print.go` and the files it is
  split into), parses the output, and calls `Equal` and `KeptMismatch`. A
  top-level block that fails the check prints as written (design 10).
  `Strict` returns the error instead, for the printer tests.
- `internal/format/testdata/cases`: 211 `NAME.in.md` and `NAME.out.md` pairs.
  Each GitHub printer fixture has a pair named `github-` and its section.
- `internal/format/testdata/spec`: CommonMark 0.31.2 examples (goldmark's
  `spec.json`) and goldmark's extra and GFM case files, as idempotence data.
- `internal/markdown/format_test.go`: `FuzzSource` fuzzes
  `format.Source` as users run it. `FuzzFormat`, an external test of
  `internal/format` against the test HTML (design 10.5). Its seeds are every
  corpus, the pairs, `testdata/cases`, and in `testdata/fuzz/FuzzFormat` the
  inputs that the fuzzer found, each fixed. `TestFormatSource` bounds the
  output-to-input ratio of the corpora. Its pathological subtest bounds the
  time of `Source` at n and 10n bytes for the pathological inputs and the
  inputs that deep nesting makes slow in the printer, and its long subtest
  runs them at the input limit within the time and memory budget.
- `internal/markdown/testdata/snapshot`: the tree and the output of every
  corpus input. `task snapshot` updates them.
- `internal/markdown/testdata/bench`: the frozen inputs of the benchmarks.
  `task bench` and `task bench-compare BASE=main` run them.
- `tools/go.mod`: golangci-lint v2.13.2 and benchstat, kept out of the root `go.mod`, which
  has no requirements (product rule 8).
- `docs/design/parser.md`: the parser design.
- `.scratch/design-research/` (local only, not tracked): the research reports
  and the five rounds of design reviews behind the parser design.

## Decisions

| Decision | Reason |
| --- | --- |
| Go | One static binary, fast start, `io.Reader`/`io.Writer`, built-in fuzzing, easy cross-compilation. |
| Public API at the module root, implementation in `internal/` | Import path `github.com/strangemattersystems/markfmt`. `/pkg` adds nothing. |
| golangci-lint in `tools/go.mod` | A `tool` line in the root `go.mod` raises the versions of shared modules for every module that imports markfmt. Tested: a consumer's `golang.org/x/sys` went from v0.30.0 to v0.47.0. |
| Test cases on disk in `testdata/cases` | Easy to add and review. Separate from `testdata/fuzz`, which Go's fuzzer owns. |
| `testdata/.editorconfig` and `.gitattributes` `-text` | Fixtures depend on exact bytes: trailing spaces, no final newline, CR. Editors and Windows checkouts change them. |
| LF output | gofmt writes LF, even inside raw strings. CommonMark reads CRLF, CR and LF as line endings. |
| Write our own parser | 11 fuzz findings had three root causes. 6 came from rebuilding source from goldmark positions, which are not lossless. 2 came from goldmark HTML renderer quirks used as the safety check. 3 were goldmark deviations from the spec. We do not want to wait for upstream fixes. |
| No HTML output in markfmt | markfmt is Markdown in, Markdown out. |
| Lossless concrete syntax tree in a flat node array | Every byte is in one leaf, so one linear check proves losslessness. 16-byte pointer-free nodes are not scanned by the collector. Design 3. |
| Runtime safety check compares event sequences | Enter, Content and Exit events, with a key per structure kind, content groups and dialect spans. No renderer, no circular check. Design 10. |
| Remove goldmark completely | It is a temporary test oracle only. See stage 7 for the exit criteria. |
| Test-only HTML renderer | The specs give expected results only as HTML. The renderer lives in `_test.go` files, never in the binary or the API. |
| Grammar scope: CommonMark, GFM, front matter, and GitHub footnotes | Most users expect what GitHub renders. GitHub math and alerts are HTML filters, not grammar: the runtime check compares every input they read, so no math or alert kinds exist. Design 9. |
| Dialect rows and predicates | Where GitHub and CommonMark 0.31.2 disagree, a `dialect.md` row records it. At stage 6 a predicate finds each span, `Equal` compares spans both ways with their bytes, and the printer prints each top-level block that holds one as written. Design 2.1, 10.4. |
| Input limit 8 MiB, output limit 16 MiB | Constants, not options, chosen from a 4 GiB worst-case memory budget. Design 7.2. |
| `Format` recovers panics | A parser or printer bug returns an `*InternalError` and writes nothing, so one bad file does not stop the CLI. The error holds the stack, and `Error()` leaves it out; the CLI prints it. Design 1. |
| One grammar in production and tests | No test-only grammar switch. Core examples that markfmt's GFM or front matter rules change are listed in `grammar-differs.txt`, each with a case of markfmt's expected result. Design 11.2. |
| Streaming not planned | The documents that would need it are one top-level block, so per-block streaming would not help. Design 7.4. |
| Design document at `docs/design/parser.md` | It is reviewed and versioned with the code. |
| `grammar-differs.txt` also lists corpus examples where cmark and commonmark.js disagree | markfmt follows cmark (design 2). The named case holds the output of `cmark --unsafe`. commonmark.js regression 25: cmark 0.31.1 makes a list loose after a blank line in an HTML block. |
| Full case folding table from Unicode `CaseFolding.txt` | Label matching needs Unicode full case folding (CM 540: `ẞ` matches `SS`). Go's `unicode` package has only simple folding. Design 15, commit 19. |
| Differential fuzz budget: 1 CPU-hour at stage 5, about 2 CPU-hours more at stage 7 | The stage 7 budget was 24 CPU-hours. On 2026-09-16 the user chose to run no more than about 30 minutes on a laptop, and to remove goldmark after that. The parser had passed every corpus. Stage 7 ran 8 workers for 8 minutes, with no disagreement, and for 7 minutes, which found one goldmark deviation (a differential case). `FuzzFormat` does not replace the differential test: it compares the output with markfmt's own parser, so it cannot see a misparse that input and output share. |
| No goldmark extensions in the differential test | goldmark's GFM extensions are not cmark-gfm: with them on, goldmark disagrees on 81 corpus examples with a GFM construct. The corpora and GitHub fixtures test GFM. Design 11.5. |
| Control characters in destinations and absolute URIs follow cmark | The spec text excludes them, but no example tests it, and cmark, commonmark.js, cmark-gfm and goldmark all take them. The user chose cmark on 2026-09-13. Design 8.4. |
| Code spans, definition titles and label lengths follow the spec where cmark does not | cmark 0.31.1 and cmark-gfm leave the second code span of `a `` b `c` d `e`` as text, a bug in their search record. They also keep a failed definition title as the title of the link, and cap a link label at 1000 bytes, not 999 characters. The spec text and commonmark.js agree on all three. The user asked to prefer the spec and the result users expect, 2026-09-13. `dialect.md` has the rows. |
| Canonical style by consensus | The style follows modern best practice across the major formatters and style guides, not personal preference. The survey of 2026-09-13 read the source and docs of Prettier 3.9.6, dprint-plugin-markdown 0.24.0 (`deno fmt` pins 0.20.0), mdformat 1.0.0 with mdformat-gfm 1.0.0, markdownlint 0.41.1, and the Google Markdown style guide at `895579e`. A source that keeps the input form, or accepts any consistent form, has no vote. The rows below give the votes. Design appendix B lists the exceptions that keep meaning. |
| A directory path gives its `.md` and `.markdown` files, in lexical order, outside hidden files and directories and the `--exclude` patterns. A file path is formatted whatever its extension. | `.md` and `.markdown` are the extensions that GitHub and most tools read as Markdown. gofmt also skips names that start with a dot, and hidden directories hold tool state. Skips by name such as `testdata` or `node_modules` guess at a project's layout, so `--exclude` names them. A pattern matches a name or the printed path with `filepath.Match`. A path named on the command line is walked even when a skip would match it. |
| A block whose canonical form fails the check keeps the bytes of the input | The owner's principle, 2026-09-18: format what can be formatted, and pass through what cannot, without breaking the document. The check names a node, the block of the document that holds it prints as the input holds it, and the check runs again, at most 4 times; the whole input stays as it is after that. `format.Strict` keeps the error for the printer tests, so a block that keeps its bytes still fails `FuzzFormat`. Design 10. |
| The runtime check compares `Kept` as well as `Equal` | `Equal` reads values, where GitHub can read the bytes of an escape, an entity, a label or an invalid byte. The owner chose runtime checks that are cheap: 7.7% on unformatted input, and none on formatted input, which skips the check. |
| The CLI formats files at once while their input bytes stay within the input limit, and each file counts at least the limit divided by `GOMAXPROCS` | Peak memory follows the input bytes being formatted (design 7.2), so the budget of one largest input holds for a run. The CLI reads at most one byte past the limit, and counts the bytes it read. The floor bounds the files at once to `GOMAXPROCS`, which follows a container's CPU limit. A check of `~/Development` (1,383 unformatted files) took 1.0 s and 28 MB, 2026-09-17. |
| Bullet `-`, and `*` for the next adjacent sibling list | Prettier (`print/list.js`), dprint (`generate.rs`), mdformat (`renderer/_util.py`). Google uses `*`. markdownlint: consistent. |
| Ordered delimiter `.`, and `)` for the next adjacent sibling list | Prettier, dprint, mdformat. Google and markdownlint have no rule. |
| A list item that keeps its input marker writes its list's bullet or delimiter | The sign decides where a list ends (spec 5.3). Alternation is the only thing that keeps adjacent lists apart, because a `<!-- -->` separator adds a node (appendix B, trap 3). Every sign is one column wide, so the kept columns do not change. Design 12. |
| Ordered numbering counts up from the start number. A list that starts at 1 and whose second item is 1 numbers every item 1 | No majority. Prettier and dprint count up and keep this lazy form; mdformat numbers every item after the first 1; Google and markdownlint accept both. The user chose the rule of Prettier and dprint, 2026-09-13. The rule is dprint's: it is idempotent for every start number. The lazy form is kept syntax (design 12). |
| Emphasis `_`, and `*` where `_` would parse differently; strong emphasis `**` | Prettier (`print/mdast.js`), dprint (`resolve_config.rs`). mdformat keeps the source. markdownlint: consistent. Google has no rule. |
| Fenced code blocks with backticks, or tildes when the info string has a backtick. The fence is 3 long, or one longer than the longest run of its character in the content | Fenced: mdformat, Google ("we strongly recommend fencing"); Prettier and dprint keep the source form. Fence: Prettier, dprint, mdformat (`renderer/_context.py`). |
| Thematic break `---` | Prettier, dprint. mdformat writes 70 `_`. markdownlint: consistent. Google has no rule. |
| Hard line break `\` | dprint, mdformat, Google ("Use a trailing backslash to break lines"). Prettier keeps the source form. markdownlint allows 2 spaces. |
| Tables with columns aligned by display width, outer pipes, and delimiter cells of at least 3 dashes with their colons. A right-aligned column pads before its cells, a centered column pads half before (rounded down) and the rest after, and other columns pad after. A W or F character of Unicode `EastAsianWidth.txt` is 2 columns wide, and a control character or a nonspacing or enclosing mark is 0 | Prettier (`print/table.js`, `utilities/get-string-width.js`), dprint (`TableCellPadding::Align`, which measures with `unicode-width`), mdformat-gfm (`plugin.py`). markdownlint: any. Google has no rule. |
| ATX headings without a closing sequence | dprint, mdformat, Google. Prettier 3.9 writes ATX, and keeps setext headings. |
| List item content indented by the marker width plus 1 | Prettier, dprint, mdformat, markdownlint (MD030). Google indents 4 columns. |
| Escapes and entity references as written. The printer adds and removes none | Prettier and dprint keep them; mdformat decodes them. The printer uses indentation and syntax choices where the others add escapes, so `Kept` stays exact (design 12). The user chose this over added escapes, 2026-09-13. |
| No byte order mark in the output | A BOM is not meaning: cmark and GitHub strip it (design 4.1), and UTF-8 needs no byte order. |
| No blank line between adjacent link reference definitions, except in a loose list item | Prettier writes them this way (`print/children.js`). A blank line between them is not meaning, except between the children of a list item, where it can make the list loose (spec 5.3). |
| A table stays directly below a paragraph that it split off when the paragraph starts with `[` | After a blank line, the paragraph would start with a link reference definition (design 5.4). Other paragraphs get a blank line before the table. |
| Task boxes `[ ]` and `[x]`, with one space after them | GitHub reads `x` and `X` as checked, and its docs write `[x]`. |
| A list item's content starts on its marker line, unless the content starts with columns that padding would take | Padding takes up to 4 columns, so such content keeps a blank first line (spec 5.2). A block quote writes its content on its marker line. |
| A list's padding grows past the indentation of an HTML block or code block after the list, and past a list whose first item keeps the columns of its input marker | Otherwise the block would continue the last item. When more than 4 columns of padding would be needed, the list keeps its input layout. |
| Strikethrough `~~` | GitHub reads `~` and `~~` the same, and its docs write `~~`. No `~` goes next to a `~~` delimiter (design appendix B, trap 13). |
| Emphasis and strikethrough keep their input delimiters where the canonical delimiter could pair differently, or could change a construct next to them | `_` flanks differently from `*` inside words and next to symbols (spec 6.2, trap 19). A node whose content has a character of its delimiters keeps them, so that a second format decides the same. An underscore in the last two segments of a domain keeps an extended autolink from forming (design 6.2), so a node in such a word keeps its delimiters. |
| A code span has the shortest fence that its value does not hold, and one space of padding where the input has it or the value needs it | Prettier chooses the shortest run that the content does not hold (`print/mdast.js`). A code span over lines or with a cell pipe escape keeps its bytes. Padding stays because its space can keep the text around the code span from forming an inline link destination or a definition, which hold no space (appendix B, trap 22). Changed 2026-09-16. |
| An inline link has no line break in its parentheses, one space after the opening parenthesis, and before the closing parenthesis of a link without a title, where the input has whitespace there, and one space before its title | The title uses the quotes of a definition title. The destination keeps its bytes and angle brackets. The spaces inside the parentheses stay because a destination that starts before the link could otherwise run through it (appendix B, trap 22). Changed 2026-09-16. |
| The printer keeps each NUL and invalid UTF-8 byte | cmark writes an invalid byte in a destination as `%A6`, where markfmt's value is U+FFFD, so `Equal` cannot see the byte. `Kept` checks it (design 12). |
| Tab expansion is bounded: at most 2.86 bytes of output per input byte | The printer writes structural indentation as spaces, and tab-indented code in a block quote or a list item as a fenced block (design appendix B, trap 20). It is the largest output-to-input ratio of the corpora and the cases, which `TestFormatSource` reports. An input of such lines near the 8 MiB input limit passes the 16 MiB output limit, and `Format` returns an error instead of output. |
| One blank line between blocks, one final newline, line breaks as written | Prettier (`proseWrap: "preserve"`), dprint (`TextWrap::Maintain`), mdformat (`wrap: keep`). markdownlint MD012 and MD047 agree. |

## Open decisions

None.

## Plan

Each stage has gates. A stage is done only when its gates pass. Mark the
checkbox in the same commit that passes the gates. Design 15 sketches the
commits for stages 1 to 6.

### Stage 0: design

- [x] Review the goldmark v2 design and record what we take, what we drop,
  and why. Design appendix C.
- [x] Write the parser design in `docs/design/parser.md`, from first
  principles for a formatter.
- [x] Settle the grammar scope, the test-only HTML renderer and the design
  document location. See Decisions.

Gate: the user approves the design document. Approved 2026-09-12.

### Stage 1: harness

- [x] Tree, builder with always-on checks, `Verify`, and `FuzzParse`
  (lossless round trip, no panic). No per-input timer in fuzzing.
- [x] `.golangci.yml`: `exhaustive` with `explicit-exhaustive-switch: true`
  and `default-signifies-exhaustive: false`. `task fuzz` takes `PKG` and
  `FUZZ`.
- [x] Line iterator, LineEnding and BOM leaves.
- [x] A reader for `spec.txt` example blocks, with an example count test per
  pinned file.
- [x] Pin the corpora in `internal/markdown/testdata` with a notice for each.
  See Corpora.
- [x] The test-only HTML renderer and `normalize.py` normalization.
- [x] A conformance runner with `failing.txt` per corpus (design 11.2): the
  test fails if an unlisted example fails, if a listed example passes, or if
  an entry names no example. The list can only get shorter.

Gate: `task ci` passes with every example on the expected-failure list.
Passed 2026-09-12.

### Stage 2: block structure

- [x] Paragraphs, blank lines, thematic breaks, ATX and setext headings,
  indented and fenced code, HTML blocks (all 7 kinds), with their `dialect.md`
  rows.
- [x] Block quotes, lazy lines, prefix leaves and emission order; tabs and
  `virt`; list items and lists with looseness.
- [x] Link reference definitions, labels, and the setext re-dispatch rule.
- [x] Front matter, `grammar-differs.txt` and `markfmt/grammar.txt`.
- [x] `Equal` for block kinds, the stage 2 pairs, `FuzzEqual` with block
  mutations.
- [x] Pathological block inputs, the long test and `task long` in the ubuntu
  CI job.

Gates:

- The lossless fuzz test holds.
- Every block-section example that does not need inlines passes, or is in
  `grammar-differs.txt` and its named case passes (250 of 296, by the
  mechanical classification in design 11.2).
- The pathological subtest of `TestParse` passes, including `- `×n `a` and
  deep lists with blank lines. `task long` passes.
- `Equal` passes the stage 2 pairs, and `FuzzEqual` finds no false acceptance.

Passed 2026-09-13. `FuzzParse` and `FuzzEqual` ran 90 seconds each with no
finding. At the input limit, `task long` takes at most 0.6 s and 1.5 GiB of
memory per input, for nested block quotes; the stage 2 bound is 2 GiB (design
11.1).

### Stage 3: inlines

- [x] Backslash escapes, entity and numeric character references (table
  generated from WHATWG `entities.json`), code spans.
- [x] Emphasis and strong emphasis with the delimiter run algorithm.
- [x] Links, images, reference links, autolinks, raw HTML, with pass 1 and the
  pass label check.
- [x] Hard and soft line breaks.
- [x] `Equal`, the pairs and the `FuzzEqual` mutations extended with each
  construct.

Gates:

- CommonMark 0.31.2, cmark-gfm and commonmark.js regression corpora at 100%:
  every example passes, or is in `grammar-differs.txt` and its named case
  passes. cmark-gfm regression examples tagged with GFM extensions count at
  stage 4.
- Pathological inline inputs pass the pathological subtest and `task long`.
- `benchstat` output recorded here: parse throughput within 2 times goldmark's,
  with pass 1 included.

Passed 2026-09-13. The CommonMark and commonmark.js regression `failing.txt`
lists are empty. commonmark.js regression 25 is in `grammar-differs.txt`, and
its named case passes. The cmark-gfm regression list holds only examples
tagged with GFM extensions: 8, 10, 11, 13 and 20 to 23. The pathological
subtest has 25 inputs, and each linear-time mechanism of design 6.8 was
checked with a mutation. At the input limit, `task long` takes at most 0.62 s
and 1.34 GiB of memory per input, for nested block quotes. `FuzzParse` and
`FuzzEqual` ran 90 seconds each with no finding. `BenchmarkParse` on an Apple
M2 Pro, with `go run golang.org/x/perf/cmd/benchstat@v0.0.0-20260908200009-22c9c6c9d4da -col /parser`:

```
                       │   markfmt   │               goldmark               │
                       │   sec/op    │    sec/op     vs base                │
Parse/input=spec-10      1.249m ± 1%    1.504m ± 0%  +20.42% (p=0.000 n=10)
Parse/input=corpora-10   943.5µ ± 0%   1873.3µ ± 3%  +98.54% (p=0.000 n=10)
Parse/input=design-10    405.3µ ± 1%    532.6µ ± 0%  +31.42% (p=0.000 n=10)
geomean                  781.7µ         1.145m       +46.47%

                       │   markfmt    │               goldmark               │
                       │     B/s      │     B/s       vs base                │
Parse/input=spec-10      156.5Mi ± 1%   130.0Mi ± 0%  -16.95% (p=0.000 n=10)
Parse/input=corpora-10   41.37Mi ± 0%   20.83Mi ± 3%  -49.64% (p=0.000 n=10)
Parse/input=design-10    175.5Mi ± 1%   133.5Mi ± 0%  -23.91% (p=0.000 n=10)
geomean                  104.3Mi        71.24Mi       -31.73%
```

markfmt parses faster than goldmark on every input.

### Stage 4: GFM and GitHub syntax

The scope is what GitHub renders, because most users expect it.

- [x] GFM extensions: tables, strikethrough, task list items (design 6.5),
  extended autolinks.
- [x] Footnote definitions and references (design 6.3, 9.2).
- [x] GitHub fixtures for footnotes, tasks, tables, strikethrough and every
  `dialect.md` row, with the GitHub normalizer.
- [x] The test renderer applies the GFM tag filter (design 11.3).
- [x] `grammar-differs.txt` entries for regression examples that GFM rules
  change (design 11.2). No regression example changes; extended autolinks
  change CommonMark 602, 606, 608, 611 and 612.
- [x] Capture math and alert fixtures for stage 6. Math and alerts are GitHub
  HTML filters, not grammar (design 9.1, 9.3).
- [x] Plain text that GitHub gives meaning to needs no grammar, but escaping
  must never change it: emoji shortcodes (`:+1:`), mentions (`@user`) and
  issue references (`#1`). The filters read decoded text (design 9.4).

Gates:

- The GFM extension examples and cmark-gfm `extensions.txt` at 100%. Take
  only extension examples from the GFM spec: it is pinned at 0.29, and its
  copies of core examples are older than CommonMark 0.31.2.
- The cmark-gfm regression examples tagged with GFM extensions at 100%.
- Footnote, task, table and strikethrough fixtures at 100%.

Passed 2026-09-13. The GFM, cmark-gfm extensions, cmark-gfm regression and
GitHub `failing.txt` lists are empty. The GitHub fixtures of the dialect rows
are in `github/grammar-differs.txt`, and their named cases pass. The long test
of the gate found that the per-line indentation memo of design 5.1 was
missing: nested list items indented line by line took 7.3 s at the input
limit. `29668ad` adds it. The pathological subtest has 41 inputs, and each
new linear-time mechanism was checked with a mutation. At the input limit,
`task long` takes at most 0.82 s per input, for a table header of many cells
with one-cell rows, and at most 1.50 GiB of memory, for unmatched link
openers. `FuzzParse` and `FuzzEqual` ran 90 seconds each with no finding.

### Stage 5: differential fuzzing

- [x] `internal/markdown/differential_test.go` fuzzes our parser against
  goldmark v2.0.2 with no extensions and compares test HTML (design 11.5).
  goldmark is a test-only requirement of the root module until stage 7. No
  release happens before stage 7.
- [x] The fuzzer skips inputs in markfmt's grammar outside CommonMark: GFM,
  GitHub footnotes and front matter.
- [x] One predicate per row of Known goldmark deviations that shows in HTML,
  so the fuzzer skips known deviations.
- [x] Triage every disagreement against the spec text. Save each one as a
  permanent case in `testdata/differential/cases.txt`, marked "fixed in
  markfmt" or "goldmark deviates, spec section X".

Gate: a `FuzzDifferential` run of 1 CPU-hour (workers × wall time) finds no
disagreement that has not been triaged.

Passed 2026-09-13. After `213833f`, a run of 6 minutes with 10 workers found
no disagreement in 34.8 million inputs. `cases.txt` has 49 cases: 42 goldmark
deviations, each with its predicate and its Known goldmark deviations row, and
7 fixed in markfmt. The markfmt fixes follow cmark where the spec text has no
example (design 8.4): NUL as U+FFFD in code, HTML blocks, autolinks and
destinations, control characters in destinations, kind 7 open tags with kind 1
names, and VT and FF in info strings, HTML tags and link labels. The code span
fix follows the spec text, not cmark. Six new `dialect.md` rows have no GitHub
fixture yet.

### Stage 6: printers on the new tree

- [x] Settle the canonical style. See Open decisions.
- [x] A printer for every node kind, meeting design 12: facts through
  accessors, kept syntax and `Kept`, size decisions on canonical measures, and
  the output limit.
- [x] Dialect predicates and spans in `Equal` (design 10.4).
- [ ] `FuzzFormat`: no check mismatch on any input, and equal test HTML for
  input and output (design 10.5). Nine findings of 2026-09-17 are fixed, each
  with a case and a seed: the kept blank lines of a quote, a kept marker at
  the column where its list continues, a definition below a definition, the
  padding of a lazy line, the first line of code that prints as a fence, a
  link tail line ending, the indentation of a span line with a tab, the sign
  of a kept marker, and a table below the paragraph that it split.

  Five more findings of 2026-09-18 are fixed with the column model: the
  printer now knows the column it writes at and the column each container
  continues on, so a kept marker moves left to clear the list above it and
  pads itself back to its own columns. The others are a marker below a list
  that a block separates, a block's first line in both hard break forms, the
  bullet risk of a table that prints with pipes, and the indentation of a
  label line.

  Twenty-five minutes of fuzzing then gives `"* 0\r--\n  |-"`: an item holds
  a paragraph and a table whose header row is `--`. The header prints as
  `| --  |`, which reads as the delimiter row of the paragraph above it, so
  the paragraph becomes the header. Every layout fails: a blank line between
  them makes the list loose, printing the table as written gives `--`, which
  reads as a delimiter row as well, and a delimiter cell takes only spaces,
  dashes and colons, so no padding breaks the shape. `Source` returns an
  error, so no output is wrong.

  The owner chose on 2026-09-18 to pass such a block through: `Source`
  keeps the bytes of the block that the check names, and `Strict` still
  fails, so the finding stays open for the printer.

  On 4,000 real files (39 MB of module caches) the branch gives the output of
  main, byte for byte, with no error, and formatting them again changes
  nothing (2026-09-17).
- [x] Move `internal/format` to the new parser and delete the goldmark-based
  code. Keep `testdata/cases` and make every case pass.

Gates: `testdata/cases` pass. Fuzzing shows idempotence and no check mismatch.
Math and alert fixtures pass as printer cases. `go list -deps ./cmd/markfmt`
lists no goldmark package.

All gates pass except the fuzzing gate. `FuzzFormat` still finds a failure
within about 15 seconds on adversarial input, and each is a distinct small
bug. On 10,296 real Markdown files on the user's machine (module caches and
repositories), markfmt gave no error and no unstable output (2026-09-16). The
user chose on 2026-09-16 to go on to stages 7 and 8 and to come back to the
fuzzing gate after them. The real-file check formats each file twice and
compares the two outputs.

### Stage 7: remove goldmark

Remove the differential test when all of these are true:

- [x] All corpora at 100%: every example passes, or is listed in a
  `grammar-differs.txt` and its named case passes. Every `failing.txt` is
  empty.
- [x] A `FuzzDifferential` run of about 2 CPU-hours finds no disagreement
  that has not been triaged (Decisions).
- [x] Every disagreement is a permanent case in `testdata`.

Gate: `differential_test.go` is deleted and `go mod tidy` has run. No goldmark
import anywhere. The root `go.mod` has no requirements. goldmark's copied test
files can stay as data, with their MIT notice.

### Stage 8: product

- [x] CLI directory walking that skips hidden files and directories and the
  `--exclude` patterns. Concurrent work is limited by input bytes.
- [x] Release setup: goreleaser, version stamping, `release.yml`, Homebrew
  tap. Follow hamnir's conventions. v0.0.1 released on 2026-09-17: archives,
  signed checksums, SBOMs, attestations, the GHCR image and the cask all
  checked from outside, and `go install ...@latest` works.
  - [x] goreleaser, `-version` stamping, `release.yml`, the GHCR image, SBOMs
    and cosign signing, as hamnir has them. `task release-snapshot` builds
    every archive and both images (2026-09-16).
  - [x] Homebrew tap. `strangemattersystems/homebrew-tap` holds the cask.
    goreleaser pushes to it over SSH with a write deploy key of that
    repository, whose private key is the `HOMEBREW_TAP_DEPLOY_KEY` secret of
    this one. A prerelease tag publishes no cask (2026-09-17). The binaries
    are not notarized, so the cask removes the quarantine attribute on
    install, which bypasses Gatekeeper's check; notarizing needs an Apple
    Developer account and would replace the hook.
  - [x] Before the first tag, the repository must be public: `gomod.proxy`
    and `task warm-proxy` read the module from proxy.golang.org, which cannot
    fetch a private module. Public since 2026-09-17, after a gitleaks scan of
    the whole history.
- [x] markfmt.com. Cloudflare Pages serves `website/` and deploys each push
  to `main` through the Cloudflare GitHub app. `www.markfmt.com` is a proxied
  `AAAA 100::` record with a 301 redirect rule to the apex, and Always Use
  HTTPS is on (2026-09-17). The site's download links use
  `releases/latest/download`, so release archives carry no version.

## Testing strategy

| Property | Test |
| --- | --- |
| Lossless tree | `FuzzParse` and `Verify`: printing the tree unchanged gives the input byte for byte. Needs no oracle. |
| No panic, linear time | The pathological and long subtests of `TestParse`, and `task long` (design 11.1). |
| Spec conformance | Corpus runner with `failing.txt` and `grammar-differs.txt`. |
| Runtime check is sound | The pair corpus, checked against test HTML, and `FuzzEqual` (design 10.5). |
| Agreement with a second parser | Differential fuzzing against goldmark, stages 5 to 7 only. |
| GitHub syntax | Fixtures captured from the GitHub Markdown API. Compare document structure, not GitHub's extra attributes. |
| Printer is idempotent | Fuzz, `Kept`, and `testdata/cases`. |
| Printer keeps meaning | `FuzzFormat` and `testdata/cases`. |

## Corpora

| Corpus | Source | Version | License | Use |
| --- | --- | --- | --- | --- |
| CommonMark spec examples | `commonmark/commonmark-spec` `spec.txt` | 0.31.2 | CC BY-SA 4.0 | Core conformance |
| CommonMark HTML normalization | `commonmark/commonmark-spec` `test/normalize.py` | 0.31.2 | BSD-2-Clause | Design reference for test HTML comparison |
| GFM spec extension examples | `github/cmark-gfm` `test/spec.txt` | 0.29, tag `0.29.0.gfm.13` | CC BY-SA 4.0 | GFM conformance, extension examples only |
| cmark-gfm extension tests | `github/cmark-gfm` `test/extensions.txt` | tag `0.29.0.gfm.13` | CC BY-SA 4.0 | GFM conformance |
| cmark-gfm regression tests | `github/cmark-gfm` `test/regression.txt` | tag `0.29.0.gfm.13` | BSD-2-Clause | Regressions |
| cmark pathological inputs | `github/cmark-gfm` `test/pathological_tests.py` | pin at import | BSD-2-Clause | Linear-time tests |
| commonmark.js regressions | `commonmark/commonmark.js` `test/regression.txt` | tag `0.31.2` | BSD-2-Clause | Regressions |
| goldmark cases | `yuin/goldmark` `_test/extra.txt`, `extension/_test/*.txt` | v2.0.2 | MIT | Extra edge cases, already in `internal/format/testdata/spec` |
| markdown-it fixtures | `markdown-it/markdown-it` `test/fixtures` | optional | MIT | Extra cases |
| HTML entities | WHATWG `entities.json` | fetched 2026-09-13, `Last-Modified` 2025-11-12 | CC BY 4.0 | Generate the entity table |
| Unicode case folding | Unicode `CaseFolding.txt` | 17.0.0, the version of Go 1.27's `unicode` package | Unicode License v3 | Generate the full case folding table for labels |
| GitHub docs Markdown examples | `github/docs` `content/get-started/writing-on-github` | commits `6ed0ad8`, `5e4bf35` | CC BY 4.0 | Footnote, math and alert cases |
| GitHub Markdown API output | `POST /markdown` with `mode=gfm` | captured 2026-09-13 | GitHub API terms | Expected results for GitHub syntax, captured into fixtures; capture again before each release |

## Licensing rules

This is our reading of the licenses, not legal advice.

- Read goldmark for design ideas only. Write our own code. If we ever port
  real code, keep goldmark's MIT copyright notice with it.
- Implement the algorithm that the CommonMark spec describes. Ideas are not
  covered by copyright; the spec text is CC BY-SA 4.0.
- Give every copied corpus a README or NOTICE with its source, version and
  license, as `internal/format/testdata/spec/README.md` does.
- Generate tables from primary sources: WHATWG `entities.json`, Unicode
  `CaseFolding.txt`, and Go's `unicode` package for punctuation.
- Send only test inputs written for the purpose to the GitHub Markdown API,
  never user documents.

## Known goldmark deviations

Stage 7 removed goldmark. This list stays as the record of how
`testdata/differential/cases.txt` was triaged.

Use this list to triage stage 5 disagreements. The last column says how
`FuzzDifferential` handles each row (design 11.5).

| Input | goldmark behaviour | Expected | Differential test |
| --- | --- | --- | --- |
| Table directly after a paragraph | The table gets the paragraph's position. | Position of the header row. | Positions are not in HTML. |
| `0\n0\n    -\n0\n-` | Block positions out of order. | Positions in source order. | Positions are not in HTML. |
| Paragraph with trailing spaces, then a table | HTML keeps the space: `<p>0 </p>`. | No trailing space. | A table is markfmt grammar. |
| HTML block at end of input without a final newline | HTML differs from the same input with a final newline. | The same HTML. | goldmark reads a final line ending. |
| Lone CR | Not a line ending. | A line ending (CommonMark). | goldmark reads LF line endings. |
| `*\r\n` | A paragraph. | An empty list item. | goldmark reads LF line endings. |
| `[\xa6]: /u\n\n[\ufffd]` | No link: an invalid UTF-8 byte does not match U+FFFD in a label. | A link, as cmark gives (design 6.7, 8.4). | goldmark reads U+FFFD for invalid UTF-8. |
| `[a](/\x00)` | The link destination `/%00`. | `/%EF%BF%BD`: NUL is U+FFFD (spec 2.3). | goldmark reads U+FFFD for NUL. |
| `a <b c="\ufffd">` | Text: an attribute value cannot contain U+FFFD. | Raw HTML (spec 6.6). | Predicate, differential case. |
| `a *`, a vertical tab, `a*` | Text: a vertical tab, U+0085, U+2028 and U+2029 are whitespace for flanking, as for Go's `unicode.IsSpace`. | Emphasis (spec 2.1, 6.2). | Predicate, differential case. |
| `[](a(b )` | A link to `a(b`: a destination can end inside an open parenthesis. | Text (spec 4.7, 6.3). | Predicate, differential case. |
| A fence indented 2 spaces, then a line of 1 space | The code is the space: a line of spaces keeps what the fence indentation should remove. | Empty code (spec 4.5). | Predicate, differential case. |
| `* a`, then a tab, `*`, two tabs and `0` | The code `0`: tabs after a list item prefix or a nested list marker stop at columns counted from the item's content. | The code `  0` (spec 2.2, 5.2). | Predicate, differential case. |
| `* `, then `   - b` | Two lists: a line that starts like a bullet list item does not go into the empty item. | A nested list (spec 5.2). | Predicate, differential case. |
| `[a](b`, a form feed, `c)` | A link: a form feed does not end a destination. | Text, as cmark gives (spec 4.7, 6.3). | Predicate, differential case. |
| `<pre/>`, a blank line, `b` | One HTML block of kind 1 to the end: kind 1 starts at `<pre/`. | HTML block kind 7, then paragraph `b` (spec 4.6). | Predicate, differential case. |
| `~~~`, then a form feed | The class `language-` and the form feed: VT and FF stay in an info string. | No class, as cmark trims VT and FF (spec 4.5, design 8.4). | Predicate, differential case. |
| `~~~0`, a tab, `0` | The class `language-0`, a tab and `0`: the first word ends only at a space. | The class `language-0` (spec 4.5). | Predicate, differential case. |
| `~~~ &#32;a&#32;` | The class `language-`: decoded whitespace stays in an info string. | The class `language-a`, as cmark trims after decoding (spec 4.5, design 8.4). | Predicate, differential case. |
| `*`, a line of a tab, `  0` | An empty item, then paragraph `0`, as commonmark.js gives. | The item `0`, as cmark gives (spec 5.2, design 5.1). | Predicate, differential case. |
| `a`, three backslashes, a line ending, `b` | `a`, two backslashes and a soft break. | `a`, a backslash and a hard line break (spec 2.4, 6.7). | Predicate, differential case. |
| `- -`, a blank line, `  b` | The paragraph `b` after the list: the outer item ends. | The paragraph `b` in the outer item (spec 5.2). | Predicate, differential case. |
| `>0*0`, then `>*` | Emphasis: the `>` before the second `*` counts as the character before it. | Text: a line ending comes before the `*` (spec 6.2). | Predicate, differential case. |
| `0<?>` | Raw HTML: `<?` and `?>` share the `?`. | Text (spec 6.6). | Predicate, differential case. |
| `<A A=`, U+0014, `>` | Text: an unquoted attribute value cannot hold a control character. | HTML block (spec 6.6). | Predicate, differential case. |
| `</ A0>` | An HTML block: whitespace can follow `</`. | Paragraph (spec 6.6). | Predicate, differential case. |
| `<p`, then a tab | A paragraph: kind 6 does not start at a tab after the tag name. | HTML block (spec 4.6). | Predicate, differential case. |
| `![a`, a line ending, `b](/u)` | The alt text `a`, a line ending, `b`. Raw HTML, code spans and autolinks are left out of the alt text. | The alt text `a b`, as cmark writes it (spec 6.4). | Predicate, differential case. |
| `[foo]: /url`, then `"title"ok`, then `[foo]` | A link with the title `title`, and the paragraph `"title"ok`. | A link without a title, as commonmark.js gives (spec 4.7, CM 210). | Predicate, differential case. |
| A definition alone with a label of 1001 characters, or a reference label of 500 `é` | No output for the definition: a definition takes a label of any length. No link from the reference: a reference label has at most 999 bytes. | The paragraph with the text, and a link: a label has at most 999 characters (spec 4.7, 6.3). | Predicate, differential case. |
| `* [a]: /u`, then `  b` | A loose list: a definition and a paragraph are two blocks. | A tight list (spec 5.3). | Predicate, differential case. |
| `a`, a backslash, two spaces, then `\!` | The escape `\!` as written. | `!` (spec 2.4, 6.7). | Predicate, differential case. |
| `<A>`, then a tab | A paragraph: a tab or FF after the tag is not whitespace for kind 7. | HTML block (spec 4.6). | Predicate, differential case. |
| 1000 `[`, then `a](b)` | Text: no link forms when about 1000 bytes lie between the first and the last open bracket before it. | A link (spec 6.3). | Predicate, differential case. |
| `[a](<b>"t")` | A link with the title `t`: no whitespace is needed after an angle destination. | Text (spec 6.3). | Predicate, differential case. |
| `</A/>` | An HTML block: a closing tag can end with `/>`. | Paragraph (spec 6.6). | Predicate, differential case. |
| `<div`, then a form feed | A paragraph: FF is not whitespace in a tag, except after a kind 1 tag name. | An HTML block, as cmark reads FF (spec 6.6, design 8.4). | Predicate, differential case. |
| `[a b]`, then `[a`, a form feed, `b]: /u` | Text: FF inside a label is a label character. A label of FF alone is a label. | A link, and a blank label, as cmark reads FF (spec 4.7, design 8.4). | Predicate, differential case. |
| `> `, a tab, `*` | The paragraph `*`: the tab after the marker's space counts from the quote's content, also before a setext underline. | An empty list item (spec 2.2, 5.2). | Predicate, differential case. |
| `- x`, then indented code with a line of spaces | The line is empty, or gone from an HTML block: goldmark drops the spaces beyond the indentation in a list item. | The spaces stay (spec 4.4, 5.2). | Predicate, differential case. |
| `<` and a scheme of 33 letters, then `:h>` | An autolink: a scheme has no length cap. | Text (spec 6.5). | Predicate, differential case. |
| `<a b`, a tab, `>` | A paragraph: a tab before the `>` of a block tag is not whitespace. | An HTML block (spec 6.6). | Predicate, differential case. |
| `[0]:\n0\n''0` | The destination line also appears as paragraph text. | Paragraph `''0` only (compare example 210). | Predicate, differential case. |
| `x<!x>`, `<!doctype html>` | Text: a declaration needs an uppercase letter after `<!`, as on GitHub. | Raw HTML and an HTML block (spec 4.6, 6.6). | Predicate, differential case. |
| `a\n<meta>` | `<meta>` interrupts the paragraph: goldmark's HTML block kind 6 tag list has `meta`. | Paragraph with raw HTML (spec 4.6). | Predicate, differential case. |
| `</script>` | A paragraph with raw HTML: no HTML block of kind 7 starts at a closing tag of `pre`, `script` or `style`. | An HTML block (spec 4.6). | Predicate, differential case. |
| `[a](<b<c>)` | A link: an angle destination can contain `<`. | Text (spec 4.7, 6.3). | Predicate, differential case. |
| `[foo]: /url\n---` | A thematic break, as commonmark.js gives, and an empty list item for `-`. | Paragraph `---`, as cmark gives (`dialect.md`). | Predicate, differential case. |
| ``- ```\n  a\n\n- b`` | A loose list, as commonmark.js gives. | A tight list, as cmark gives (spec 5.3, design 2). | Predicate, differential case. |
| ```` ```language-r ```` | The class `language-language-r`. | The class `language-r`, as cmark writes (spec 4.5). | Predicate, differential case. |
| `* >+`, then `  >` twice | A second, empty block quote after the list: the outer list item ends at the last `>` line. | One block quote in the item (spec 5.1, 5.2), as commonmark.js gives. | Differential case, found by the last stage 7 run after the predicates went. |

## How to resume

1. Read this document, then `docs/design/parser.md`.
2. Run `git log --oneline` and `git status`.
3. Run `task ci`. It must pass before new work starts.
4. Find the first unchecked item in the plan, or an open decision that blocks
   it.
5. Update this document in the same commit as the work.
