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

Last updated: 2026-09-13. Nothing after `ff7de5b` is pushed.

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

Layout:

- `cmd/markfmt`: the CLI. `markfmt [-check] [path ...]`. With no path or `-`,
  it reads stdin and writes stdout. It writes files atomically.
- `markfmt.go`: the public API, `Format(w io.Writer, r io.Reader) error`.
- `internal/markdown`: the new parser and its tree (stage 1 onwards).
- `internal/markdown/testdata/<corpus>`: the conformance corpora `commonmark`,
  `gfm`, `cmark-gfm-extensions`, `cmark-gfm-regression` and
  `commonmark-js-regression`, each with a README and `failing.txt`.
- `internal/markdown/testdata/dialect.md`: the rules where GitHub and
  CommonMark 0.31.2 differ, one row each (design 2.1).
- `internal/markdown/testdata/github`: GitHub Markdown API fixtures, their
  inputs and the capture loop (design 11.4). `github.txt` runs as conformance
  after the GitHub normalizer; `printer.txt` holds math, alert and plain text
  fixtures for stage 6.
- `internal/markdown/testdata/markfmt/grammar.txt`: markfmt's own cases, with
  the expected HTML of each example that a `grammar-differs.txt` lists
  (design 11.2). `testdata/commonmark/grammar-differs.txt` lists CM 96 and 98
  (front matter). `testdata/commonmark-js-regression/grammar-differs.txt`
  lists regression example 25, where cmark and commonmark.js disagree on list
  looseness and markfmt follows cmark.
- `internal/markdown/testdata/pairs`: the pair corpus of `TestEqual`, each
  pair equal or different with a reason, checked against the test HTML
  (design 10.5).
- `internal/markdown/testdata/unicode/CaseFolding.txt`: the source of the
  generated full case folding table.
- `internal/markdown/testdata/entities/entities.json`: the source of the
  generated entity table.
- `internal/format`: the goldmark-based formatter. It formats headings (ATX)
  and paragraphs, copies other blocks from source, passes front matter
  through, and compares goldmark HTML of input and output.
- `internal/format/testdata/cases`: 26 `NAME.in.md` and `NAME.out.md` pairs.
- `internal/format/testdata/spec`: CommonMark 0.31.2 examples (goldmark's
  `spec.json`) and goldmark's extra and GFM case files.
- `internal/format/testdata/fuzz/FuzzSource`: 9 inputs that the fuzzer found.
- `tools/go.mod`: golangci-lint v2.13.2, kept out of the root `go.mod`.
- `docs/design/parser.md`: the parser design.
- `.scratch/design-research/` (local only, not tracked): the research reports
  and the five rounds of design reviews behind the parser design.

The goldmark-based formatter is a stopgap. Stage 6 replaces it.

Open fuzz finding, not fixed and not in `testdata`: `"[0]:\n0\n''0"` is
rejected. goldmark repeats the link destination as paragraph text; compare
CommonMark example 210. Our own parser makes this finding obsolete.

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
| Dialect rows and predicates | Where GitHub and CommonMark 0.31.2 disagree, a `dialect.md` row records it. At stage 6 a predicate finds each span, `Equal` compares spans both ways with their bytes, and the printer keeps them. Design 2.1, 10.4. |
| Input limit 8 MiB, output limit 16 MiB | Constants, not options, chosen from a 4 GiB worst-case memory budget. Design 7.2. |
| `Format` recovers panics | A parser or printer bug returns an error and writes nothing, so one bad file does not stop the CLI. Design 1. |
| One grammar in production and tests | No test-only grammar switch. Core examples that markfmt's GFM or front matter rules change are listed in `grammar-differs.txt`, each with a case of markfmt's expected result. Design 11.2. |
| Streaming not planned | The documents that would need it are one top-level block, so per-block streaming would not help. Design 7.4. |
| Design document at `docs/design/parser.md` | It is reviewed and versioned with the code. |
| `grammar-differs.txt` also lists corpus examples where cmark and commonmark.js disagree | markfmt follows cmark (design 2). The named case holds the output of `cmark --unsafe`. commonmark.js regression 25: cmark 0.31.1 makes a list loose after a blank line in an HTML block. |
| Full case folding table from Unicode `CaseFolding.txt` | Label matching needs Unicode full case folding (CM 540: `ẞ` matches `SS`). Go's `unicode` package has only simple folding. Design 15, commit 19. |
| Canonical style by consensus | The style follows modern best practice across the major formatters and style guides, not personal preference. See Open decisions. |

