# markfmt roadmap

Read this document first when you resume work on markfmt. It records the
product rules, the current state, the decisions and the reasons for them, and
the plan with its gates. Update it in the same commit as the work it tracks.

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

Last updated: 2026-09-12. Nothing after `ff7de5b` is pushed.

| Commit | Content |
| --- | --- |
| `ff7de5b` | README |
| `fac76fe` | Formatter skeleton on goldmark v2.0.2, CLI, golangci-lint, Taskfile, CI, Dependabot |
| `bc99d39` | Spec corpus test, `FuzzSource`, fixes for 10 corpus and fuzz findings |
| `3898ac7` | LF line endings |

Layout:

- `cmd/markfmt`: the CLI. `markfmt [-check] [path ...]`. With no path or `-`,
  it reads stdin and writes stdout. It writes files atomically.
- `markfmt.go`: the public API, `Format(w io.Writer, r io.Reader) error`.
- `internal/format`: the goldmark-based formatter. It formats headings (ATX)
  and paragraphs, copies other blocks from source, passes front matter
  through, and compares goldmark HTML of input and output.
- `internal/format/testdata/cases`: 26 `NAME.in.md` and `NAME.out.md` pairs.
- `internal/format/testdata/spec`: CommonMark 0.31.2 examples (goldmark's
  `spec.json`) and goldmark's extra and GFM case files.
- `internal/format/testdata/fuzz/FuzzSource`: 9 inputs that the fuzzer found.
- `tools/go.mod`: golangci-lint v2.13.2, kept out of the root `go.mod`.

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
| Runtime safety check compares our own trees | The parser parses input and output, and the trees must be equal with spans ignored. No renderer quirks, no circular check. |
| Remove goldmark completely | It is a temporary test oracle only. See stage 7 for the exit criteria. |
| Test-only HTML renderer | The specs give expected results only as HTML. The renderer lives in `_test.go` files, never in the binary or the API. |
| Grammar scope: CommonMark, GFM, front matter, and GitHub syntax (footnotes, math, alerts) | Most users expect what GitHub renders. Math in the grammar stops the printer from changing math source. |
| Design document at `docs/design/parser.md` | It is reviewed and versioned with the code. |
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
   replaces these.

## Plan

Each stage has gates. A stage is done only when its gates pass. Mark the
checkbox in the same commit that passes the gates.

### Stage 0: design

- [ ] Review the goldmark v2 design: AST, source-backed and owned text
  values, positions, parser and renderer separation, block parsers by trigger
  character, inline parsers, AST transformers, extensions. Record what we take,
  what we drop, and why. Read for design only; do not copy code.
- [ ] Write the parser design in `docs/design/parser.md`, from first
  principles for a formatter. Current views for the design to test:
  - A lossless concrete syntax tree: tokens for markers, delimiters and
    whitespace, not only content nodes.
  - A closed set of node kinds: CommonMark, GFM, GitHub syntax and front
    matter. No plugin system. Exhaustive switches.
  - Two phases, as in the CommonMark appendix: blocks line by line, then
    inlines after all link reference definitions are known. This fits a later
    two-pass streaming parser.
  - Parsing cannot fail: every input is valid Markdown.
- [x] Settle the grammar scope, the test-only HTML renderer and the design
  document location. See Decisions.

Gate: the user approves the design document.

### Stage 1: harness

- [ ] A reader for `spec.txt` example blocks.
- [ ] Pin the corpora in `testdata` with a notice for each. See Corpora.
- [ ] A conformance runner with a checked-in expected-failure list. The test
  fails if an unlisted example fails, and also if a listed example passes. The
  list can only get shorter.
- [ ] The test-only HTML renderer and HTML normalization, as in cmark's
  `normalize.py`.
- [ ] Fuzz targets: lossless round trip, no panic, time limit per input.

Gate: `task ci` passes with every example on the expected-failure list.

### Stage 2: block structure

