# Parser design

Status: approved 2026-09-12, after five review rounds. Stage 0 of
`docs/roadmap.md`. Section 16's roadmap changes are applied.

This document designs the markfmt parser: a lossless concrete syntax tree for
CommonMark 0.31.2, GFM, front matter and GitHub footnotes, built for a
formatter. It records each decision and the reason for it. Section 16 lists
the changes this design makes to the roadmap. Section 17 lists the research
and reviews behind it. Appendix C records what the design takes from goldmark
and what it drops.

## 1. Goals and rules

Goals, in priority order:

1. **Correct.** The tree gives each document the meaning that CommonMark
   0.31.2 and GitHub give it. Conformance is measured, not assumed.
2. **Lossless by construction.** Every input byte belongs to exactly one leaf,
   in order. Printing the unchanged tree gives the input back byte for byte.
   One linear check proves this for any tree.
3. **Built for a printer and a safety check.** The tree keeps every fact a
   printer needs, and says exactly which facts are meaning.
4. **Never fails, never slow.** Every input within the size limit parses. Time
   and memory are linear in the input.
5. **Zero dependencies.** Standard library only.

Rules for all code in this design:

- **R-lines.** The work for one line is O(1 + bytes consumed + blocks closed).
  It is never O(nesting depth) (section 5.1).
- **R-walk.** Every walk over a tree uses a cursor or an explicit stack. No
  walk recurses. A Go stack overflow is fatal, and `>` × 10⁶ or nested
  emphasis would cause one.
- **R-fact.** Each fact has one byte-level function, in the file of its
  construct. The parser and the accessor both call it. Code outside the parser
  reads facts only through accessors. An accessor reads only leaves in the
  node's own subtree, or at a fixed offset from it, so a walk that calls every
  accessor once per node stays linear. A value accessor costs O(value size),
  and a caller reads it once per node.
- **R-build.** Only the builder writes nodes. Its checks are always on and
  panic. `markfmt.Format` recovers a panic from parsing, printing and `Equal`,
  and returns it as an error with the stack (product rule 6). `Parse` does not
  recover, so tests and fuzzing see the panic.

Non-goals: HTML output (a renderer exists only in `_test.go` files), plugins,
editing the tree, error recovery, streaming (section 7.4).

## 2. Grammar and dialect

| Layer | Definition | Authority |
| --- | --- | --- |
| Core | CommonMark 0.31.2 | `spec.txt` examples; cmark, then commonmark.js, where the spec has no example |
| GFM | Tables, task list items, strikethrough, extended autolinks | GitHub production behaviour: GFM 0.29 extension examples, cmark-gfm tests, GitHub API fixtures |
| GitHub | Footnotes | GitHub API fixtures and GitHub docs examples |
| Front matter | YAML or TOML block at the start of the input | Section 8.1 |

GitHub math and alerts are rendering filters, not grammar (section 9). GFM's
disallowed raw HTML filter changes only rendering and is not grammar.

### 2.1 Where GitHub and CommonMark 0.31.2 differ

GitHub runs cmark-gfm, which lags CommonMark 0.31.2 in some core rules.

Decision:

- Core constructs follow CommonMark 0.31.2 and its reference implementations.
  GFM and GitHub constructs follow GitHub production behaviour.
- Every known divergence is a row in `internal/markdown/testdata/dialect.md`:
  the rule, an input, both results, and the tests. The row lands in the commit
  that implements the rule markfmt follows, with its markfmt test. Its GitHub
  fixture lands at stage 4.
- At stage 6, each row gets a **dialect predicate**: a function over the tree
  that finds the span GitHub parses or renders differently. The span is the
  smallest node whose kind or extent differs between the two rules: for
  `<search>` after a paragraph line, the paragraph and the HTML block; for
  flanking next to a symbol, the smallest block with inline content
  (paragraph, heading or table cell).
- `Equal` compares dialect spans (section 10.4), and the printer keeps their
  bytes (section 12).

Known divergences at draft 3:

| Rule | CommonMark 0.31.2 | GitHub |
| --- | --- | --- |
| HTML block kind 6 tag list | has `search`, no `source` | has `source`, no `search` |
| HTML block kind 4 start | `<!` + any ASCII letter | `<!` + uppercase letter |
| HTML comments | `<!-->` and `<!--->` are comments | older grammar |
| HTML block kind 7 on a lazy candidate line (`> a` then `<del>`) | lazy paragraph text | block quote, then HTML block |
| Unicode punctuation for flanking | P and S; U+FFFD is punctuation | P only; U+FFFD is not punctuation |
| `[foo]: /url` then `---` | cmark: definition, paragraph `---`; commonmark.js: definition, empty paragraph, thematic break | paragraph `---` |
| Footnote reference whose `^` is an escape or an entity (`[\^1]`, `[&#94;1]`) | reference (decoded text starts with `^`, as cmark-gfm tests) | reference, rendered as garbled text |
| Footnote reference label across a line ending | reference with a line ending in its label | rendered as garbled text |
| Paragraph split off above a table | `\|` is an escaped pipe | backslash removed |
| List items that start on one line | every item opens | at most 99 blocks start on a line |
| Code span after an unmatched backtick run (`a `` b `c` d `e``) | `c` and `e` are code spans (spec text, commonmark.js) | `e` stays text (cmark and cmark-gfm) |
| VT and FF in a link destination | end the destination (cmark, section 8.4) | part of the destination |
| VT and FF at the ends of an info string | trimmed (cmark, section 8.4) | kept |
| Definition title that other characters follow on its line | the definition has no title (spec text, commonmark.js) | the title stays (cmark and cmark-gfm) |
| Link label length | at most 999 characters (spec text, commonmark.js) | at most 1000 bytes (cmark and cmark-gfm) |
| VT and FF in a link label | whitespace (cmark, section 8.4) | label characters |

markfmt follows the CommonMark column, and cmark where cmark and commonmark.js
disagree, but not where cmark contradicts the spec text (the code span, definition
title and label length rows),
with two exceptions. The footnote rows
have no CommonMark rule, so markfmt follows cmark-gfm's tests. The paragraph
split off above a table follows GitHub: the table that splits it is GFM
grammar, so section 5.4 gives the paragraph the cell pipe rule.

Reason: the tree needs one grammar to be testable. The printer's job is to
keep meaning under the renderers people use, so it must not rewrite where they
disagree, and must not create a place where they disagree.

## 3. Representation

### 3.1 Node array

A flat, preorder array of fixed-size, pointer-free nodes. Leaves tile the
input. Interior nodes group leaves.

```go
type Node struct {
	kind  Kind
	flags uint8  // per-kind facts (section 3.5)
	virt  uint8  // columns left for the leaf of its first byte, a split tab (section 4.3)
	_     uint8
	start uint32 // byte offset
	end   uint32 // byte offset, exclusive
	link  uint32 // interior: index one past the last descendant; prefix leaf: owning container; other leaves: 0
}

type Tree struct {
	src   []byte
	nodes []Node // preorder; nodes[0] is the document
}
```

- 16 bytes per node, no pointers, so the Go collector does not scan the array.
- Real documents need 0.05 to 0.15 nodes per input byte: 0.8 to 2.4 bytes of
  node array per input byte. The worst sustained case is 2 nodes per byte
  (`>` × n, `||||` rows).
- Fields are unexported. Consumers use accessors (section 7.3).
- The owner in a prefix leaf's `link` has one reader, `Verify`. It catches
  prefix attribution bugs, which no other test can see.

Alternatives rejected:

| Option | Why not |
| --- | --- |
| Typed pointer tree (goldmark, dprint-plugin-markdown) | Lossless only by test: prefix bytes are gaps between node spans. One heap object per node, all scanned by the collector. |
| Content array plus a line table | Lossless by construction, but the check and the unchanged print must merge two sequences, and two structures must stay in sync. |
| Event stream (micromark) | No parent and child navigation. micromark writes prefix events before containers are decided and moves events afterwards. |
| One block phase, then a rebuild that inserts inline nodes | A second array write and a remap of every `link`. Two passes (section 7.1) append in order and need no remap. |

### 3.2 Invariants

`Tree.Verify` checks all of them in one iterative pass. Tests and fuzzing call
it on every tree.

1. Leaves in array order have `start == previous end`. The first leaf starts
   at 0. The last leaf ends at `len(src)`. A tree with no leaves has
   `len(src) == 0`.
2. A leaf has `start < end`. A leaf with `virt` above 0 starts with a tab, and
   its `virt` is at most 3.
3. An interior node's `start` is its first descendant leaf's `start`, and its
   `end` is its last descendant leaf's `end`. An interior node with no leaves
   has `start == end`, equal to the `start` of the next leaf, or `len(src)`.
4. An interior node's `link` is greater than its own index and not greater
   than its parent's `link`. `nodes[0]` is the Document, and its `link` is
   `len(nodes)`, so every other node is inside it.
5. A prefix leaf's `link` is the index of an ancestor container. Every other
   leaf has `link == 0`.
6. Each kind is a leaf kind or an interior kind, and has one comparison class
   (section 3.4).
7. `flags` has no bit outside its kind's mask, and every value is in range
   (HTML block kind 1 to 7, alignment 0 to 3). Each flag that also has a byte
   form agrees with its bytes: TableCell alignment with the delimiter row cell,
   ListItem task and checked with the TaskBox leaf, HTMLBlock kind with its
   start condition.
8. The node count is at most 3 × `len(src)` + 3. Charge table: each leaf is
   charged to its own bytes. BlockQuote is charged to its `>`, List and
   ListItem to the marker bytes, FootnoteDefinition to its `[`, Table and
   TableRow to the row's line ending (a last row with no line ending: its first
   byte), TableCell to its pipe or first byte, SoftBreak and HardBreak to the
   line ending, an inline span to its opener, every other interior kind to its
   first byte. No byte carries more than two interior charges. Table and
   TableRow are appended before their bytes, so the builder checks
   `len(nodes) ≤ 3 × end + 3` at each leaf append.

Printing the unchanged tree is one loop over leaves: write `src[start:end]`.

### 3.3 Kinds

`Kind` is an integer enum. Appendix A is the only list of kinds. Other
sections use its names.

Adding a kind is a design change: appendix A and the class function change in
the same commit. Code adds a kind in the commit that first emits it.

Switches that must cover every kind carry `//exhaustive:enforce`: the class
function, the key function (section 10.2), the test renderer and the printer.
The `exhaustive` linter runs with `explicit-exhaustive-switch: true` and
`default-signifies-exhaustive: false`. Other switches on `Kind` are partial and
need no comment.

### 3.4 Comparison classes

Every kind has exactly one class. The class decides what the runtime check
compares (section 10).

| Class | Nodes | Compared |
| --- | --- | --- |
| Structure | Interior kinds | Kind, key (section 10.2), and where the subtree ends |
| Content | Leaf kinds that carry meaning | Decoded value, joined across a run of the same group (section 10.3) |
| Syntax | Leaf kinds the printer may rewrite | Not compared |

Line endings are the trap. A line ending between paragraph lines is syntax:
the SoftBreak node around it carries the meaning. A line ending inside code,
an HTML block, front matter, a title, a label, raw inline HTML or a code span
is content: its kind is `VerbatimLineEnding`, value `\n`. The same holds for
spaces: spaces beyond the indentation on a blank line in a code block are
`CodeText`, not `BlankLine`.

### 3.5 Where facts live

Leaves hold bytes, never decoded values. Each fact has one byte-level
function (R-fact). `flags` holds a fact only when an accessor cannot read it
from the node's own subtree: a parent, a sibling position, or the rest of the
list. The parser writes these flags when it emits the node,
from the same byte-level function, and `Verify` checks them against the bytes
(invariant 7).