## Open decisions

1. **Canonical style.** Stage 6 needs this, not before. The style follows
   modern best-practice consensus, not personal preference. At stage 6:
   1. Survey the defaults of Prettier, dprint (which `deno fmt` uses),
      mdformat, markdownlint and the Google Markdown style guide for each
      construct below.
   2. Where a clear majority exists, adopt it and record the evidence in
      Decisions.
   3. Where no consensus exists, ask the user one clear question with the
      evidence.

   Constructs: bullet marker, ordered list numbering, emphasis and strong
   delimiters, code fence character and length, thematic break, hard line
   break, table alignment, heading style, list indentation, escaping.

   The current formatter writes ATX headings, one blank line between blocks
   and one final newline, and keeps prose line breaks. The survey confirms or
   replaces these. Design appendix B lists the printer traps the style must
   respect.

## Plan

Each stage has gates. A stage is done only when its gates pass. Mark the
checkbox in the same commit that passes the gates. Design 15 sketches the
commits for stages 1 to 4.

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

### Stage 5: differential fuzzing

- [ ] `internal/markdown/differential_test.go` fuzzes our parser against
  goldmark and compares test HTML. goldmark is a test-only requirement of the
  root module until stage 7. No release happens before stage 7.
- [ ] One predicate per row of Known goldmark deviations, so the fuzzer skips
  known deviations.
- [ ] Triage every disagreement against the spec text. Save each one as a
  permanent case, marked "fixed in markfmt" or "goldmark deviates, spec
  section X".

Gate: the agreed fuzz budget (proposal: 24 CPU-hours) finds no disagreement
that has not been triaged.

### Stage 6: printers on the new tree

- [ ] Settle the canonical style. See Open decisions.
- [ ] A printer for every node kind, meeting design 12: facts through
  accessors, kept syntax and `Kept`, size decisions on canonical measures, and
  the output limit.
- [ ] Dialect predicates and spans in `Equal` (design 10.4).
- [ ] `FuzzFormat`: no check mismatch on any input, and equal test HTML for
  input and output (design 10.5).
- [ ] Move `internal/format` to the new parser and delete the goldmark-based
  code. Keep `testdata/cases` and make every case pass.

Gates: `testdata/cases` pass. Fuzzing shows idempotence and no check mismatch.
Math and alert fixtures pass as printer cases. `go list -deps ./cmd/markfmt`
lists no goldmark package.

### Stage 7: remove goldmark

Remove the differential test when all of these are true:

- [ ] All corpora at 100%: every example passes, or is listed in a
  `grammar-differs.txt` and its named case passes. Every `failing.txt` is
  empty.
- [ ] The stage 5 fuzz budget has run with every disagreement triaged.
- [ ] Every disagreement is a permanent case in `testdata`.

Gate: `differential_test.go` is deleted and `go mod tidy` has run. No goldmark
import anywhere. The root `go.mod` has no requirements. goldmark's copied test
files can stay as data, with their MIT notice.

### Stage 8: product

- [ ] CLI directory walking that skips `testdata`, hidden directories and
  vendored code. Concurrent work is limited by input bytes.
- [ ] Release setup: goreleaser, version stamping, `release.yml`, Homebrew
  tap. Follow hamnir's conventions.
- [ ] markfmt.com.

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

Use this list to triage stage 5 disagreements. Each row gets a predicate in
the differential test.

| Input | goldmark behaviour | Expected |
| --- | --- | --- |
| Table directly after a paragraph | The table gets the paragraph's position. | Position of the header row. |
| `0\n0\n    -\n0\n-` | Block positions out of order. | Positions in source order. |
| Paragraph with trailing spaces, then a table | HTML keeps the space: `<p>0 </p>`. | No trailing space. |
| HTML block at end of input without a final newline | HTML differs from the same input with a final newline. | The same HTML. |
| Lone CR | Not a line ending. | A line ending (CommonMark). |
| `*\r\n` | A paragraph. | An empty list item. |
| `[0]:\n0\n''0` | The destination line also appears as paragraph text. | Paragraph `''0` only (compare example 210). |

## How to resume

1. Read this document, then `docs/design/parser.md`.
2. Run `git log --oneline` and `git status`.
3. Run `task ci`. It must pass before new work starts.
4. Find the first unchecked item in the plan, or an open decision that blocks
   it.
5. Update this document in the same commit as the work.