- [ ] Containers: block quotes, list items and lists, with tabs.
- [ ] Leaf blocks: thematic breaks, ATX and setext headings, indented and
  fenced code, HTML blocks (all 7 kinds), link reference definitions,
  paragraphs, blank lines.
- [ ] Front matter.

Gates:

- The lossless fuzz test holds: printing the tree unchanged gives the input
  byte for byte.
- Every block-level spec section conforms, with inline content as raw text.
- cmark's pathological block inputs run in linear time.

### Stage 3: inlines

- [ ] Backslash escapes, entity and numeric character references (table
  generated from WHATWG `entities.json`), code spans.
- [ ] Emphasis and strong emphasis with the delimiter run algorithm.
- [ ] Links, images, reference links, autolinks, raw HTML.
- [ ] Hard and soft line breaks.

Gates: CommonMark 0.31.2 conformance at 100%, cmark and commonmark.js
regression corpora at 100%, pathological inline inputs in linear time.

### Stage 4: GFM and GitHub syntax

The scope is what GitHub renders, because most users expect it.

- [ ] GFM extensions: tables, strikethrough, task list items, extended
  autolinks, disallowed raw HTML.
- [ ] Footnote references and definitions.
- [ ] Math: inline `$…$` and `` $`…`$ ``, and block `$$…$$`. A ```` ```math ````
  fence is an ordinary code block.
- [ ] Alerts: a block quote whose first line is `[!NOTE]`, `[!TIP]`,
  `[!IMPORTANT]`, `[!WARNING]` or `[!CAUTION]`.
- [ ] Plain text that GitHub gives meaning to needs no grammar, but escaping
  must never change it: emoji shortcodes (`:+1:`), mentions (`@user`) and
  issue references (`#1`).

Study GitHub's math behaviour with the Markdown REST API
(`gh api markdown -f mode=gfm -f text=...`) before writing the math rules.
Observations on 2026-09-12:

| Input | GitHub HTML |
| --- | --- |
| `$a*b*c$` | `$a<em>b</em>c$`. Emphasis wins, and there is no math element. |
| `` $`a*b`$ `` | A `math-renderer` element with `$a*b$`. The content is raw. |
| `$$`, `a*b*c`, `$$` on three lines | A display `math-renderer` element with `$$ a_b_c $$`. Emphasis applies inside, and GitHub writes it back as `_`. |

GitHub math is not a simple raw span. The grammar must match GitHub's
behaviour, and the printer must never change what MathJax receives.

Gates:

- The GFM extension examples and cmark-gfm `extensions.txt` at 100%. Take
  only extension examples from the GFM spec: it is pinned at 0.29, and its
  copies of core examples are older than CommonMark 0.31.2.
- Footnotes, math and alerts have no public spec. Their cases come from the
  GitHub docs examples and from captured GitHub API output, saved as
  fixtures, at 100%.

### Stage 5: differential fuzzing

- [ ] A separate test-only module that fuzzes our parser against goldmark and
  compares test HTML. The root `go.mod` does not change.
- [ ] Triage every disagreement against the spec text. Save each one as a
  permanent case, marked "fixed in markfmt" or "goldmark deviates, spec
  section X".

Gate: the agreed fuzz budget (proposal: 24 CPU-hours) finds no disagreement
that has not been triaged.

### Stage 6: printers on the new tree

- [ ] Settle the canonical style. See Open decisions.
- [ ] A printer for every node kind. Raw content is written from its exact
  span. No copying by guessed positions.
- [ ] The runtime check: parse input and output, compare the trees with
  spans ignored.
- [ ] Move `internal/format` to the new parser and delete the goldmark-based
  code. Keep `testdata/cases` and make every case pass.

Gates: `testdata/cases` pass, and fuzzing shows idempotence and equal trees.
goldmark is no longer in the binary.

### Stage 7: remove goldmark

Remove the stage 5 module when all of these are true:

- [ ] All corpora at 100% with an empty expected-failure list.
- [ ] The stage 5 fuzz budget has run with every disagreement triaged.
- [ ] Every disagreement is a permanent case in `testdata`.