| Fact | Byte-level function in | Accessor reads |
| --- | --- | --- |
| Heading level | `heading.go` | ATXMarker or SetextUnderline leaf |
| List ordered, start number | `list.go` | ListMarker leaf of the first item (`003.` is 3) |
| Bullet character, ordered delimiter | `list.go` | ListMarker leaf |
| List loose | `list.go` (section 5.6) | `flags` |
| Task item, checked | `task.go` | `flags` of ListItem |
| HTML block kind | `htmlblock.go` | `flags` |
| Table column count | `table.go` | TableDelimiter leaves, O(columns): read once at Table enter and kept for its rows |
| Cell alignment, header | `table.go` | `flags` of TableCell |
| Cell pipe escape group | `inline.go` | `flags` of CellPipeEscape: the kind of the content leaves around it |
| Link and image form | `link.go` | `flags` |
| Reference label | `chars.go` | label leaves, normalized |
| Footnote reference resolved | `link.go` | `flags` |
| Autolink form, URL or email | `autolink.go` | its first leaf and its text leaves |
| Code content | `code.go` | CodeText and VerbatimLineEnding leaves, with `virt` spaces |
| Destination, title | `link.go`, `linkref.go` | their leaves, decoded |
| Footnote definition label | `footnote.go` | FootnoteLabel leaves, raw |
| Front matter kind | `frontmatter.go` | FrontMatterFence leaf |

## 4. Lines

### 4.1 Line iterator

The line iterator yields one record per line. It keeps no list of lines.

```go
type line struct {
	start, end uint32 // content bytes, without the line ending
	eol        uint32 // end of the line ending: end, end+1 (LF or CR) or end+2 (CRLF)
}
```

- LF, CR and CRLF are line endings. `\r\r\n` is two line endings.
- The last line has no line ending when the input does not end with one. The
  block phase treats end of input as a line ending.
- No other code looks for `\n` or `\r`.
- A UTF-8 byte order mark at offset 0 is a BOM syntax leaf, and takes zero
  columns: the first line's column 0 is the byte after it. cmark and GitHub
  strip a BOM; commonmark.js does not; cmark wins.

### 4.2 Blank lines

A blank line has only spaces and tabs after the prefixes it matched.

- A structural blank line is one `BlankLine` leaf: its spaces, tabs and line
  ending. At end of input with no spaces and no line ending, there is no line.
- It follows the line's prefix leaves, in the deepest block that is still open
  after the line's close decisions. That can be a List, when the item before it
  did not continue (CM 280, CM 315).
- Inside fenced code, indented code and HTML block kinds 1 to 5, a blank line is
  content: `CodeText` or `HTMLText` for spaces beyond the block's indentation
  (for HTML blocks: all of them), and `VerbatimLineEnding`. A blank line ends
  HTML block kinds 6 and 7 and is structural.
- Trailing blank lines of indented code are pending (section 5.3). They become
  code content if more code follows, and structural blank lines otherwise.

The parser never moves a leaf after appending it.

### 4.3 Columns, tabs and `virt`

- A tab advances to the next multiple of 4 columns.
- Container continuation is measured from the offset after the prefixes this
  line has matched so far. Each open container stores `markerOffset` and
  `padding` relative to that offset, as cmark does. A ListItem continues when
  the indentation is at least `markerOffset + padding`.
- The column is tracked while the line is consumed. It is never recomputed from
  the line start.
- A tab byte belongs to the one leaf that consumes its last column. A
  structure that consumes only some of a tab's columns splits the tab and emits
  no leaf for them. A later structure that consumes more of the tab lowers the
  columns left.
- `virt` on a leaf whose first byte is a split tab is the number of the tab's
  columns left for the leaf (1 to 3). On every other leaf it is 0.
- So the number of prefix leaves on a line does not give the number of matched
  containers.
- A value that starts with a split tab writes `virt` spaces for that tab:
  `CodeText` and `HTMLText` on each line. Every split tab is in a leaf, so a
  content line that is only a split tab after its prefixes keeps its spaces,
  also at the end of the input. The printer writes `virt` as spaces when it
  moves such content.

Reason: cmark and commonmark.js add the rest of a split tab as spaces to every
content line. A rule that gives the tab to the leaf that consumes its first
column leaves no leaf for those spaces when the rest of the line is empty.

Examples: CM 5, 6, 7; `>  ```` then `>→→x` (content ` →x`); `   > - a` then
`   >→    code` (content ` code`); `>→>→→foo` (content `  foo`); `> ```` then
`>→` (content `  `).

## 5. Block phase

### 5.1 Per-line algorithm

The block phase follows the CommonMark appendix. For each line:

1. **Continue.** Walk the open containers from the document. Each tests its
   continuation on the rest of the line:
   - BlockQuote: up to 3 columns of indentation, `>`, optional space.
   - ListItem: if the indentation is at least `markerOffset + padding`,
     consume that many columns. Else, if the rest is blank and the item has a
     child, consume the rest of the spaces and tabs and match. Else it does not
     match (CM 280).
   - FootnoteDefinition: 4 columns, or an empty line: no byte before its line
     ending. cmark-gfm tests the whole line, so a line of fewer than 4 columns
     of spaces, or a `>` line of a block quote around the definition, ends it
     (GitHub API, 2026-09-13).

   Blank-line fast path: when the rest of a line is empty (no spaces or tabs
   remain), the open stack gives the index of the first container that such a
   line may fail to continue: a BlockQuote, a ListItem with no child, or,
   when the line is not empty, a FootnoteDefinition. The line matches every
   container below that index in O(1). From that index,
   containers are tested one by one. A blank line with spaces or tabs walks
   the containers that consume them, which is work on consumed bytes.
2. **Start.** On the rest of the line, try block starts. At the first start,
   close the unmatched containers, then open the new block. Repeat while a
   container starts. Rules:
   - Indented code and HTML block kind 7 never start while the deepest open
     block is a paragraph, matched or not (CM 238; kind 7 is a `dialect.md`
     row).
   - Any other start that "cannot interrupt a paragraph" is blocked only when
     the open paragraph is a child of the last matched container. So `> a` then
     `2) x` starts a list.
   - Per-line memos keep starts that read the rest of the line linear. The
     thematic break memo holds P: a scan that fails at a byte that is not a
     marker, space or tab sets P to that byte's offset; a scan that fails at a
     different marker character sets P to that offset minus 1; a start at or
     before P needs no scan. A second memo holds the first non-space byte after
     an offset.
   - Order for a line after an open paragraph: setext underline, thematic
     break, list item, other starts, table delimiter row.
3. **Lazy or close.** If no block started, some containers did not match, the
   deepest open block is a paragraph, and the rest is not blank, the line is a
   lazy continuation. Otherwise close the unmatched containers.
4. **Add.** Give the rest of the line to the deepest open block.

Containers: BlockQuote, ListItem, FootnoteDefinition, and List around list
items. Leaf blocks: Paragraph, Heading, ThematicBreak, CodeBlock, HTMLBlock,
LinkReferenceDefinition, Table, FrontMatter.

No construct is found after the fact. There are no paragraph transformers and
no tree transformers.

### 5.2 Per-line emission order

For each line, the builder appends in this order:

1. If the line is not lazy, close unmatched blocks. Closing flushes pending
   blocks, whose bytes come before this line.
2. The prefix leaves of the matched containers: into the pending line, if the
   line continues a pending leaf block; otherwise directly after the last
   appended node.
3. For each new container: `open`, then its marker leaf (QuoteMarker,
   ListMarker, or the footnote label leaves).
4. The line's content.

So the prefix leaves of a leaf block's first line come before the leaf block's
node, and the prefix leaves of its later lines are inside it. No consumer
depends on the depth of a prefix leaf.

### 5.3 Pending blocks

A leaf block whose kind or extent can still change keeps its lines pending:

- Paragraph: all lines until close.
- CodeBlock (indented): content lines are appended directly. Only the run of
  trailing blank lines is pending.

```go
type pendingLine struct {
	line        line
	prefixFirst uint32 // index into the block's prefix arena
	prefixN     uint16
	virt        uint8
	lazy        bool
}
```

24 bytes, pointer-free. Each pending block has one prefix arena. When the
block closes or its kind is final, the builder appends its nodes in source
order, with each line's prefix leaves in place.

Unclosed fenced code, HTML blocks and table rows are not pending: they append
line by line.

### 5.4 Paragraph decisions

| Construct | Decided at | Rule |
| --- | --- | --- |
| Link reference definitions | Paragraph close; setext underline | Parse definitions from the first pending line. A definition found is committed. The paragraph keeps exactly the lines after the last definition token. |
| Setext heading | Underline line, with every container matched and the paragraph as the tip | Parse definitions first (CM 215, 216). If content lines remain, they and the underline become the heading. If none remain, the underline line is the first line of the open paragraph, and no other block starts on it, as in cmark (`dialect.md`): `---`, `-` and `===` become paragraph text. |
| GFM table | First delimiter row candidate in the paragraph, with every container matched | Split the last pending line into cells, as raw text, with no definition parse. If the cell count matches the delimiter row, the earlier pending lines become a Paragraph with no definition parse and with the cell pipe rule of section 8.2 (`dialect.md`), and the table opens. If not, mark the paragraph "table tried" and add the line as paragraph text. A paragraph tries at most once. |
| Footnote definition | Its start line | It interrupts the paragraph like any block start. |

Link reference definition details:

- A failed title rewinds to the end of the destination. The definition holds
  only if nothing but spaces and tabs follows on that line (CM 209, 210).
- The title search needs no memo. The definition parse stops at the first
  definition that fails, so at most one title scan per paragraph reads past
  its own line. When a title fails and its definition holds, the next line
  starts with that title's quote, so no definition starts there. Pass 1 runs
  the same parse, so the bound holds for both passes.
- Label length is checked against the cap before any scan (section 6.8).

### 5.5 Containers

- **BlockQuote.** One QuoteMarker leaf per line: indentation, `>`, optional
  space. A tab that the optional space splits is not in it (section 4.3). A
  lazy line has none.
- **ListItem.** Spec rules 1 to 4: padding N with 1 ≤ N ≤ 4; N ≥ 5 means N = 1
  and indented code; a blank first line gives padding 1; an empty item cannot
  interrupt a paragraph (CM 285). One ListMarker leaf on the first line
  (indentation, marker, padding), and one ItemIndent leaf on a later line when
  bytes are consumed.
- **List.** A List closes when its parent gets a child that is not a ListItem
  with the same bullet character or ordered delimiter (CM 301, 302), or when
  its parent closes.
- **FootnoteDefinition.** Section 9.2.

### 5.6 List looseness

The rule follows cmark:

- Each open block has a `lastLineBlank` bit.
- A blank rest sets the bit on the block that receives the line, except: fenced
  code, BlockQuote, Heading, ThematicBreak, and an item's own empty marker line.
- A non-blank line added to a block clears that block's bit.
- When any container gets a new direct child, its bit is cleared. If it is a
  ListItem or a List and the bit was set, the List (the container itself, or
  the item's List) is marked loose first.
- When a block closes, it ORs its bit into its parent's bit, only when the
  parent is a List or a ListItem.

Every step is O(1) per line or per close. Tests: CM 306 to 326, GitHub
fixtures g11 to g13, and the red-team cases for nested blank lines, empty
items, HTML comments, indented code and footnote definitions in items.

## 6. Inline phase

### 6.1 When and where

Inline parsing runs when a block with inline content closes: Paragraph,
Heading, TableCell. Link reference definitions, code, HTML and front matter
have their own grammar. Inline parsing never changes block structure
(requirements section 8.9).

The input is the block's lines up to each line ending: content bytes, with the
prefix and Indent leaves of each continuation line emitted in place. Every
reader in this section, the link tail reader included, reads these lines, not
`src`. After a SoftBreak or HardBreak node come the next line's prefix leaves,
then its Indent leaf.

Only the inline scanner classifies spaces before a line ending: TrailingSpace,
HardBreakMarker, or content inside a code span or raw HTML.

### 6.2 Scan

A single left-to-right scan with a `[256]` trigger table writes leaves and
delimiter entries to a scratch buffer:

- Text is one leaf per maximal run of plain bytes. The emitter joins adjacent
  Text leaves.
- Escape leaves (2 bytes), EntityRef leaves.
- Code spans, autolinks and raw HTML match at their opening byte. The leftmost
  construct wins (CM 341 to 346).
- `*`, `_` and `~` runs push delimiter entries. `[` and `![` push bracket
  entries with a push sequence number.
- At `]`, look for a link or image immediately (section 6.3).
- GFM extended www and URL autolinks match at `w` and `:`, as cmark-gfm's
  `www_match` and `url_match` find them, and only when the bracket stack is
  empty: any `[` or `![` entry, active or not, blocks them. A www autolink
  needs a line start, a space, `*`, `_`, `~` or `(` before it. A URL
  autolink takes the letters before `:` back from the last Text piece as its
  scheme: `http`, `https` or `ftp`, in any case. The domain scan stops at a
  byte that is not a host character, `-`, `_` or `.`, reads the byte after a
  `\`, and never reads the last byte of the block's content; `_` in one of
  the last two segments rejects a domain with at most 10 dots. The link then
  runs to a space or `<`, and loses trailing punctuation, extra `)` and an
  entity-like `&letters;`. Its text is not decoded, so it is Text leaves.

### 6.3 Links, images, footnote references

At `]`, find the nearest bracket entry.

1. Inactive opener: `]` is text.
2. Try an inline link: read destination and title across line boundaries.
   Emit their leaves and continue the scan after `)`.
3. Try a full, then collapsed, then shortcut reference against the pass 1 link
   label list. A bracket followed by a label is never a shortcut (CM 569 to
   571). With an empty label list, skip every lookup.
   - Full reference: the label is the second bracket pair, scanned after `]`.
     The link text may contain brackets (CM 529).
   - Collapsed and shortcut reference: the label is the bracket text. A
     bracket text with an unescaped `[` or `]` is not a label (CM 546 to 548):
     if any bracket entry was pushed after this opener, reject in O(1) with no
     normalization.
4. Try a footnote reference: if the decoded bracket text starts with `^` and
   has at least one more character, the span becomes a FootnoteReference, at
   any length (cmark-gfm).
   - The scan pushes a `^` directly after `[` as its own piece, and `![` as a
     `!` piece and a `[` piece, which an image joins into one Bracket leaf. So
     the Caret and Bracket leaves are whole pieces, and no piece is inserted.
   - The delimiter stack is truncated to the opener's bottom. cmark-gfm
     processes the inner delimiters with the opener as the bottom and then
     frees their nodes, so none pairs with an outer delimiter. No
     `openers_bottom` value lives between two runs of process emphasis, so none
     needs a change. No bracket entry is above the opener, and
     `linkFormedAfter` is below its sequence number: a link inside would make
     the opener inactive.
   - The pieces between the brackets are rewritten in place as label leaves
     (FootnoteLabel, VerbatimLineEnding). Prefix and Indent leaves stay syntax,
     and in a table cell CellPipeEscape leaves stay CellPipeEscape, with label
     bytes per section 6.7. The rewrite passes each footnote reference that
     completed inside the brackets in one step, from its opener to its closer,
     so each piece is rewritten once (section 6.8).
   - Earlier openers stay active. For `![`, the `!` stays text.
   - "Resolved": the label has 1 to 1,000 label bytes, no `]` was handled after
     the opener (a definition label cannot contain `]`, so no normalization
     runs), and the normalized label is in the pass 1 footnote label list. An
     unresolved reference renders as its source text.
5. Otherwise `]` is text, and the opener entry is removed.

After a link (not an image, not a footnote reference), earlier `[` openers
become inactive through the sequence number `linkFormedAfter`, in O(1).
Emphasis inside link text is resolved with the bracket as the stack bottom.

### 6.4 Emphasis and strikethrough

- `*` and `_` follow "process emphasis" with `openers_bottom` indexed by
  character, closer length mod 3, and "closer can open".
- `~` follows cmark-gfm: only runs of 1 or 2 tildes are delimiters. A closer
  pairs with the nearest `~` opener at or above `openers_bottom` for its length
  mod 3 that passes the same rule-of-3 test as emphasis; openers that fail the
  test are skipped. Equal lengths make Strikethrough. Unequal lengths make the
  opener, the closer and every delimiter entry between them text. When a
  closer finds no opener, `openers_bottom` is set to the closer's position, so
  the closer can still open (`a~b~c` gives `a<del>b</del>c`).
- Both share the delimiter stack, so results nest properly.
- A run split across roles becomes several Delimiter leaves, one per role, and
  Text leaves for unused characters. A closer takes characters from the left of
  its run, an opener from the right.

### 6.5 Task list items

A ListItem is a task when its first child that is not a
LinkReferenceDefinition is a Paragraph whose first bytes are `[`, one of space,
tab, `x` or `X`, `]`, then a space or tab, then at least one non-whitespace
byte on the same line, and those brackets did not form a link (GitHub: a
defined `[x]` wins). The paragraph's inline phase emits the TaskBox leaf, `[`,
the character and `]`, then a Whitespace leaf for the spaces and tabs after it,
and sets the item's task and checked flags. A heading is never a task. The
test renderer writes the checkbox at the TaskBox leaf, so in a loose list it is
in the paragraph, as GitHub writes it.

### 6.6 Emit

The emitter walks the scratch buffer once and appends nodes and leaves in
preorder. The scratch buffer never inserts:

- A delimiter run split across roles is a side record keyed by scratch index.
- Spans are indexed by opener piece (Delimiter piece, Bracket, CodeFence).
  Each piece opens at most one span, so emission is O(1) per leaf.
- Spans never cross: links close at `]`, footnote references remove their inner
  entries, and emphasis nests.
- GFM email autolinks are found here, as cmark-gfm's `postprocess_text` finds
  them in a text node: in a maximal run of Text-group pieces (Text, Escape,
  EntityRef, and CellPipeEscape in text) within one span and outside Link and
  Autolink spans, on the decoded run. The emitter splits a Text leaf where a
  match starts or ends, and wraps the match in an Autolink node, which can
  contain Escape and EntityRef leaves. It decodes a run only when the run can
  hold `@`.

### 6.7 Characters and labels

- One decoder serves every character decision: WHATWG UTF-8 decoding, where
  each maximal invalid subsequence is U+FFFD, and NUL is U+FFFD.
- Whitespace for flanking: Unicode `Zs`, tab, line ending, form feed.
  Punctuation: Unicode `P` and `S`. U+FFFD is `So`, so punctuation
  (`dialect.md`).
- Label bytes: for a LinkLabel or FootnoteLabel, its leaves and
  VerbatimLineEnding; for a collapsed or shortcut reference, every leaf between
  the brackets except prefix and Indent leaves, syntax leaves included
  (`[*foo* bar]` has the label `*foo* bar`).
- In a table cell, a CellPipeEscape leaf gives label bytes equal to its source
  bytes minus the backslash of its `\|` pair: a 3-byte `\\|` gives `\|`, a
  2-byte `\|` gives `|`. So `[x\\|y]` has the label `x\|y` (GitHub).
- Label normalization: UTF-8 decoding only (escapes and entity references stay
  as written, so `[foo\!]` does not match `[foo!]`, CM 545), Unicode full case
  fold, trim and collapse runs of space, tab and line ending, and in a link
  label also VT and FF, to one space, as cmark does (section 8.4). A footnote
  label keeps VT and FF, as cmark-gfm does. Other Unicode whitespace, NBSP
  included, is kept.
- Label cap: 999 characters, for link reference definitions, links and images,
  checked in this order before any scan or normalization: the bracket sequence
  number (section 6.3), then a byte length of at most 999 (accepted without a
  count), then a byte length above 3,996 (rejected), then a character count.
- A footnote reference resolves only with at most 1,000 label bytes, with no
  character count. cmark-gfm's map lookup checks bytes. GitHub API,
  2026-09-13: a label of 1,000 ASCII bytes resolves; 1,001 ASCII bytes and 334
  × `€` (1,002 bytes) do not.

### 6.8 Linear time

| Input | Mechanism |
| --- | --- |
| Emphasis without partners, mismatched `*` and `_`, multiples of 3 | `openers_bottom` per character, length mod 3, can-open |
| `*a ` × n then `a~ ` × n | `openers_bottom` for `~` per length |
| `[` or `]` without partners | Bracket stack cleared at block end; O(1) empty check |
| `![[]()` repeated | `linkFormedAfter`; inactive openers removed when met |
| `[`×n `a` `]`×n, and `[`×499 `a` `]`×499 repeated | Bracket-in-label rejection by sequence number; label cap |
| `[^`×333 `a` `]`×333 repeated | Delimiter stack truncated to the opener's bottom; unresolved by `]` sequence number, with no normalization |
| `[^`×n `a` `]`×n, and `[^⏎`×n `a` `]`×n | The label rewrite passes each completed inner reference in one step (section 6.3) |
| Backtick runs of every length | Per-block index of backtick runs by length, with a cursor per length |
| Unclosed `<!--`, `<?`, `<![CDATA[`, `<!X` | Per-block memo of the first failed search offset per closer |
| `[a](<b` repeated | Angle destination stops at the next unescaped `<` (CM 494) |
| `[a](b` repeated | Parenthesis depth limit of 32 |
| `[ (](` repeated | Parenthesized title stops at an unescaped `(` |
| `[a]: b 'c` lines | Definition parse stops at the first failure (section 5.4); an inline title opener closes the scan of the title before it |
| `<a x="1"` lines | A failed tag reads only its own attributes: no paragraph line is blank, so tag whitespace holds at most one line ending |
| Extended autolinks, `_www.` × n | A domain with more than 10 dots is accepted despite underscores, as cmark-gfm does, so no candidate after it scans again; a scheme rewind stops at a non-letter |
| `***a*** ` × n, `a@b.cc ` × n | Side records; spans indexed by opener piece |
| Many definitions and references | Go map; label cap |
| `- `×n `a` on one line | Per-line memos (section 5.1) |
| `- `×d `a` then blank lines × m | Blank-line fast path (section 5.1) |
| `>`×n `a`, nested lists | O(1) work per consumed byte; iterative close |
| Tables with many rows | One header attempt per paragraph; no cells created for missing cells |
| `[^a]: `×n `b` on one line | O(1) work per consumed byte, as `>`×n |
| Header of C cells, R one-cell rows | `Equal` compares present cells only (section 10.3) |
| Nested strong emphasis output | R-walk |

## 7. Passes, API and limits

### 7.1 Two passes

1. **Pass 1** runs the block phase with the real builder into a node array that
   is reset whenever only the Document is open at a line boundary. It skips the
   inline phase. It keeps the link label list and the footnote label list, in
   document order.
2. **Pass 2** runs the block phase with the inline phase at each close, and
   builds the tree.

Pass 1 must run the full block phase: definitions depend on block context
(CM 211 to 218, footnote rules). Block decisions keep their state on the open
stack, never in the node array, so both passes decide the same way.

Skip: every definition contains `]:`. If `src` has no `]:`, both lists are
empty, and pass 1 is skipped (67% of bytes and 89% of files in a module-cache
sample).

Pass 2 checks that its definitions equal pass 1's lists, in order, on every
parse, in O(label bytes). A mismatch panics: it is a parser bug.

Pass 1 arrives in stage 3, with reference links.

### 7.2 Limits

`Format` holds, at peak: the input, the input tree, the pass 1 lists, pending
records and the inline scratch buffer of the largest block, the output, and the
output tree. Worst sustained density is 2 nodes per byte (32 bytes of nodes
per input byte); append growth keeps up to 1.8 × the largest array live; the
collector allows up to 2 × live heap.

| Limit | Value | Reason |
| --- | --- | --- |
| Input | 8 MiB | Worst case about 3 to 3.6 GiB peak; real documents about 120 to 140 MiB. |
| Output | 16 MiB, absolute | The printer writes into a writer that fails at the limit, so it never builds more. Appendix B bounds each expanding rule, so real files stay far below it. |

Both limits are constants, not options. `Format` returns an error above
either. The long test (section 11.1) measures peak memory in a child process
at the input limit: `Parse`, `Verify` and `Equal` of a tree with itself from
stage 2, and `Format` within the 4 GiB budget from stage 6. The stage 8 CLI limits concurrent work
by input bytes, not by file count.

### 7.3 API

```go
package markdown // internal/markdown

func Parse(src []byte) *Tree // precondition: len(src) within the input limit

func (t *Tree) Verify() error
func (t *Tree) Walk() Cursor            // preorder enter and exit events, no recursion
func (c *Cursor) Next() (Event, bool)
func (t *Tree) Kind(id NodeID) Kind
func (t *Tree) Raw(id NodeID) []byte
func (t *Tree) AppendValue(dst []byte, id NodeID) []byte
// fact accessors, one per row of section 3.5, each in its construct's file
func Equal(a, b *Tree) error            // section 10
```

`Event` is an enter or exit of a node ID. `Cursor` keeps an explicit stack of
subtree ends. It is a cursor, not an `iter.Seq`, because `Equal` walks two
trees in lockstep, and `iter.Pull` measured 43 times slower.

### 7.4 Streaming: not planned

Streaming is not planned until a user needs files over the input limit. The
documents that need it (one huge table, list or quoted email) are one
top-level block, so per-block streaming would not help them. If it is planned
later, it needs: a base offset for `Raw`, seekable input for pass 1, a
temporary output file for product rule 6, a comparison that lags one block (a
printed block can change how the block before it parses), and a memory bound
of c × the largest top-level block plus the label lists.

## 8. Constructs

Each construct lists its leaves and its key. Syntax is the default class;
content kinds are marked. Traps and example numbers are in the requirements
research (section 17).

### 8.1 Front matter

- Only at offset 0, after a BOM. Opener: a line that is exactly `---` or `+++`,
  with optional trailing spaces and tabs. Closer: a later line with the same
  delimiter and optional trailing spaces and tabs.
- An empty body is allowed. Without a closer, there is no front matter.
- CM 96 and CM 98 start with such a pair. They are listed in
  `grammar-differs.txt` (section 11.2) with the reason "front matter".
- Leaves: FrontMatterFence, LineEnding, FrontMatterText (content) and
  VerbatimLineEnding (content) per body line, FrontMatterFence, LineEnding.
- Key: YAML (`---`) or TOML (`+++`).

### 8.2 Leaf blocks

| Construct | Leaves | Key |
| --- | --- | --- |
| Thematic break | Indent, ThematicRun, LineEnding | none |
| ATX heading | Indent, ATXMarker, Whitespace, inlines, Whitespace, ATXClose, LineEnding | level |
| Setext heading | Indent, inlines per line, Indent, SetextUnderline, LineEnding | level |
| Indented code | CodeIndent, CodeText (content), VerbatimLineEnding (content) | code value |
| Fenced code | Indent, FenceMarker, Whitespace, InfoString (content), LineEnding, per line CodeIndent, CodeText, VerbatimLineEnding, then Indent, FenceMarker, LineEnding if closed | info string, code value |
| HTML block | HTMLText (content: every byte after the prefixes, indentation included), VerbatimLineEnding | HTML value |
| Link reference definition | Indent, Bracket, LinkLabel, Bracket, Colon, Whitespace, AngleBracket, Destination (content), Whitespace, TitleQuote, Title (content), TitleQuote, LineEnding | normalized label |
| Paragraph | Indent, inlines, LineEnding | none |
| Table | Indent, TablePipe, cells, TablePipe, LineEnding per row; the delimiter row as TableDelimiter and TablePipe leaves | column count |
| Table cell | inlines | alignment, header |

- A code or HTML value gets a final `\n` when the block's last content line
  exists and has no line ending, whether or not the line has content leaves (a
  line exists when it has any bytes). The closing fence line of a fenced code
  block is not a content line, so ```` ```⏎x⏎``` ```` at end of input has the
  value `x\n`. End of input is a line ending, so `    x` and `    x⏎` have
  equal values.
- Code blocks have no "fenced" or "closed" key: indented code and fenced code
  without an info string have equal meaning, and closing an unclosed fence
  keeps meaning when its content stays equal.
- Table rows continue on each line that matches every open container, where
  no block starts and whose cells cmark-gfm reads: an optional `|`, then cells,
  each ended by a `|` or the end of the line. A blank line, a lazy line, a line
  with no cell, or a block start, indented code included, ends the table.
- A row of more than 65,535 cells is not a row: cmark-gfm counts cells in a
  `uint16`. A header or delimiter row that long starts no table.
- After 524,288 missing cells, the next line is not a row: cmark-gfm and
  GitHub end the table there and read the line as a paragraph (GitHub API,
  2026-09-13: a header of 1,000 cells and 530 one-cell rows gives 525 body
  rows and a paragraph). The count is the column count times the rows, the
  header row included, minus the cells present up to the column count.
- A table cell splits at each `|` that does not directly follow a `\`. The pair
  `\|` never splits. The scanner reads a cell as cmark-gfm does: the backslash
  of each `\|` pair is removed first, then the inline grammar applies. Leaves
  still cover the source bytes:
  - In a context that decodes backslash escapes (text, destination, title), a
    `\|` pair is a CellPipeEscape leaf, value `|`. An unescaped `\` directly
    before the pair joins it, because after removal it escapes the pipe: `\\|`
    is one CellPipeEscape leaf, value `|`, so `a\\|b` is `a|b` and
    `[a](/u\\|v)` has the destination `/u|v`.
  - In a context that does not decode escapes (code span, raw HTML, autolink,
    label), the pair is a 2-byte CellPipeEscape leaf, value `|`.

### 8.3 Inlines

| Construct | Leaves | Key |
| --- | --- | --- |
| Code span | CodeFence, CodeText (content), VerbatimLineEnding (content), CellPipeEscape, CodeFence | code span value |
| Emphasis, strong, strikethrough | Delimiter | none |
| Link, image | Bracket, inlines, Bracket, Paren, AngleBracket, Destination, TitleQuote, Title, Paren; or Bracket, LinkLabel, Bracket | form; normalized label for references |
| Autolink | AngleBracket, AutolinkText (content), AngleBracket; or, extended, Text, Escape, EntityRef | angle or extended; URL or email |
| Raw HTML | HTMLText (content), VerbatimLineEnding (content) | none |
| Hard break | HardBreakMarker, LineEnding | none |
| Soft break | LineEnding | none |
| Footnote reference | Bracket, Caret, FootnoteLabel, VerbatimLineEnding, Bracket | resolved; normalized label |

### 8.4 Decisions

| Question | Decision |
| --- | --- |
| Line ending kind | Not meaning (product rule 7). |
| Escapes and entity references in text | Compared decoded. A decoded character never becomes syntax. The printer keeps the source form and adds no escape (section 12, roadmap Decisions). |
| Entity references in an angle autolink | Decoded, as cmark decodes them. A backslash is not an escape there (CM 603). |
| Reference link vs inline link | Different meaning: the key has the form and the label. |
| Unused and duplicate definitions | Kept. Compared in document order. |
| Table cells beyond the header count | Kept as content. |
| Missing table cells | Equal to empty cells (section 10.3). |
| NUL, invalid UTF-8 | Raw bytes in leaves. U+FFFD in every character decision and value. |
| Soft break vs space | Different meaning. |
| ASCII control characters in a destination or an absolute URI | A destination ends at space, tab, VT and FF, and takes the other control characters. An absolute URI takes DEL. cmark and commonmark.js do this. The spec text excludes control characters, but no example tests it (section 2). |
| HTML block kind 7 at an open tag named `pre`, `script`, `style` or `textarea` | Starts when the tag is not kind 1, as in `<pre/>`. cmark and commonmark.js do this. The spec text excludes these names, but no example tests it (section 2). |
| Whitespace at the ends of an info string | Trimmed after entity references decode: space, tab, line ending, VT and FF, as cmark trims them. The spec text trims the spaces and tabs of the line only, and no example tests the rest (section 2). |
| VT and FF in a link label | Whitespace: trimmed, collapsed, and blank in a label of them alone, as cmark reads them. The spec text names spaces, tabs and line endings, and no example tests VT or FF (section 2). |
| VT and FF in an HTML tag | Whitespace, as cmark and commonmark.js read them: after a kind 1 or kind 6 tag name, around attributes, in a closing tag, and in inline raw HTML. After the tag of an HTML block of kind 7, cmark takes FF but not VT. The spec text makes whitespace of spaces, tabs and one line ending, and no example tests VT or FF (section 2). |

## 9. GitHub syntax

GitHub renders GFM with cmark-gfm, then runs filters over the HTML. Math and
alerts are filters. Footnotes are grammar. Evidence: GitHub Markdown API,
2026-09-12 (section 17).

### 9.1 Math: a filter

- Inline `$…$` is found inside one DOM text node after inline parsing. The
  opener follows whitespace, `(` or the start of the text node. The closer is
  not followed by `[A-Za-z0-9_]`. Content is decoded text.
- `` $`…`$ `` is `$`, a code span and `$`, with nothing between them.
- `$$…$$` display math is an ordinary paragraph whose content starts and ends
  with `$$`.
- A ```` ```math ```` fence is an ordinary fenced code block with the info word
  `math`.
- Math depends on ancestry: not inside `<em>`, `<a>`, footnote definitions,
  tight list items (display) or `<details>`.

Decision: no math kinds. Every input the filters read is in the comparison:
decoded text by group, text node boundaries (SoftBreak, HardBreak and inline
nodes), paragraph boundaries and subtree ends, looseness, ancestry, the info
string, and code content with its line endings. Equal event sequences with
equal dialect spans give equal GitHub math. The GitHub evidence rows become
stage 6 printer cases.

### 9.2 Footnotes: grammar

Definition (container):

- Start: up to 3 columns of indentation, `[^`, a label, `]:`, and spaces and
  tabs, where the content starts. It can interrupt a paragraph. It is allowed
  in every container, footnote definitions included. It starts only when fewer
  than 99 blocks started before it on its line, as cmark-gfm's
  `MAX_LIST_DEPTH` counts them: `[^a]: ` × 100 gives 99 definitions and the
  text `[^a]: `. cmark-gfm limits list items the same way and cmark 0.31.1
  does not, so markfmt opens every list item (`dialect.md`).
- Label: one or more bytes, none of them `]`, space, tab, CR, LF or NUL; `\]`
  also ends it. NBSP is allowed. `[^a b]: x` is a link reference definition
  with label `^a b`.
- Continuation: 4 columns consumed, or an empty line (after LF, CRLF or CR),
  as section 5.1 says. A later line of any paragraph in the definition can be
  lazy.
- A definition whose first line is empty continues past blank lines.
- Key: raw label. GitHub writes the label as written into element ids.

Reference (inline): section 6.3, step 4. A reference label can span a line
ending (`dialect.md`). Key: resolved and normalized label.

### 9.3 Alerts: a filter

A block quote not inside a block quote, list item, footnote definition or
`<details>`, whose first paragraph's first line is exactly `[!TYPE]` (NOTE,
TIP, IMPORTANT, WARNING, CAUTION, any case, then only spaces or a `\` hard
break), with more content after it, renders as an alert. A link reference
definition with label `!type` stops every alert of that type.

Decision: no alert kind. The comparison sees every input the filter reads.

### 9.4 Emoji, mentions and issue references: filters

GitHub writes an emoji for `:+1:`, a link for `@user`, and, with a repository
context, a link for `#1`. The filters read decoded text: an escape before `:`,
`@` or `#` gives the same result, and a code span stops them (GitHub API,
2026-09-13, `testdata/github/printer.txt`).

Decision: no kinds and no printer rule. `Equal` compares decoded text by group,
so the filters read the same text in input and output.

## 10. Runtime check

`markfmt.Format` parses the input, prints it, parses the output, and calls
`markdown.Equal`. A mismatch, or a recovered panic, returns an error and
writes nothing.

### 10.1 Events

The projection of a tree is an event sequence, produced by an iterative walk:

- `Enter(kind, key)` for each structure node.
- `Content(group, value)` for each run of content leaves of the same group
  under the same structure node. Syntax leaves inside a run do not end it. A
  structure node, or a content leaf of another group, ends it.
- `Exit` when the walk passes a structure node's subtree end.

Structure nodes whose key includes a value emit no Content events for the
leaves that make up that value: CodeBlock (InfoString, code), HTMLBlock,
CodeSpan, and the LinkLabel and FootnoteLabel leaves of LinkReferenceDefinition,
Link, Image, FootnoteDefinition and FootnoteReference. Link and image text
always emits its events, also when it is the label of a collapsed or shortcut
reference. Syntax leaves produce no events.

`Equal` walks both trees in lockstep and compares events. Values are compared
with two decoding cursors, byte by byte, with O(1) extra memory.

### 10.2 Keys

One function with `//exhaustive:enforce` returns the key of a structure node.
Its rows:

| Kind | Key |
| --- | --- |
| Document, BlockQuote, Paragraph, ThematicBreak, TableRow, Emphasis, Strong, Strikethrough, RawHTML, SoftBreak, HardBreak | none |
| FrontMatter | YAML or TOML |
| List | ordered; start, when ordered; loose |
| ListItem | task; checked |
| FootnoteDefinition | raw label |
| Heading | level |
| CodeBlock | info string value; code value |
| HTMLBlock | HTML value |
| LinkReferenceDefinition | normalized label |
| Table | column count |
| TableCell | alignment; header |
| Link, Image | form; normalized label for references (label bytes per section 6.7) |
| Autolink | angle or extended; URL or email |
| CodeSpan | code span value: CellPipeEscape as `\|` value, line endings as spaces, then the one-space strip rule |
| FootnoteReference | resolved; normalized label when resolved, raw label bytes when not (GitHub shows the raw text) |

Values in keys compare through the decoding cursors, not as stored strings.
`Equal` reads a Table's column count once at its Enter event and keeps it for
the rows.

### 10.3 Groups and table rows

- Groups: Text, Escape, EntityRef form the Text group. CellPipeEscape belongs
  to the group of the content leaves around it, which the parser writes in
  its flags (section 3.5): Text in text, Destination in a
  destination, Title in a title, AutolinkText in an autolink, HTMLText in raw
  HTML, LinkLabel or FootnoteLabel in a label. VerbatimLineEnding belongs to the
  group given by its parent: CodeText in a CodeBlock or CodeSpan, HTMLText in an
  HTMLBlock or RawHTML, FrontMatterText in FrontMatter, Title in a title,
  LinkLabel or FootnoteLabel in a label. Every other content kind is its own
  group.
- Table rows: cells compare in pairs up to the smaller present count. On the
  longer side, remaining cells up to the header count must be empty, and cells
  beyond the header count compare as content. The cost is O(present cells).
  No missing cell is ever made.

### 10.4 Dialect spans

From stage 6, each structure node matched by a dialect predicate (section 2.1)
adds its set of `Dialect(row)` values to its Enter event. `Equal` requires:

- equal row sets at every event, in both directions, so the printer can neither
  create nor remove a place where GitHub and CommonMark disagree;
- for each span, equal non-prefix bytes, with a split tab read as its `virt`
  spaces (section 4.3) and each line ending as LF (product rule 7), and for
  each of its lines after the first that is not blank the same number of
  matched containers, so a lazy line stays lazy. The prefix leaves of a line
  do not give that number: a container that consumes columns only inside a
  split tab has no leaf. The cost is O(span bytes).

### 10.5 Tests of the check

- `compare_test.go` has a pair corpus: `testdata/pairs/NAME.a.md` and
  `NAME.b.md`, with a list that marks each pair equal or different. Every pair
  has a reason: a spec example or a GitHub fixture. Each pair lands in the
  commit that adds its constructs. Stage 2 pairs: block quote and blank line,
  code blank lines, math fence info, list start, looseness, code and HTML at end
  of input.
- `FuzzEqual` finds false acceptances: after a syntax-only mutation (bullet
  character, emphasis character, line ending kind, ordered delimiter, fence
  character, blank line count, prefix form), if `Equal` reports equal, the
  test HTML renderings must be equal. The mutation may change meaning; the
  property holds either way, so mutations need no preconditions.
- The pair test checks every "equal" pair against the test HTML: both files
  must render equal HTML, so a wrong pair cannot teach `Equal` to accept a
  change of meaning. A "different" pair renders different test HTML, or names
  the GitHub fixture that shows the difference (math, alerts, dialect rows).
- False rejections are found in two ways:
  - Before stage 6: the "equal" pairs of the pair corpus, each with a reason.
    Rewrites that keep meaning only under conditions (code span padding, table
    row padding, the hard break form, fenced against indented code, a final
    newline) are pairs, one per condition, not fuzz mutations. Writing their
    conditions correctly is printer work, and review round 5 showed that
    hand-written preconditions miss cases.
  - From stage 6: `FuzzFormat` asserts that `Format` never reports a check
    mismatch, and that the test HTML of input and output is equal, so `Equal`
    is not its own oracle. The real printer is the source of meaning-keeping
    rewrites, so every failure is a printer bug or a check bug, and both must
    be fixed. `FuzzFormat` lives in `internal/markdown` as an external test
    (`package markdown_test`) that imports `internal/format` and reaches the
    test renderer through `export_test.go`.
- Stage 6 adds a dialect mutation test: for each `dialect.md` row, rewrite a
  neighbour and assert a dialect span or a rejection.

## 11. Testing

### 11.1 Invariant and timing tests

| Test | Property |
| --- | --- |
| `FuzzParse` | No panic. `Verify` passes. Printing leaves gives the input. |
| Builder checks | Always on. `close()` closes the innermost open node, so mis-nesting cannot be written. |
| `TestParse` subtest `"pathological"` | Each input of section 6.8 at size n and 10n, where the 10n run takes at least 50 ms. Best of 3. Fails at a ratio above 30. `TestParse` does not call `t.Parallel`, with one comment that gives the reason (timing). |
| `TestParse` subtest `"long"` | Each input of section 6.8 runs in a child process (the test binary with `-test.run` and an environment variable), at a tenth of the input limit and at the limit. The child builds the input, then times only `Parse`, `Verify` and `Equal` of the tree with itself, best of 3, and reports the times and `len(nodes)` on stdout. The parent fails the input when: the time ratio between the two sizes is above 30; or the time at the limit is above 20 × (bytes × tb + nodes × tn), where tb is prose time per byte and tn is time per node of `>a` lines, both calibrated in the same run (this catches a linear path with a large constant, such as nested label normalization, whatever its node count); or its peak memory is above the bound (at stage 2, 2 GiB: half the 4 GiB `Format` budget, because `Format` parses two trees; from stage 6, the 4 GiB `Format` budget). Peak memory is `Maxrss` from `ProcessState.SysUsage`, converted by `maxrssBytes` (KiB on Linux, bytes on darwin; unit tested with a child that touches a known size), minus the `Maxrss` of a child that runs the same path on an empty input. `TotalAlloc` is not used: `append` growth allocates about 5 times an array's final size. Skipped under the race detector (a `//go:build race` constant in `race_test.go`) and unless `MARKFMT_LONG=1`. Runs as `task long`, without `-race`, in the ubuntu CI job. |

### 11.2 Conformance

- The conformance subtests are in `TestParse` in `parse_test.go`, paired with
  `parse.go`, and call `t.Parallel`.
- Subtests: `t.Run("commonmark")`, then `t.Run("commonmark example 42")`. The
  example number is its ordinal in the pinned file. Section names appear only
  in failure messages.
- Corpora in `internal/markdown/testdata/<corpus>/`, each with a README that
  gives source, version and license. Every corpus uses the `spec.txt` example
  format, so one reader serves all of them. `spec_test.go` holds the reader,
  and asserts the example count of each pinned file (652 for CommonMark 0.31.2).
- `testdata/<corpus>/failing.txt`: one example ID per line, sorted
  numerically, `#` comments allowed. The test fails when an unlisted example
  fails, when a listed example passes, and when an entry names no example. The
  check runs against the whole corpus, not only the subtests that `-run`
  selected. The first list is generated once by a documented command and
  reviewed as data. No flag adds entries.
- One grammar, in production and in tests. `testdata/<corpus>/grammar-differs.txt`
  lists examples that markfmt's GFM or front matter rules change: CommonMark
  examples, and regression examples that upstream runs without GFM
  extensions. It also lists commonmark.js regression examples where cmark and
  commonmark.js disagree: markfmt follows cmark (section 2), and the named
  case holds the output of `cmark --unsafe`. Each entry gives the rule and names a case in
  `testdata/markfmt/grammar.txt` (`spec.txt` format) with the same input and
  the expected HTML under markfmt's grammar: hand-written for front matter, the
  normalized GitHub fixture for GFM. The named case runs as ordinary
  conformance. The test fails when an entry no longer differs, when its case is
  missing or has a different input, and when an entry is also in
  `failing.txt`. Each file is created in the commit that makes its first entry
  differ: front matter at stage 2, GFM at stage 4. "100%" gates mean "every
  example passes, or is listed here and its named case passes".
- Stage 2 gate: an example "needs inlines" when its expected HTML, outside
  every `<pre>` element, contains an inline element (`em`, `strong`, `a`,
  `img`, `code` or `br`) or a character reference other than `&quot;`,
  `&amp;`, `&lt;` and `&gt;`, or its Markdown contains `\` or `&`. CM 148
  and CM 201 also need inlines: the expected HTML of CM 148 has `<em>` inside
  the raw `<pre>` of an HTML block, and that of CM 201 has the raw inline HTML
  `<bar>`. The block sections are Tabs, Precedence, and the sections of
  Leaf blocks and Container blocks. Every block-section example that does not
  need inlines passes, or is in `grammar-differs.txt` and its named case
  passes: 250 of 296. CM 96
  and 98 are front matter (section 8.1). Link reference definitions get unit tests on the tree. The
  classification is deleted in the commit that passes the stage 3 gate.
- `internal/format/testdata/spec` (goldmark's `spec.json`) stays as data for
  the idempotence subtest of `TestSource`.

### 11.3 Test HTML renderer

- `html_test.go` in `package markdown`: a test helper, not the test of one
  source file. It is not linked into any binary.
- It normalizes HTML as cmark's `normalize.py` does.
- It writes missing table cells, as cmark-gfm does. Above cmark-gfm's cap of
  524,288 missing cells, cmark-gfm and GitHub read no more rows, so the cap is
  grammar (section 8.2). markfmt follows GitHub there, so the cap is not a
  `dialect.md` row. Appendix B trap 14 keeps the count.
- It applies the GFM tag filter to raw HTML, as cmark-gfm's renderer does,
  where upstream enables the filter: examples whose fence names `tagfilter`,
  every `cmark-gfm-extensions` example, and the GitHub fixtures. The filter is
  rendering, not grammar (section 2), so its examples need no grammar rule.
- The GitHub normalizer lands at stage 4 with the fixtures. It rewrites the
  test HTML and the fixture with a fixed list, each with a unit test: `<br>` as
  a space (mode `gfm` writes `<br>` for every soft break, so the fixtures do not
  tell soft breaks from hard breaks), the table wrapper and `role`,
  `notranslate` classes, `rel` attributes, the link and style around an image,
  task list classes, empty ids and labels, the footnote heading, the footnote
  classes and back reference index of GitHub and cmark-gfm, and the
  `user-content-` prefix and hash of footnote ids.

### 11.4 GitHub fixtures

- Format: `spec.txt` examples. A file header gives the capture date and the
  `gh api markdown` command. Capture is a shell loop documented in the README.
  Tests never call the network.
- Footnote, task list, table and strikethrough fixtures are conformance cases
  from stage 4.
- Each `dialect.md` row names its fixture. A row where markfmt differs from
  GitHub is an entry in `testdata/github/grammar-differs.txt`, whose
  `markfmt/grammar.txt` case holds markfmt's result under the row's rule. A row where markfmt follows GitHub is an ordinary fixture. GitHub
  removes most raw HTML, so the input of a raw HTML row shows its rule in
  visible text.
- Math and alert fixtures are printer cases at stage 6.

### 11.5 Differential fuzzing (stage 5)

- `internal/markdown/differential_test.go` holds `FuzzDifferential`. It
  compares the test HTML of `Parse` with the HTML of goldmark v2.0.2, both
  after `normalizeHTML`. goldmark runs with `html.WithUnsafe`, with
  `html.WithXHTML` so that a void element inside script or style content,
  which `normalizeHTML` keeps as written, has the form of the test renderer,
  and with no extensions.
- goldmark reads the input with LF line endings, a final line ending, and
  U+FFFD for NUL and for each maximal invalid UTF-8 subsequence. CommonMark
  gives the line ending forms the same meaning (product rule 7) and replaces
  NUL (spec 2.3), but goldmark does not: rows 4 to 6 of the roadmap's "Known
  goldmark deviations", and a NUL in a destination. markfmt reads invalid
  UTF-8 as U+FFFD (section 8.4), as cmark does in labels, and goldmark reads
  the bytes. `Parse` reads the input as written, so its line ending and
  decoding rules stay under test.
- No goldmark extension is on. goldmark's tables, strikethrough, linkify and
  footnotes are not cmark-gfm. With its table, strikethrough and task list
  extensions on, goldmark disagrees on 81 corpus examples that have a GFM
  construct (2026-09-13). The corpora and the GitHub fixtures test GFM,
  GitHub footnotes and front matter (sections 11.2, 11.4).
- `goldmarkDiffers` returns the reason to skip an input, or nothing. The fuzz
  function skips an input that has a reason:
  - markfmt grammar outside CommonMark: the tree has a Table, Strikethrough,
    FootnoteDefinition, FootnoteReference, FrontMatter, extended Autolink or
    task ListItem, or a Text or Delimiter leaf holds `~`. A `~` run can make
    emphasis delimiters text with no Strikethrough node (section 6.4).
  - A known goldmark deviation: one predicate per row that shows in HTML.
    The rows about positions do not show in HTML and have no predicate.
- Each disagreement becomes a case in `testdata/differential/cases.txt`, in
  the `spec.txt` format, which runs as conformance. The section of a case
  gives its verdict: `fixed in markfmt: RULE` or `goldmark deviates, spec
  section X: RULE`. The expected HTML is the output of `cmark --unsafe`
  0.31.1, or of cmark-gfm 0.29.0.gfm.13 with its extensions for a GFM rule.
  `TestGoldmarkDiffers` checks each case: a predicate matches a "goldmark
  deviates" case and goldmark still disagrees on it; no predicate matches a
  "fixed in markfmt" case and goldmark agrees on it.
- Triage uses the spec text first, then cmark 0.31.1 for core rules and
  cmark-gfm 0.29.0.gfm.13 for GFM rules (section 2). A markfmt bug lands as a
  failing test in the component that owns it, the fix and its case. A goldmark
  deviation lands as its row, its predicate and its case, in one commit.
- goldmark follows CommonMark. On the input of a `dialect.md` row it can give
  GitHub's result, or commonmark.js's where cmark and commonmark.js disagree.
  That disagreement is a goldmark deviation with the row's rule. Any other
  result on a dialect row input is triaged like any disagreement.
- The seeds are the examples of every corpus. The inputs that the fuzzer
  writes to `testdata/fuzz/FuzzDifferential` are not committed: each one is a
  case in `cases.txt`.
- Budget: the stage 5 gate is a run of 1 CPU-hour (workers × wall time) with
  no disagreement that is not triaged. A run of 24 CPU-hours is an exit
  criterion of stage 7, before goldmark goes.
- goldmark stays a test-only requirement in the root `go.mod` until stage 7
  deletes this file. It then appears in a consumer's `go.sum` and module list,
  but not in its build. No release happens before stage 7.

### 11.6 Performance

`BenchmarkParse` parses three inputs with markfmt and with goldmark v2.0.2
with no extensions: CommonMark `spec.txt` as one document, the inputs of every
corpus joined, and `docs/design/parser.md`. A manual gate at stage 3, recorded
with `benchstat` output in the roadmap: parse throughput within 2 times
goldmark's, with pass 1 included. `benchstat` runs with `go run
golang.org/x/perf/cmd/benchstat@VERSION`, not from `tools/go.mod`.

## 12. Printer interface

The printer is stage 6. This section records what the tree and the check
require of it. Appendix B collects printer traps with byte bounds.

- **Facts.** The printer reads facts only through accessors, and skips prefix
  leaves. It writes container prefixes from its own stack.
- **Kept syntax.** Printer output is a function of the projection plus a named
  set of kept syntax: escape and entity forms, raw label bytes, each NUL and
  invalid UTF-8 sequence in a content leaf, the lazy numbering of an ordered
  list (appendix B, trap 11), and for each
  dialect span its non-prefix bytes, the number of matched containers on each
  of its lines (so lazy lines stay lazy), and the blank lines before and after
  it. At stage 6, a `Kept(t)` event stream sits next to the projection, and the
  fuzz gate asserts it is equal for input and output. Anything outside the set
  is canonical: code block style, heading style, emphasis character.
- **Size decisions.** Every size-based printer decision reads only measures
  that printing does not change: the projection, `Kept`, and the printer's own
  canonical output of the construct. It never reads source byte counts. So a
  second format decides the same way.
- **Output size.** The printer writes into a writer that fails at the output
  limit (section 7.2). Appendix B gives each expanding rule its own bound.
  The stage 6 corpus gate reports the largest output-to-input ratio, and fuzz
  cases sit at each bound's threshold.

## 13. Package layout

```
internal/markdown/
  tree.go         Node, Tree, Verify, Walk, Raw, flag layouts
  kind.go         Kind enum, classes, groups
  builder.go      the only writer of nodes; always-on checks
  lines.go        line iterator, columns, tabs, BOM
  parse.go        Parse, pass driver, label lists
  block.go        per-line loop, open stack, lazy lines, emission order, pending blocks
  blockquote.go  list.go  footnote.go  heading.go  thematic.go
  code.go  htmlblock.go  linkref.go  table.go  frontmatter.go
  inline.go       scan, trigger table, emit, email autolinks
  delimiter.go    emphasis and strikethrough
  link.go         links, images, references, footnote references
  codespan.go  autolink.go  rawhtml.go  entity.go  escape.go  task.go
  entities.go     generated
  chars.go        decoding, character classes, labels
  compare.go      Equal, keys, groups, dialect spans
  dialect.go      dialect predicates (section 2.1)
  testdata/       corpora, failing and grammar-differs lists, pairs, dialect.md, entities/entities.json, unicode/CaseFolding.txt
```

- One package for the parser and the tree. The parser internals have no other
  user.
- Each construct file holds its byte-level fact functions and their accessors
  (R-fact).
- `entities.go` is generated from the pinned `testdata/entities/entities.json` by a
  `//go:build ignore` generator with a `//go:generate` line, as the standard
  library does. `entities_test.go` checks the table against the JSON.
  `casefold.go` is generated the same way from the C and F rows of
  `testdata/unicode/CaseFolding.txt`, and `casefold_test.go` checks it.

## 14. Tooling

The first stage 1 commit adds:

- `.golangci.yml`: `exhaustive` with `explicit-exhaustive-switch: true` and
  `default-signifies-exhaustive: false`.
- `Taskfile.yml`: `task fuzz` takes `PKG` and `FUZZ` variables.

Stage 2 commit 20 adds `task long` and runs it in the ubuntu CI job.

## 15. Stage plan

Each commit passes `task ci`. Each new test fails before the code that makes it
pass.

Stage 1, harness:

1. Tree, builder and `Verify`, with kinds Document and Text. `Parse` gives the
   Document plus the whole input as one Text leaf. `TestTree_Verify` builds bad
   trees by hand, one subtest per invariant. `FuzzParse`. Tooling.
2. Line iterator, LineEnding and BOM leaves.
3. Pin CommonMark 0.31.2 `spec.txt`, with the reader and the count test.
4. Test HTML renderer and `normalize.py` normalization.
5. Conformance with `failing.txt` (652 entries), stale checks and the "needs
   inlines" classification.
6. Pin the GFM and regression corpora, one commit per corpus.

Stage 2, blocks:

7. Paragraphs and blank lines: the block loop with only the document.
8. Thematic breaks. 9. ATX headings. 10. Indented code. 11. Fenced code.
12. Setext headings.
13. HTML blocks, with the block rows of `dialect.md` (tag lists, kind 4
    start). The comment row has no block part: kind 2 starts and ends the same
    way in both grammars.
14. Block quotes: continuation, starts, lazy lines, prefix leaves, emission
    order, and the kind 7 lazy line row. This is the largest commit.
15. Tabs and `virt`.
16. List items and lists, with looseness.
17. Link reference definitions and labels, with tree unit tests, the setext
    re-dispatch rule and its prefix rule, and the `[foo]: /url` then `---` row.
18. Front matter, `commonmark/grammar-differs.txt` and
    `markfmt/grammar.txt`.
19. `Equal` for block kinds, the stage 2 pairs, `FuzzEqual` with the block
    mutations only. Label normalization with the full case folding table,
    generated from the pinned Unicode `CaseFolding.txt`: Go's `unicode` package
    has only simple folding, and CM 540 needs `ẞ` to match `SS`.
20. The pathological block inputs, the long test and `task long`.

Stage 3, inlines. Each commit extends `Equal`, the pairs, the pathological
inputs and the `FuzzEqual` mutations with its constructs:

21. The inline phase and line breaks: the scan at paragraph and heading close
    over the block's lines, with prefix and Indent leaves in place; Text,
    TrailingSpace, SoftBreak, HardBreak and HardBreakMarker. Remove the
    `compare.go` stand-in rule for line endings. Line breaks come first
    because the scan classifies spaces before a line ending from its first
    version (section 6.1).
22. Backslash escapes. Text group values decode in `Equal` and the test
    renderer, with NUL and invalid UTF-8 as U+FFFD (section 8.4).
23. Entity and numeric character references, with the table generated from
    the pinned WHATWG `entities.json` (section 13).
24. Decoded Destination, Title and InfoString values (sections 8.4, 10.3).
25. Code spans, with the per-block index of backtick runs.
26. Angle autolinks.
27. Raw HTML, with the per-block memo of failed closer searches and the inline
    part of the comment row.
28. Emphasis and strong emphasis, with `openers_bottom`, side records and the
    flanking row.
29. Inline links and images, with the bracket stack and `linkFormedAfter`.
    One destination and title reader serves `link.go` and `linkref.go`.
    Between destination and title, spaces are Whitespace leaves and a line
    ending is a LineEnding leaf, as in a definition.
30. Pass 1, the pass label check, and full, collapsed and shortcut
    references.
31. `BenchmarkParse` against goldmark (section 11.6).
32. The stage 3 gate: the last corpus fixes, delete the "needs inlines"
    classification, record `benchstat` output.

Stage 4, GFM and GitHub syntax. Each commit extends `Equal`, the pairs, the
pathological inputs, the `FuzzEqual` mutations and the GitHub fixtures with its
constructs, and checks each new linear-time mechanism with a mutation:

33. This plan, the footnote reference rules of sections 6.3 and 6.7, and the
    tag filter and dialect fixture rules of sections 11.3 and 11.4.
34. The GFM tag filter in the test renderer. The example reader keeps the
    extension words of an example's fence (section 11.3).
35. GitHub fixtures in `testdata/github`, with the capture loop, the GitHub
    normalizer, `failing.txt`, and the fixtures of the existing `dialect.md`
    rows with `github/grammar-differs.txt` (section 11.4). Math, alert and
    plain text fixtures (`:+1:`, `@octocat`, `#1`, and each with an escape) are
    captured for stage 6 and not tested.
36. Strikethrough, with `openers_bottom` for `~` per length.
37. Tables: the delimiter row candidate, the header split, rows, cells with
    inlines, alignment flags, the column count, row comparison in `Equal`, and
    missing cells in the test renderer. Measure cmark-gfm and GitHub above
    524,288 autocompleted cells (section 11.3).
38. CellPipeEscape in every inline context, label bytes, the code span value,
    groups by context, and the paragraph split off above a table with its
    `dialect.md` row.
39. Task list items: TaskBox, the ListItem task and checked flags, and a
    defined `[x]` that wins.
40. Extended www and URL autolinks at `w` and `:`, with the angle or extended
    key. CM 602, 608 and 611 go in `grammar-differs.txt`.
41. Extended email autolinks at emit. CM 606 and 612 go in
    `grammar-differs.txt`.
42. Footnote definitions: the container, its label rule, FootnoteIndent, and
    the pass 1 footnote label list with the pass 2 check.
43. Footnote references: section 6.3 step 4, the resolved flag, the footnote
    section of the test renderer, and the `dialect.md` rows for an escaped or
    entity `^` and for a label across a line ending.
44. The comment pass.
45. The per-line indentation memo of section 5.1, which the long test of
    the gate showed missing.
46. The stage 4 gate.

Stage 5, differential fuzzing. Each commit that triages a disagreement adds its
case to `testdata/differential/cases.txt` (section 11.5). A goldmark deviation
adds its Known goldmark deviations row and its predicate. A markfmt bug adds a
failing test in the component that owns it, then the fix. The known
disagreements come from the corpora (2026-09-13):

47. `[foo]: /url` then `---` follows cmark 0.31.1: the underline line is
    paragraph text (`dialect.md`, github 10).
48. This plan, section 11.5, and the stage 5 and stage 7 budgets in the
    roadmap.
49. `differential_test.go`: `FuzzDifferential` against goldmark v2.0.2 with no
    extensions, goldmark's canonical input, and `goldmarkDiffers` with the
    markfmt grammar reason, one subtest per construct. Seeds: the `commonmark`
    corpus, where goldmark agrees on every example outside markfmt's grammar.
    The Known goldmark deviations table records how the test handles each row.
50. `testdata/differential` with its README, `failing.txt` and conformance,
    and the case checks of `TestGoldmarkDiffers`. The first case:
    `<!doctype html>` and `x<!x>`. goldmark needs an uppercase letter after
    `<!`, as GitHub does (spec 4.6 kind 4, 6.6; `dialect.md`).
51. The HTML block kind 6 tag list: goldmark's list has `meta`, which the spec
    does not list (spec 4.6).
52. A closing tag of `pre`, `script`, `style` or `textarea` starts kind 7, and
    goldmark gives a paragraph (spec 4.6; cmark-gfm regression 17).
53. An angle destination with an unescaped `<`, which goldmark accepts (spec
    6.3; cmark-gfm regression 6).
54. A setext underline after definitions alone: goldmark gives a thematic
    break, as commonmark.js does (`dialect.md`).
55. A blank line in a code or HTML block of a list item: goldmark makes the
    list loose, as commonmark.js does, and markfmt follows cmark (spec 5.3;
    commonmark.js regression 25).
56. The info word `language-r`: goldmark writes the class
    `language-language-r` (spec 4.5; commonmark.js regression 28).
57. `[0]:`, a destination line, then a line that is not a title: goldmark
    repeats the destination line as paragraph text (spec 4.7, CM 210).
58. Seeds from every corpus.
59. Fuzz findings, one commit each, until a run of 1 CPU-hour finds none. A
    new dialect row needs a GitHub fixture, which shifts the IDs after it:
    renumber `github/failing.txt` and `github/grammar-differs.txt`.
60. The comment pass.
61. The stage 5 gate.

Stage 6, printers on the new tree. Each printer commit adds cases to
`internal/format/testdata/cases` for the traps of appendix B that it meets,
and removes the cases it makes pass from `testdata/cases/failing.txt`.
`FuzzFormat` runs its seeds in every `task ci`. A fuzz finding lands as a
failing case in the component that owns it (the printer, `Equal` or a dialect
predicate), then the fix. A printer commit that picks a form the survey does
not cover adds a Decisions row with the reason.

62. This plan, and trap 21 of appendix B.
63. The canonical style (roadmap Open decisions 1). The survey reads the source
    and docs of Prettier 3.9.6, dprint-plugin-markdown 0.24.0, mdformat 1.0.0
    with mdformat-gfm 1.0.0, markdownlint 0.41.1, and the Google Markdown style
    guide at commit `895579e`. A source that keeps the input form, or accepts
    any consistent form, has no vote. A construct has a majority when more than
    half of its votes choose one form. Each construct with a majority is a
    Decisions row with its evidence. The other constructs are questions to the
    user, in one ask with the question of commit 64.
64. GitHub fixtures for the six `dialect.md` rows marked "Not captured yet",
    after the user allows the API call. The new fixtures shift the IDs after
    them: renumber `github/failing.txt` and `github/grammar-differs.txt`, and
    add the `markfmt/grammar.txt` cases.
65. `format.Source` on the new parser: parse, print, parse the output, and
    `Equal`. `markfmt.Format` recovers a panic as an error with the stack
    (design 1). The input limit, and an output writer that fails at 16 MiB
    (design 7.2). The first printer writes every leaf as it is, with LF line
    endings. Delete the goldmark-based formatter, `checkRendering` and
    `FuzzSource`. `testdata/spec` stays as data for the idempotence subtest of
    `TestSource`. `testdata/cases/failing.txt` lists the cases that need a
    printer, with the list rules of section 11.2. Rename the cases whose names
    describe the goldmark formatter. `task ci` fails when `go list -deps
    ./cmd/markfmt` lists a goldmark package.
66. `FuzzFormat` in `internal/markdown`, as `package markdown_test`, with
    `export_test.go` for the test renderer (section 10.5). It asserts that
    `Source` returns no error, that `Source` of the output gives the output,
    and that input and output render equal test HTML. Seeds: every corpus, the
    GitHub fixtures, the pairs, `testdata/cases` and the `FuzzSource` inputs.
67. `Kept`, a test-only event stream (section 12): Escape and EntityRef leaves
    with their bytes, raw label bytes, and each NUL and invalid UTF-8 sequence
    in a content leaf with its bytes. cmark writes an invalid byte in a
    destination as `%A6`, where markfmt's value is U+FFFD, so the printer keeps
    the byte. `FuzzFormat` asserts equal `Kept` streams for input and output.
68. Dialect spans in `Equal` (section 10.4). `dialect.go` finds the spans of a
    tree in one walk. `Equal` compares the row set of each Enter event in both
    directions, the non-prefix bytes of each span, and the matched containers
    of each span line: its prefix leaves, and the list items and footnote
    definitions whose indentation ends inside a split tab (section 4.3).
    `Kept` adds the blank lines around each span. The first predicate is the
    kind 6 row for `search`. The dialect mutation test of section 10.5 starts
    here.
69. Predicates for the other HTML rows: `source`, the kind 4 start, kind 7 on
    a lazy candidate line, and raw HTML comments.
70. Predicates for the inline rows: flanking next to a Unicode symbol or
    U+FFFD, a code span after an unmatched backtick run, VT and FF in a
    destination, VT and FF in a link label, and label length.
71. Predicates for the other rows: a setext underline after definitions alone,
    a failed definition title, VT and FF in an info string, the paragraph split
    off above a table, list items that start on one line, a footnote reference
    whose caret is an escape or an entity reference, and a footnote reference
    label across a line ending. Each predicate commit adds its pairs, with the
    GitHub fixture as the reason, and extends the dialect mutation test.
72. The block printer: the document, one blank line between blocks, one final
    newline, block quote and footnote definition prefixes from the printer's
    stack, lazy lines (trap 2), prefix tabs (trap 20), and blocks per line
    (trap 21). Leaf blocks keep their non-prefix leaves.
73. Lists: the bullet marker, ordered numbering and delimiter, content
    indentation, adjacent sibling lists (trap 3), numbering bounds (trap 11),
    blank lines from looseness (trap 15), and task boxes.
74. Paragraphs and line breaks: no indentation and no trailing spaces, the hard
    break form (trap 10), and continuation lines that would start a block
    (trap 1).
75. Headings: ATX, multi-line setext headings (trap 5), and content that ends
    in `#` (trap 6).
76. Thematic breaks (traps 4, 7).
77. Code blocks: fenced, the fence character and length, the info string, and
    `virt` spaces (traps 8, 20).
78. HTML blocks (trap 12), front matter (trap 7), and link reference
    definitions: label, destination and title bytes, and title quotes.
79. Emphasis, strong emphasis and strikethrough (traps 13, 19).
80. Code spans (traps 9, 14), links, images, autolinks (trap 13), raw HTML,
    escapes, entity references and footnote references.
81. Tables: columns aligned by display width, from a table generated from the
    pinned Unicode `EastAsianWidth.txt` (section 13); the delimiter row; pipe
    escapes; the size bound and the missing cell cap (trap 14).
82. Footnote definitions: label bytes, continuation and order (trap 18).
83. Math and alert fixtures as printer cases (traps 16, 17). Each input of
    `github/printer.txt` has a case in `testdata/cases`, and a test checks that
    every fixture has one. If the user allows it, the capture loop also sends
    each case output to the API, and the test checks that the HTML of the
    output equals the HTML of the input.
84. The last cases: `testdata/cases/failing.txt` is empty and deleted.
85. Output bounds: the largest output-to-input ratio over the corpora, fuzz
    seeds at the threshold of each bound (section 12), the tab expansion bound
    (section 17), and `Format` in the long test within the 4 GiB budget
    (sections 7.2, 11.1).
86. The comment pass.
87. The stage 6 gate: a `FuzzFormat` run of 1 CPU-hour (workers × wall time)
    with no finding, as for stage 5; `go list -deps ./cmd/markfmt`; and the
    roadmap.

## 16. Roadmap changes

Apply these in the commit that marks stage 0 done.

- Stage 0: the goldmark review item is met by appendix C.
- Decisions, "Grammar scope" row: footnotes are grammar; math and alerts are
  GitHub rendering filters that the runtime check protects (section 9). Remove
  "Math in the grammar stops the printer from changing math source".
- Decisions, "Runtime safety check compares our own trees" row: replace with
  "compares event sequences with keys, content groups and dialect spans"
  (section 10).
- Decisions, new rows: dialect rows and predicates (2.1); 8 MiB input and
  16 MiB output limits (7.2); no test-only grammar switch, and
  `grammar-differs.txt` lists (11.2); streaming not planned (7.4); `Format`
  recovers panics (1).
- Stage 1: builder checks always on; `exhaustive` explicit mode; conformance
  runner details (11.2); timing test at n and 10n (11.1); no per-input timer in
  fuzzing.
- Testing strategy, "No panic, linear time" row: replace "fuzz with a time
  limit" with "the pathological and long subtests of `TestParse`, and `task
  long`" (11.1).
- Stage 2 gate: every block-section example that does not need inlines passes
  (250 of 296); pathological block inputs, including `- `×n `a` and deep
  lists with blank lines; the long test; `Equal` for block kinds with the stage
  2 pairs and `FuzzEqual`.
- Stage 3: pass 1 with reference links; the pass label check; the `benchstat`
  gate (11.6). Gate wording: CommonMark at 100% means every example passes, or
  is in `grammar-differs.txt` and its named case passes.
- Stage 4: remove the math and alert grammar items and the "disallowed raw
  HTML" item. Add task list items per GitHub (6.5), footnote references per
  section 6.3, GitHub fixtures for `dialect.md` rows. Gate: footnote, task,
  table and strikethrough fixtures at 100%. Math and alert fixtures are captured
  for stage 6.
- Stage 5: differential fuzzing is `internal/markdown/differential_test.go`
  with goldmark as a test-only requirement of the root module, not a separate
  module, and with known-deviation predicates.
- Stage 6: remove the item "The runtime check: parse input and output, compare
  the trees" (it lands in stages 2 and 3). Add the printer requirements of
  section 12, dialect predicates and spans, `Kept`, the output bound, and
  `FuzzFormat` (no check mismatch on any input, section 10.5). Gate
  command for "goldmark is no longer in the binary": `go list -deps
  ./cmd/markfmt` lists no goldmark package.
- Stage 7: delete `differential_test.go` and run `go mod tidy`. Gate wording:
  all corpora at 100% means every example passes, or is listed in a
  `grammar-differs.txt` and its named case passes.

## 17. Research and reviews

Local reports (not tracked), 2026-09-12:

- goldmark v2.0.2 design review, with root causes of the 7 known deviations and
  5 quadratic inline cases.
- Requirements from CommonMark 0.31.2, GFM 0.29, cmark-gfm tests and GitHub API
  probes, per construct.
- Prior art on lossless trees: Roslyn, swift-syntax, rowan, tree-sitter and
  tree-sitter-markdown, micromark, markdown-rs, comrak,
  dprint-plugin-markdown, Prettier, mdformat.
- GitHub math, footnote and alert behaviour from the GitHub Markdown API.
- Review round 1: spec (0 blocking, 10 important, 10 minor), printer and check
  (2, 9, 7), performance (3, 7, 9), engineering (2, 10, 9), red team (42
  inputs: 16 broken, 14 underspecified).
- Review round 2: spec (0, 4, 10), printer and check (0, 7, 7), performance
  (1, 7, 5), engineering (0, 6, 6), red team (42 old inputs all handled; 26 new:
  8 broken, 8 underspecified).
- Review round 3: spec (0, 0, 3), printer and check (0, 3, 2), performance
  (0, 4, 5), engineering (0, 4, 2), red team (68 old inputs all handled; 17
  new: 1 broken, 4 underspecified).
- Review round 4, verification: spec (0, 0, 0), printer and check (0, 1, 0),
  performance (0, 2, 0), engineering (0, 1, 0), red team (all round 3 cases
  handled; 8 new: 1 broken). Each finding is fixed in draft 4.
- Review round 5, verification of the post-round-4 edits: end-of-input rule
  sound (14 cases); table cell pipe rule fixed for labels and footnote labels;
  long test revised to a two-term time bound, `Maxrss` per OS and timing inside
  the child, then confirmed; `FuzzEqual` preconditions replaced by pairs
  checked against test HTML and `FuzzFormat` with the test HTML as oracle.
- Open, not blocking: a bound on tab-to-space expansion for inputs near the
  input limit (stage 6 printer design).

## Appendix A. Kinds

A stage may split a kind when a construct needs it. The class of each kind
stays fixed.

Structure (interior):

- Blocks: Document, FrontMatter, BlockQuote, List, ListItem,
  FootnoteDefinition, Paragraph, Heading, ThematicBreak, CodeBlock, HTMLBlock,
  LinkReferenceDefinition, Table, TableRow, TableCell.
- Inlines: Emphasis, Strong, Strikethrough, Link, Image, Autolink, CodeSpan,
  RawHTML, SoftBreak, HardBreak, FootnoteReference.

Content (leaf):

- Text group: Text, Escape, EntityRef.
- Group by context (section 10.3): CellPipeEscape, VerbatimLineEnding.
- Own groups: CodeText, InfoString, FrontMatterText, HTMLText, Destination,
  Title, LinkLabel, FootnoteLabel, AutolinkText.

Syntax (leaf):

- Lines: BOM, LineEnding, BlankLine, Indent, Whitespace, TrailingSpace.
- Prefix leaves (with an owner in `link`): QuoteMarker, ListMarker,
  ItemIndent, FootnoteIndent.
- Blocks: TaskBox, ThematicRun, ATXMarker, ATXClose, SetextUnderline,
  FenceMarker, CodeIndent, FrontMatterFence, TablePipe, TableDelimiter.
- Inlines: Delimiter, Bracket, Paren, AngleBracket, Colon, Caret, TitleQuote,
  CodeFence, HardBreakMarker.

Flags:

| Kind | Flags | Also in bytes |
| --- | --- | --- |
| List | loose | no |
| ListItem | task, checked | TaskBox leaf |
| HTMLBlock | kind 1 to 7 | start condition of the first line |
| Link, Image | form: inline, full, collapsed, shortcut | no |
| TableCell | alignment, header | delimiter row cell |
| CellPipeEscape | group: Text, CodeText, HTMLText, AutolinkText, Destination, Title, LinkLabel or FootnoteLabel | no |
| FootnoteReference | resolved | no |

## Appendix B. Printer traps for stage 6

Collected from the research and reviews. They are inputs to the stage 6
design, not parser gates.

1. A paragraph continuation line whose content would start a block, a setext
   underline or a delimiter row after the printer's prefix keeps 4 columns of
   indentation. No block starts there at 4 columns, and the printer adds no
   escape (section 12). The rule depends on content, not on whether the source
   line was lazy, so it is idempotent (CM 238).
2. A lazy line stays lazy when its full canonical prefix is longer than its
   printed content (after trap 1). Inside a dialect span, lazy lines always
   stay lazy (section 10.4).
3. Adjacent sibling lists of one type alternate the marker by position (`-`,
   `*`; `.`, `)`). No `<!-- -->` separator: it adds a node.
4. A thematic break in a list item uses a character different from the
   bullet, and a thematic break after a paragraph line or a link reference
   definition line never uses `---`.
5. A multi-line setext heading stays setext.
6. ATX content that ends in `#` gets a closing sequence: `Foo #` is
   `## Foo # ##`.
7. The first block never looks like front matter.
8. Fence length exceeds any run of the fence character at a content line
   start. An info string with a backtick needs a tilde fence. Fenced content
   that starts or ends with blank lines cannot become indented code (CM 117).
9. Code span backtick count and padding follow the content.
10. A hard break after a literal trailing `\` keeps its trailing spaces:
    `\\` is an escaped backslash.
11. Sequential numbering never exceeds 9 digits. A list that starts at 1 and
    whose second item is 1 numbers every item 1 (kept syntax, section 12). A
    list that interrupts a paragraph keeps start 1. Renumbering adds at most the width difference of
    the largest number to each continuation line.
12. A blank line follows an HTML block of kind 6 or 7. Unclosed kinds 1 to 5
    keep trailing blank lines as content.
13. No escapes inside extended autolinks. No new `~` next to a `~~` delimiter.
14. `|` in table cells is escaped, also in code spans. Short-row padding and
    column alignment together add no more bytes than the size of the unpadded
    canonical table. Otherwise rows stay short and columns unaligned (section
    10.3 treats both as equal). Padding a short row lowers the count of missing
    cells, so a table whose count reaches 524,288 keeps its short rows, and
    no printed row grows beyond it (section 8.2).
15. Blank lines inside lists come from looseness.
16. GitHub math: no change to characters next to `$`, to line breaks, or to
    blank lines and looseness around a `$$` paragraph. Never convert between a
    `math` fence and `$$`.
17. GitHub alerts: the `[!TYPE]` line stays alone and first.
18. Footnotes: definition label bytes, 4-column continuation (8 for indented
    code), and definition order stay.
19. An emphasis delimiter next to a character in Unicode S but not P, or next
    to U+FFFD, uses `*`: `£*a*£` is emphasis under both rules, `£_a_£` only
    under CommonMark.
20. Tabs: a tab inside code content is content and stays. Structural
    indentation is written from the printer's canonical prefix, whatever tabs
    the source used. `virt` adds at most 3 spaces per line.
21. A printed line starts at most 99 blocks, unless a dialect span keeps the
    line. So a footnote definition stays a definition (section 9.2), and no
    printed line of list items becomes a `dialect.md` span. A container whose
    first child would be block 100 on its line puts that child on the next
    line.

## Appendix C. goldmark v2: taken and dropped

From the goldmark v2.0.2 design review (section 17). goldmark was read for
design only; no code is copied.

| goldmark design | Verdict | Reason |
| --- | --- | --- |
| Two phases: blocks, then inlines with all definitions known | Take | The CommonMark appendix algorithm. |
| Per-line open-block stack: continue, start, lazy, close | Take, rewritten | Sound. markfmt records consumed prefixes as leaves. |
| First-byte dispatch tables for block and inline triggers | Take | Cheap and simple. |
| Raw source spans with decoding on demand | Take | Raw bytes stay the truth; decoded text is a view. |
| The spec's delimiter-run algorithm | Take, adapted | Delimiter stack kept outside the tree; `openers_bottom` added (goldmark lacks it and is quadratic). |
| Byte tables, `IndexByte` scanning, no-escape fast paths | Take | Proven. |
| Pointer tree with a 30-method `Node` interface and mutation API | Drop | Not lossless, heap-heavy, and its `InsertBefore` miscounts children. |
| Start-only positions set by hand, in mixed units | Drop | Root of 6 of the 11 fuzz findings. Spans derive from leaves. |
| `Segment.Padding`, `ForceNewline`, owned strings | Drop | Values differ from source bytes. `virt` and the end-of-input rule replace them. |
| Paragraph transformers and AST transformers | Drop | Root of 4 of the 7 known deviations. Decisions happen at the deciding line. |
| Line splitting at `\n` only; end of input per parser | Drop | One line iterator for LF, CR and CRLF. |
| Global kind registry, priorities, untyped context slots, extension API | Drop | A closed grammar needs a closed `Kind` and exhaustive switches. |
| HTML concerns in the tree (attributes, heading IDs, rune helpers) | Drop | markfmt has no HTML output. |
| Generic renderer with reflection and decorators | Drop | One printer and one test renderer, each a switch. |