Gate: no goldmark import anywhere. The root `go.mod` has no requirements.
goldmark's copied test files can stay as data, with their MIT notice.

### Stage 8: product

- [ ] CLI directory walking that skips `testdata`, hidden directories and
  vendored code.
- [ ] Release setup: goreleaser, version stamping, `release.yml`, Homebrew
  tap. Follow hamnir's conventions.
- [ ] markfmt.com.

## Testing strategy

| Property | Test |
| --- | --- |
| Lossless tree | Fuzz: printing the tree unchanged gives the input byte for byte. Needs no oracle. |
| No panic, linear time | Fuzz with a time limit, plus cmark pathological inputs. |
| Spec conformance | Corpus runner with the expected-failure list. |
| Agreement with a second parser | Differential fuzzing against goldmark, stages 5 to 7 only. |
| GitHub syntax | Fixtures captured from the GitHub Markdown API. Compare document structure, not GitHub's extra attributes. |
| Printer is idempotent | Fuzz and `testdata/cases`. |
| Printer keeps meaning | Fuzz and `testdata/cases`: equal trees for input and output. |

## Corpora

| Corpus | Source | Version | License | Use |
| --- | --- | --- | --- | --- |
| CommonMark spec examples | `commonmark/commonmark-spec` `spec.txt` | 0.31.2 | CC BY-SA 4.0 | Core conformance |
| CommonMark HTML normalization | `commonmark/commonmark-spec` `test/normalize.py` | 0.31.2 | BSD-2-Clause | Design reference for test HTML comparison |
| GFM spec extension examples | `github/cmark-gfm` `test/spec.txt` | 0.29 | CC BY-SA 4.0 | GFM conformance, extension examples only |
| cmark-gfm extension and regression tests | `github/cmark-gfm` `test/extensions.txt`, `test/regression.txt` | pin at import | BSD-2-Clause | Conformance and regressions |
| cmark pathological inputs | `github/cmark-gfm` `test/pathological_tests.py` | pin at import | BSD-2-Clause | Linear-time tests |
| commonmark.js regressions | `commonmark/commonmark.js` `test/regression.txt` | pin at import | BSD-2-Clause | Regressions |
| goldmark cases | `yuin/goldmark` `_test/extra.txt`, `extension/_test/*.txt` | v2.0.2 | MIT | Extra edge cases, already in `testdata/spec` |
| markdown-it fixtures | `markdown-it/markdown-it` `test/fixtures` | optional | MIT | Extra cases |
| HTML entities | WHATWG `entities.json` | pin at import | CC BY 4.0 | Generate the entity table |
| GitHub docs Markdown examples | `github/docs` `content/get-started/writing-on-github` | pin at import | CC BY 4.0 | Footnote, math and alert cases |
| GitHub Markdown API output | `POST /markdown` with `mode=gfm` | capture date | GitHub API terms | Expected results for GitHub syntax, captured once into fixtures |

## Licensing rules

This is our reading of the licenses, not legal advice.

- Read goldmark for design ideas only. Write our own code. If we ever port
  real code, keep goldmark's MIT copyright notice with it.
- Implement the algorithm that the CommonMark spec describes. Ideas are not
  covered by copyright; the spec text is CC BY-SA 4.0.
- Give every copied corpus a README or NOTICE with its source, version and
  license, as `internal/format/testdata/spec/README.md` does.
- Generate tables from primary sources: WHATWG `entities.json`, and Go's
  `unicode` package for punctuation.
- Send only test inputs written for the purpose to the GitHub Markdown API,
  never user documents.

## Known goldmark deviations

Use this list to triage stage 5 disagreements.

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

1. Read this document.
2. Run `git log --oneline` and `git status`.
3. Run `task ci`. It must pass before new work starts.
4. Find the first unchecked item in the plan, or an open decision that blocks
   it.
5. Update this document in the same commit as the work.
