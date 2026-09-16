package format

import (
	"bytes"
	"slices"
	"strconv"
	"strings"

	"github.com/strangemattersystems/markfmt/internal/markdown"
)

var (
	lineFeed    = []byte{'\n'}
	quotePrefix = []byte("> ")
	spaces      = bytes.Repeat([]byte{' '}, 64)
)

// printer writes a tree in the canonical style.
type printer struct {
	tree *markdown.Tree
	out  []byte
	max  int  // the output limit
	full bool // a write would have passed max, so out is incomplete

	layout markdown.Layout
	spans  []markdown.NodeID           // the dialect spans that the walk has not passed
	lazy   map[markdown.NodeID]bool    // the list items that lazyItems finds
	breaks map[markdown.NodeID][2]bool // the lists that prescan finds
	stack  []frame                     // the open structure nodes, the document first

	lineStart bool          // the output is at the start of a line
	inputLine bool          // the next leaf starts a line of the input
	lineBegin bool          // the leaf that the printer reads starts a line of the input
	matched   int           // the containers that the input line of the output line matched
	indent    int           // the input column where columns that are not written yet start, or -1
	inSpan    int           // the open dialect spans
	inLabel   int           // the open collapsed and shortcut references, whose text is their label
	inStrike  int           // the open strikethrough nodes
	inUnder   int           // the open emphasis nodes that print with '_'
	written   bool          // the last leaf block that started has written content
	leafKind  markdown.Kind // the kind of the last leaf block that started
	prefixes  int           // the open containers of the last leaf block that started
	prefixLen int           // the bytes of the rest of those containers
	pad       int           // spaces to write after the prefix of the next line
	backslash bool          // the last byte written is a backslash that is not an escape
	bracket0  bool          // the open paragraph starts with '[', so it could start with a link reference definition
	afterText bool          // the open block follows a paragraph or definition line without a blank line
	lead      bool          // the next leaf starts with columns that list item padding would take
	lazyLine  bool          // the open line can stay lazy, so it has only the prefixes that it matched
	lazyStart int           // where its content starts in out
	lazyKept  int           // the containers whose prefixes it has
	lazyPad   bool          // its content would start a block after those prefixes
	afterBox  bool          // the last leaf written is a task box
	replace   []byte        // bytes that content writes in place of the next leaf's bytes

	// The last paragraph, heading or table cell that started, the last one
	// that a delimiter check read, and whether its text has '*', '_', '~' and
	// '<'.
	textBlock  markdown.NodeID
	delimScope markdown.NodeID
	delimText  [4]bool

	table *table // the table that prints, or nil

	afters  []blockIndent // the results of indentAfter that a later list can ask for, the last node first
	inlines []inlineFrame // the state of the open inline nodes, the outermost first

	// The code block that is open outside a dialect span: the fence that it
	// writes around its lines, the fence markers of the input, whether the
	// printer is on the input's opening fence line, and whether a line of
	// code has not ended.
	fence             []byte
	fences            int
	opening, lineOpen bool

	// The link reference definition that is open outside a dialect span: the
	// part that prints, whether the space before its destination is written,
	// whether its destination is in angle brackets, and its title quotes.
	def       defPart
	spaced    bool
	defAngle  bool
	defQuotes [2]byte

	// The heading that is open: how it prints, its level, where its content
	// starts in out, and whether its line is written.
	head      headForm
	headLevel int
	headStart int
	headDone  bool

	// The blocks that ended last, for the blank lines before the next block.
	blanks      int           // the input's blank lines after the last leaf block
	open        bool          // a blank line after the last leaf block would be its content
	span        bool          // a dialect span ended after the last block started
	lastLeaf    markdown.Kind // the kind of the last leaf block, or BlankLine after a blank line of a dialect span
	bracket     bool          // the last leaf block is a paragraph that starts with '['
	quoteGap    bool          // the last block quote ends with a paragraph
	quoteParent int           // the index of that block quote's parent in stack
}

// frame is an open structure node. A node holds only what its own kind
// needs; the state of a construct that is open alone, such as a code block or
// a link reference definition, is in the printer, and the state of an inline
// node is in inlines.
type frame struct {
	id        markdown.NodeID
	kind      markdown.Kind
	lastChild markdown.Kind // the kind of the last block that started in it
	span      bool          // a dialect span
	tight     bool          // a list that is not loose, or an item of one
	children  int           // the blocks that started in it
	inline    int32         // the index of an inline node's state in inlines, or -1

	// A list numbers its items from start, and uses its second marker with
	// alt. Its second item decides lazy numbering.
	ordered, alt, lazy bool
	bullet             byte // the bullet of a bullet list
	start              int
	minIndent          int // the columns that the list's items must continue on at least
	indent             int // the columns that a list item of the input continues on

	// The last list that ended in the node.
	listOrdered, listAlt bool
	listBullet           byte

	// A block quote, list item or footnote definition writes marker on its
	// first line and the rest of its marker on its other lines. With
	// blankFirst, its first line has only the marker, as in the input.
	container  bool
	started    bool
	blankFirst bool
	keepBlank  bool // the item keeps its blank marker line
	marker     []byte
}

// rest returns the prefix that container f writes on the lines after its
// first: its marker for a block quote, 4 columns for a footnote definition
// (appendix B, trap 18), and the columns of its marker for a list item.
func (f *frame) rest() []byte {
	switch f.kind {
	case markdown.BlockQuote:
		return quotePrefix
	case markdown.FootnoteDefinition:
		return spaces[:4]
	}
	return spaces[:len(f.marker)]
}

// inlineFrame is the state of an open inline node: the delimiter of emphasis
// or strikethrough, the bytes of a code span, and the part of an inline link
// or image that prints, with the brackets of its text, whether its
// destination is in angle brackets, and its title quotes. A collapsed or
// shortcut reference keeps the bytes of its text, which is its label.
type inlineFrame struct {
	delim    []byte
	code     []byte
	codeDone bool
	tail     tailPart
	brackets int
	label    bool
	angle    bool
	under    bool // emphasis that prints with '_'
	quotes   [2]byte
}

// isInline reports whether a node of kind k has a frame in inlines.
func isInline(k markdown.Kind) bool {
	switch k {
	case markdown.CodeSpan, markdown.Link, markdown.Image, markdown.Emphasis, markdown.Strong,
		markdown.Strikethrough:
		return true
	}
	return false
}

// write appends b to the output. At the output limit it stops, so a printer
// bug cannot build more than max bytes.
func (p *printer) write(b []byte) {
	if p.full || len(p.out)+len(b) > p.max {
		p.full = true
		return
	}
	p.out = append(p.out, b...)
}

// writeLF writes b with each CRLF and CR as LF.
func (p *printer) writeLF(b []byte) {
	for {
		i := bytes.IndexByte(b, '\r')
		if i < 0 {
			p.write(b)
			return
		}
		p.write(b[:i])
		p.write(lineFeed)
		b = b[i+1:]
		if len(b) > 0 && b[0] == '\n' {
			b = b[1:]
		}
	}
}

func (p *printer) writeSpaces(n int) {
	for ; n > len(spaces); n -= len(spaces) {
		p.write(spaces)
	}
	if n > 0 {
		p.write(spaces[:n])
	}
}

// trimSpaces removes the spaces at the end of the output after offset start.
func (p *printer) trimSpaces(start int) {
	if !p.full {
		p.out = p.out[:start+len(bytes.TrimRight(p.out[start:], " "))]
	}
}

// document prints the tree.
func (p *printer) document() {
	t := p.tree
	p.layout, p.spans = t.Layout(), t.DialectSpans()
	var depth int
	p.lazy = p.lazyItems()
	p.breaks, depth = p.prescan()
	p.stack = make([]frame, 0, depth)
	p.lineStart, p.inputLine, p.indent = true, true, -1
	c := t.Walk()
	for e, ok := c.Next(); ok && !p.full; e, ok = c.Next() {
		if e.Exit {
			p.exit()
		} else {
			p.node(e.ID)
		}
	}
}

func (p *printer) node(id markdown.NodeID) {
	t := p.tree
	//exhaustive:enforce
	switch k := t.Kind(id); k {
	case markdown.Document, markdown.FrontMatter, markdown.BlockQuote, markdown.List, markdown.ListItem,
		markdown.Paragraph, markdown.ThematicBreak, markdown.Heading, markdown.CodeBlock, markdown.HTMLBlock,
		markdown.LinkReferenceDefinition, markdown.SoftBreak, markdown.HardBreak, markdown.CodeSpan,
		markdown.Autolink, markdown.RawHTML, markdown.Emphasis, markdown.Strong, markdown.Link, markdown.Image,
		markdown.Strikethrough, markdown.Table, markdown.TableRow, markdown.TableCell,
		markdown.FootnoteDefinition, markdown.FootnoteReference:
		p.layout.Visit(id)
		p.enter(id, k)
	case markdown.Text, markdown.CodeText, markdown.VerbatimLineEnding, markdown.InfoString, markdown.HTMLText,
		markdown.LinkLabel, markdown.Destination, markdown.Title, markdown.FrontMatterText, markdown.Escape,
		markdown.EntityRef, markdown.AutolinkText, markdown.CellPipeEscape, markdown.FootnoteLabel,
		markdown.BOM, markdown.LineEnding, markdown.BlankLine, markdown.Indent, markdown.ThematicRun,
		markdown.ATXMarker, markdown.ATXClose, markdown.Whitespace, markdown.CodeIndent, markdown.FenceMarker,
		markdown.SetextUnderline, markdown.QuoteMarker, markdown.ListMarker, markdown.ItemIndent,
		markdown.Bracket, markdown.Colon, markdown.AngleBracket, markdown.TitleQuote, markdown.FrontMatterFence,
		markdown.TrailingSpace, markdown.HardBreakMarker, markdown.CodeFence, markdown.Delimiter, markdown.Paren,
		markdown.TablePipe, markdown.TableDelimiter, markdown.TaskBox, markdown.FootnoteIndent, markdown.Caret:
		p.lineBegin = p.inputLine
		if p.inputLine {
			p.matched, p.indent = p.layout.Matched(id)
			if p.inSpan > 0 {
				p.indent = -1
			}
		}
		start, end := p.layout.Visit(id)
		raw := t.Raw(id)
		p.inputLine = raw[len(raw)-1] == '\n' || raw[len(raw)-1] == '\r'
		p.leaf(id, k, start, end)
	}
}

func (p *printer) enter(id markdown.NodeID, k markdown.Kind) {
	t := p.tree
	parent := len(p.stack) - 1
	f := frame{id: id, kind: k, span: p.spanAt(id), inline: -1}
	var prev frame
	if parent >= 0 {
		prev = p.stack[parent]
		if isBlock(k) {
			p.separate(parent, id, k, f.span)
		}
	}
	switch k {
	case markdown.BlockQuote:
		f.container, f.marker = true, quotePrefix
		p.lastLeaf = markdown.Document
	case markdown.ListItem:
		f.container, f.tight, f.indent = true, p.stack[parent].tight, p.layout.ItemIndent()
	case markdown.FootnoteDefinition:
		f.container = true
		f.marker = []byte("[^" + string(t.FootnoteDefinitionLabel(id)) + "]: ")
	case markdown.List:
		f.tight = !t.ListLoose(id)
		f.start, f.ordered = t.ListStart(id)
		// Adjacent sibling lists of one type alternate their markers
		// (appendix B, trap 3).
		f.alt = prev.children > 0 && prev.lastChild == markdown.List && prev.listOrdered == f.ordered && !prev.listAlt
		if !f.ordered {
			f.bullet = p.bullet(id, prev)
		}
		// The block after the list must not continue its last item.
		f.minIndent = p.indentAfter(id) + 1
	}
	if isLeafBlock(k) {
		p.written, p.leafKind = false, k
		p.prefixes, p.prefixLen = 0, 0
		for i := range p.stack {
			if p.stack[i].container {
				p.prefixes++
				p.prefixLen += len(p.stack[i].rest())
			}
		}
		p.bracket0 = k == markdown.Paragraph && bytes.HasPrefix(p.firstLine(id), []byte("["))
	}
	if f.span {
		p.inSpan++
	}
	if k == markdown.Paragraph || k == markdown.Heading || k == markdown.TableCell {
		p.textBlock = id
	}
	if k == markdown.Table && p.inSpan == 0 && !p.spanInside(id) && !p.oddSpace(id) {
		// Padding and pipes would change the bytes of a dialect span in a
		// cell, so such a table prints as written.
		p.table = &table{start: len(p.out)}
	}
	if k == markdown.TableCell && p.table != nil {
		if t.CellHeader(id) {
			p.table.aligns = append(p.table.aligns, t.CellAlignment(id))
		}
		p.startTableLine(false)
		p.table.cells = append(p.table.cells, tableCell{start: len(p.out)})
	}
	if k == markdown.CodeBlock {
		p.fences = 0
	}
	if k == markdown.CodeBlock && p.inSpan == 0 {
		p.fence, p.opening = p.codeFence(id)
		// Indented code has no fence line: its first line is code.
		p.lineOpen = !p.opening
		p.lead, p.indent, p.matched = false, -1, p.prefixes
		p.writePrefix(false)
		p.write(p.fence)
		p.lineStart = false
		if !p.opening {
			p.endLine()
		}
	}
	if isInline(k) {
		// The input limit bounds the open inline nodes.
		f.inline = int32(len(p.inlines)) //nolint:gosec // The conversion cannot overflow.
		p.inlines = append(p.inlines, inlineFrame{})
		in := &p.inlines[f.inline]
		if (k == markdown.Link || k == markdown.Image) && (t.LinkForm(id) == markdown.CollapsedReference || t.LinkForm(id) == markdown.ShortcutReference) {
			// The raw text is the label (design 6.7), which Kept keeps.
			in.label = true
			p.inLabel++
		}
		if before, after := t.Around(id); k == markdown.CodeSpan && p.inSpan == 0 && p.inLabel == 0 && before != '$' && after != '$' {
			// A code span next to '$' is GitHub math (appendix B, trap 16).
			in.code = p.codeSpan(id)
		}
		if (k == markdown.Link || k == markdown.Image) && t.LinkForm(id) == markdown.InlineLink && p.inSpan == 0 && p.inLabel == 0 {
			in.tail, in.quotes = tailText, p.titleQuotes(id)
		}
		if (k == markdown.Emphasis || k == markdown.Strong || k == markdown.Strikethrough) && p.inSpan == 0 && p.inLabel == 0 {
			in.delim = p.delimiter(id, k)
		}
		if k == markdown.Strikethrough {
			p.inStrike++
		}
		if k == markdown.Emphasis {
			opener := in.delim
			if opener == nil {
				opener = t.Raw(id + 1)
			}
			if in.under = len(opener) > 0 && opener[0] == '_'; in.under {
				p.inUnder++
			}
		}
	}
	if k == markdown.LinkReferenceDefinition && p.inSpan == 0 {
		p.def, p.spaced, p.defAngle, p.defQuotes = defLabel, false, false, p.titleQuotes(id)
	}
	if k == markdown.Heading && p.inSpan == 0 {
		p.head, p.headLevel, p.headDone = headSingle, t.HeadingLevel(id), false
		if p.multiLine(id) {
			// A setext heading of more than one line stays setext (appendix B,
			// trap 5).
			p.head = headMulti
		}
	}
	p.stack = append(p.stack, f)
}

// spanAt reports whether node id is a dialect span.
func (p *printer) spanAt(id markdown.NodeID) bool {
	for len(p.spans) > 0 && p.spans[0] < id {
		p.spans = p.spans[1:]
	}
	return len(p.spans) > 0 && p.spans[0] == id
}

// separate writes the blank lines before block id of kind k, which starts in
// the frame at index parent. span reports whether the block is a dialect
// span.
func (p *printer) separate(parent int, id markdown.NodeID, k markdown.Kind, span bool) {
	f := &p.stack[parent]
	p.afterText = false
	if f.children > 0 {
		n := 1
		switch {
		case p.open:
			n = 0
		case p.span || span || p.inSpan > 0:
			// Kept syntax (design 12).
			n = p.blanks
		case k == markdown.Table && f.lastChild == markdown.Paragraph && p.blanks == 0 && p.bracket:
			// The table split the paragraph off, which after a blank line
			// would start with a link reference definition.
			n = 0
		case k == markdown.Paragraph && f.lastChild == markdown.LinkReferenceDefinition && p.blanks == 0 &&
			(markdown.InterruptsParagraph(p.firstLine(id), false) || markdown.StartsBlock(p.firstLine(id))):
			// The paragraph continues the definition's lines: after a blank
			// line its first line would start a block. A line that would
			// interrupt the paragraph gets padding.
			n = 0
			if markdown.InterruptsParagraph(p.firstLine(id), false) {
				p.pad = 4
			}
		case k == markdown.Paragraph && p.lazyFirst(id):
			// A blank line would end the lazy line's continuation, and the
			// containers that it does not match would end with it (appendix
			// B, trap 2).
			n = 0
		case (f.kind == markdown.List || f.kind == markdown.ListItem) && f.tight:
			// A blank line would make the list loose.
			n = 0
		case k == markdown.LinkReferenceDefinition && f.lastChild == markdown.LinkReferenceDefinition:
			n = 0
		}
		if n == 0 && p.quoteGap && p.quoteParent == parent && !markdown.InterruptsParagraph(p.firstLine(id), true) {
			// Without a blank line in the block quote, the block would
			// continue the quote's paragraph as a lazy line.
			for i := range p.stack[:parent+1] {
				if p.stack[i].container {
					p.write(p.stack[i].rest())
				}
			}
			p.write([]byte(">\n"))
		}
		for range n {
			p.writePrefix(true)
			p.write(lineFeed)
		}
		p.afterText = n == 0 && (f.lastChild == markdown.Paragraph || f.lastChild == markdown.LinkReferenceDefinition)
	}
	f.children++
	f.lastChild = k
	p.blanks, p.open, p.span, p.quoteGap = 0, false, false, false
}

// lazyFirst reports whether the first line of block id is lazy: it matches
// fewer containers than are open, so a blank line before it would take the
// block out of the containers that its line does not match (appendix B,
// trap 2).
func (p *printer) lazyFirst(id markdown.NodeID) bool {
	t := p.tree
	leaf := id + 1
	for !t.Kind(leaf).Leaf() {
		leaf++
	}
	// The prefix leaves of the line come before the block.
	for leaf > 1 {
		prev := leaf - 1
		for prev > 1 && !t.Kind(prev).Leaf() {
			prev--
		}
		if raw := t.Raw(prev); !t.Kind(prev).Leaf() || raw[len(raw)-1] == '\n' || raw[len(raw)-1] == '\r' {
			break
		}
		leaf = prev
	}
	matched, _ := p.layout.Matched(leaf)
	open := 0
	for i := range p.stack {
		if p.stack[i].container {
			open++
		}
	}
	return matched < open
}

// firstLine returns the first line of block id from its first leaf after its
// indentation, without trailing spaces and tabs.
func (p *printer) firstLine(id markdown.NodeID) []byte {
	t := p.tree
	i := id + 1
	for !t.Kind(i).Leaf() || t.Kind(i) == markdown.Indent {
		i++
	}
	return bytes.Trim(t.RestOfLine(i), " \t")
}

// writePrefix writes the prefixes of the open containers at the start of a
// line: the marker of a container on its first line, and the rest of each
// container that the input line matched, so that a lazy line stays lazy. A
// blank line gets the prefixes without trailing spaces.
func (p *printer) writePrefix(blank bool) {
	start, n, opened := len(p.out), 0, 0
prefixes:
	for i := range p.stack {
		f := &p.stack[i]
		if !f.container {
			continue
		}
		switch {
		case !f.started && blank:
			break prefixes
		case !f.started:
			if opened >= 99 && f.kind != markdown.BlockQuote && p.inSpan == 0 {
				// GitHub starts no list item or footnote definition after 99
				// blocks on a line (appendix B, trap 21).
				p.nextPrefixLine(start, i)
				start, opened = len(p.out), 0
			}
			p.write(f.marker)
			f.started = true
			opened++
			// Padding would take the columns that the content starts with, or
			// the item keeps its blank marker line.
			if f.blankFirst && f.kind != markdown.BlockQuote && (p.lead || f.keepBlank) {
				p.nextPrefixLine(start, i+1)
				start, opened = len(p.out), 0
			}
		case blank || n < p.matched:
			p.write(f.rest())
		default:
			break prefixes
		}
		n++
	}
	if blank {
		p.trimSpaces(start)
	}
}

// nextPrefixLine ends the line of prefixes that starts at offset start, and
// writes the rest of each container in the first end frames of the stack.
func (p *printer) nextPrefixLine(start, end int) {
	if p.full {
		return
	}
	p.trimSpaces(start)
	p.write(lineFeed)
	for j := range p.stack[:end] {
		if p.stack[j].container {
			p.write(p.stack[j].rest())
		}
	}
}

func (p *printer) leaf(id markdown.NodeID, k markdown.Kind, start, end int) {
	t := p.tree
	top := &p.stack[len(p.stack)-1]
	var in *inlineFrame
	if top.inline >= 0 {
		in = &p.inlines[top.inline]
	}
	code := top.kind == markdown.CodeBlock && p.fence != nil
	if code && p.lineBegin && !p.opening {
		p.lineOpen = true
	}
	switch {
	case k == markdown.ListMarker:
		p.listMarker(top, id, start)
		if p.indent >= 0 {
			// The marker writes the columns of the input's marker and
			// padding, also after a footnote definition starts on the line.
			p.indent = max(p.indent, end)
		}
	case k == markdown.BlankLine:
		switch {
		case top.container && top.children == 0 && !top.blankFirst:
			// The first blank line of a container is the rest of its marker
			// line, or a blank line at its start.
			top.blankFirst = true
		case p.inSpan > 0 || p.span:
			// A blank line in or after a dialect span is kept syntax (design
			// 12), with the prefixes of the containers that its line matched:
			// the next block can be in fewer containers than it is.
			start := len(p.out)
			p.writePrefix(false)
			p.trimSpaces(start)
			p.write(lineFeed)
			// The blank line ends the paragraph of a block quote that it
			// follows, so no gap line goes before the next block.
			p.lastLeaf, p.quoteGap = markdown.BlankLine, false
		default:
			p.blanks++
		}
	case k == markdown.QuoteMarker, k == markdown.ItemIndent, k == markdown.FootnoteIndent:
		// A container that starts on the line after its first leaf.
		if p.indent >= 0 {
			p.indent = max(p.indent, end)
		}
	case k == markdown.BOM, top.kind == markdown.Document, top.kind == markdown.List:
		// The printer writes container syntax from its stack.
	case top.container:
		// The start of a footnote definition.
		p.indent = end
		if (k == markdown.LineEnding || k == markdown.VerbatimLineEnding) && top.children == 0 {
			top.blankFirst = true
		}
	case p.table != nil && (top.kind == markdown.Table || top.kind == markdown.TableRow):
		// The printer writes the pipes, the spaces and the delimiter row.
		if k == markdown.LineEnding {
			p.lineStart = true
		} else {
			p.startTableLine(top.kind == markdown.Table)
		}
	case p.table != nil && top.kind == markdown.TableCell && k == markdown.Whitespace:
	case code:
		p.codeLeaf(id, k, start)
	case in != nil && in.code != nil:
		// The code span prints whole at its first leaf.
		if !in.codeDone {
			in.codeDone = true
			p.replace = in.code
			p.content(id, start)
		}
	case in != nil && in.tail != tailNone:
		p.tailLeaf(in, id, k, start)
	case in != nil && in.delim != nil && k == markdown.Delimiter:
		p.replace = in.delim
		p.content(id, start)
	case p.def != defNone && top.kind == markdown.LinkReferenceDefinition:
		p.definitionLeaf(id, k, start)
	case top.kind == markdown.FrontMatter && k == markdown.Whitespace:
		// Spaces after a fence line are not meaning.
	case p.head == headSingle && p.headDone:
		// The underline line of a setext heading that prints as ATX.
	case p.head != headNone && top.kind == markdown.Heading && (k == markdown.ATXMarker || k == markdown.ATXClose || k == markdown.Whitespace):
		// The printer writes the markers from the level.
	case p.head == headSingle && k == markdown.LineEnding:
		p.endHeading()
		p.endLine()
	case p.head == headMulti && k == markdown.SetextUnderline:
		p.indent = -1
		p.replace = []byte("===")
		if p.headLevel == 2 {
			p.replace = []byte("---")
		}
		p.content(id, start)
	case k == markdown.LineEnding, k == markdown.VerbatimLineEnding:
		p.endLine()
	case k == markdown.Indent && (top.kind == markdown.Paragraph || top.kind == markdown.Heading ||
		top.kind == markdown.ThematicBreak || top.kind == markdown.TableRow) && !p.written && p.inSpan == 0:
		// Indentation before a paragraph, a heading, a thematic break or the
		// first row of a table is not meaning. A table that prints as written
		// would start indented code with it.
		p.indent = -1
	case (k == markdown.Indent || k == markdown.CodeIndent) && p.inSpan == 0:
		// Indentation is its columns, whatever tabs it holds (design 4.3):
		// content writes them.
	case k == markdown.Whitespace && p.afterBox && p.inSpan == 0:
		p.write(spaces[:1])
	case k == markdown.TrailingSpace && p.inSpan == 0 && p.inLabel == 0:
		// After a backslash that is not an escape, the line ending would
		// make a hard break (spec 6.7). The content of a heading that prints
		// as ATX ends on its line, where no break forms, and the heading
		// drops the space when it is read again (design 12).
		if p.backslash && p.head != headSingle {
			p.write(spaces[:1])
		}
	case k == markdown.HardBreakMarker && p.inSpan == 0 && p.inLabel == 0 && t.Raw(id)[0] != '\\':
		// A hard break is a backslash, except after a backslash that is not
		// an escape, which the backslash would escape (appendix B, trap 10),
		// and in a paragraph that starts with '[', where the backslash could
		// be the destination of a link reference definition. After a
		// character of a delimiter run, the break keeps its input form: a
		// backslash is punctuation and a line ending is whitespace, so the
		// form decides whether the run flanks. After '$', it keeps its form
		// too (appendix B, trap 16).
		last := byte(0)
		if len(p.out) > 0 {
			last = p.out[len(p.out)-1]
		}
		switch {
		case last == '*' || last == '_' || last == '~' || last == '$':
			if t.Raw(id)[0] == '\\' {
				p.write([]byte{'\\'})
			} else {
				p.write(spaces[:2])
			}
		case p.backslash || p.bracket0:
			p.write(spaces[:2])
		default:
			p.write([]byte{'\\'})
		}
	case len(p.out) == 0 && len(p.stack) == 2 && k == markdown.Text && p.inSpan == 0 &&
		(p.head != headSingle || p.headDone) &&
		string(bytes.TrimRight(t.RestOfLine(id), " \t\r\n")) == "+++":
		// The first block never looks like front matter (appendix B, trap 7).
		// A heading that prints as ATX writes its markers first, so its line
		// never starts with the fence.
		p.indent = -1
		p.write(spaces[:1])
		p.content(id, start)
	default:
		if k == markdown.FenceMarker {
			p.fences++
		}
		p.content(id, start)
	}
}

// listMarker sets the marker and the rest of list item f, whose ListMarker
// leaf id starts at column start: '-' or '*' in a bullet list, and in an
// ordered list the item's number and '.' or ')' (roadmap Decisions), with
// the padding that the list's minimum indentation needs.
func (p *printer) listMarker(f *frame, id markdown.NodeID, start int) {
	list := &p.stack[len(p.stack)-2]
	i := list.children - 1
	switch {
	case !list.ordered:
		f.marker = []byte{list.bullet}
	default:
		if i == 1 && list.start == 1 {
			raw := bytes.TrimLeft(p.tree.Raw(id), " \t")
			digits := raw[:len(raw)-len(bytes.TrimLeft(raw, "0123456789"))]
			n, _ := strconv.Atoi(string(digits))
			list.lazy = n == 1
		}
		n := list.start + i
		if list.lazy {
			n = 1
		}
		// A marker has at most 9 digits (appendix B, trap 11).
		delim := byte('.')
		if list.alt {
			delim = ')'
		}
		f.marker = append(strconv.AppendInt(nil, int64(min(n, 999999999)), 10), delim)
	}
	minIndent := list.minIndent
	if p.lazy[f.id] {
		// A lazy line of a dialect span keeps its indentation, so the item
		// continues on its input columns at least.
		minIndent = max(minIndent, f.indent)
	}
	padding := max(minIndent-len(f.marker), 1)
	// The rest of a blank marker line is a BlankLine leaf.
	end, _ := p.tree.Next(f.id)
	blank := id+1 == end || p.tree.Kind(id+1) == markdown.BlankLine
	if padding > 4 || padding > 1 && blank {
		// No padding lets the item continue on so many columns, and an item
		// whose marker line is blank continues after one column of padding
		// (spec 5.2): the item keeps its indentation, marker and padding. Its
		// marker line stays blank, so that a second format reads the same
		// item and keeps the same marker (design 12).
		f.marker, f.keepBlank = p.sourceMarker(f, id, start), blank
	} else {
		f.marker = append(f.marker, spaces[:padding]...)
	}
}

// bullet returns the bullet of bullet list id, whose parent frame is prev:
// '-', or '*' when '-' does not work, or '+'. A bullet does not work after an
// adjacent sibling list with that bullet (appendix B, trap 3). It does not
// work on the marker line of an item with that bullet, or when an item's
// first line is only that character with spaces: "- - -" and "- --" are
// thematic breaks.
func (p *printer) bullet(id markdown.NodeID, prev frame) byte {
	var adjacent, line byte
	if prev.children > 0 && prev.lastChild == markdown.List && !prev.listOrdered {
		adjacent = prev.listBullet
	}
	if prev.kind == markdown.ListItem && !prev.started && len(prev.marker) > 0 {
		line = prev.marker[0]
	}
	breaks := p.breaks[id]
	switch {
	case adjacent != '-' && line != '-' && !breaks[0]:
		return '-'
	case adjacent != '*' && line != '*' && !breaks[1]:
		return '*'
	}
	return '+'
}

// prescan walks the tree once, before printing. It returns the lists with an
// item whose first line is at least two '-' and spaces only, with true in the
// first element, and with such a line of '*', with true in the second; and
// the greatest number of structure nodes open at once, which sizes the
// printer's stack. The items of lists that start on one line share their
// first line, so one walk finds it for all of them.
func (p *printer) prescan() (map[markdown.NodeID][2]bool, int) {
	t := p.tree
	breaks := map[markdown.NodeID][2]bool{}
	open, depth := 0, 0
	var lists []markdown.NodeID    // the open lists
	var items [][2]markdown.NodeID // the open items before their first line, with their lists
	c := t.Walk()
	for e, ok := c.Next(); ok; e, ok = c.Next() {
		k := t.Kind(e.ID)
		switch {
		case e.Exit:
			open--
		case !k.Leaf():
			open++
			depth = max(depth, open)
		}
		switch {
		case e.Exit && k == markdown.List:
			lists = lists[:len(lists)-1]
		case e.Exit:
			if n := len(items); n > 0 && items[n-1][0] == e.ID {
				items = items[:n-1]
			}
		case k == markdown.List:
			lists = append(lists, e.ID)
		case k == markdown.ListItem:
			items = append(items, [2]markdown.NodeID{e.ID, lists[len(lists)-1]})
		case len(items) == 0 || !k.Leaf() || isPrefix(k) || k == markdown.Indent || k == markdown.BlankLine:
		case k == markdown.ThematicRun:
			// The printer writes a thematic break without the bullet.
			items = items[:0]
		default:
			line := bytes.Trim(t.RestOfLine(e.ID), " \t")
			dashes := len(bytes.Trim(line, "- \t")) == 0 && bytes.Count(line, []byte("-")) >= 2
			stars := len(bytes.Trim(line, "* \t")) == 0 && bytes.Count(line, []byte("*")) >= 2
			for _, item := range items {
				if k == markdown.CodeIndent && !p.isSpan(item[0]) && !p.isSpan(e.ID-1) {
					// The first line of indented code outside a dialect span is
					// a fence line.
					continue
				}
				if dashes || stars {
					b := breaks[item[1]]
					breaks[item[1]] = [2]bool{b[0] || dashes, b[1] || stars}
				}
			}
			items = items[:0]
		}
	}
	return breaks, depth
}

// sourceMarker returns the indentation, marker and padding of list item f of
// the input, whose ListMarker leaf id starts at column start.
func (p *printer) sourceMarker(f *frame, id markdown.NodeID, start int) []byte {
	raw := p.tree.Raw(id)
	lead, i := start, 0
	if p.tree.SplitTab(id) > 0 {
		lead, i = start+p.tree.SplitTab(id), 1
	}
	for ; i < len(raw) && (raw[i] == ' ' || raw[i] == '\t'); i++ {
		if raw[i] == '\t' {
			lead += 4 - lead%4
		} else {
			lead++
		}
	}
	chars := bytes.TrimRight(raw[i:], " \t")
	return append(append(bytes.Clone(spaces[:lead-start]), chars...), spaces[:max(f.indent-(lead-start)-len(chars), 1)]...)
}

// lazyItems returns the list items that are the first container that a line
// of a dialect span does not match.
func (p *printer) lazyItems() map[markdown.NodeID]bool {
	t := p.tree
	if len(p.spans) == 0 {
		return nil
	}
	items := map[markdown.NodeID]bool{}
	layout, spans := t.Layout(), p.spans
	var open, inSpan []markdown.NodeID
	lineStart := true
	c := t.Walk()
	for e, ok := c.Next(); ok; e, ok = c.Next() {
		k := t.Kind(e.ID)
		switch {
		case e.Exit:
			if len(open) > 0 && open[len(open)-1] == e.ID {
				open = open[:len(open)-1]
			}
			if len(inSpan) > 0 && inSpan[len(inSpan)-1] == e.ID {
				inSpan = inSpan[:len(inSpan)-1]
			}
			continue
		case k.Leaf():
			if lineStart && len(inSpan) > 0 {
				if matched, _ := layout.Matched(e.ID); matched < len(open) && t.Kind(open[matched]) == markdown.ListItem {
					items[open[matched]] = true
				}
			}
			layout.Visit(e.ID)
			raw := t.Raw(e.ID)
			lineStart = raw[len(raw)-1] == '\n' || raw[len(raw)-1] == '\r'
			continue
		case k == markdown.BlockQuote || k == markdown.ListItem || k == markdown.FootnoteDefinition:
			open = append(open, e.ID)
		}
		layout.Visit(e.ID)
		for len(spans) > 0 && spans[0] < e.ID {
			spans = spans[1:]
		}
		if len(spans) > 0 && spans[0] == e.ID {
			inSpan = append(inSpan, e.ID)
		}
	}
	return items
}

// isSpan reports whether node id, which the walk has not passed, is a
// dialect span.
func (p *printer) isSpan(id markdown.NodeID) bool {
	_, ok := slices.BinarySearch(p.spans, id)
	return ok
}

// oddSpace reports whether a whitespace leaf of table id holds a byte that is
// not a space or a tab. A canonical table writes its own spaces, so it would
// drop a vertical tab or a form feed, which GitHub reads as a character of a
// label or a destination (dialect.md).
func (p *printer) oddSpace(id markdown.NodeID) bool {
	t := p.tree
	end, _ := t.Next(id)
	for i := id + 1; i < end; i++ {
		if t.Kind(i) == markdown.Whitespace && len(bytes.Trim(t.Raw(i), " \t")) > 0 {
			return true
		}
	}
	return false
}

// spanInside reports whether a dialect span is inside node id, which the walk
// has not passed.
func (p *printer) spanInside(id markdown.NodeID) bool {
	end, _ := p.tree.Next(id)
	i, _ := slices.BinarySearch(p.spans, id)
	return i < len(p.spans) && p.spans[i] < end
}

// indentAfter returns the columns of indentation of the first line of the
// block after node id, when that block is an HTML block, a dialect span, or a
// list whose first item keeps the columns of its input marker, all of which
// keep their indentation. Otherwise it returns 0. Nested lists that end
// together share the block after them, so a result stays in p.afters while a
// later list can ask for it.
func (p *printer) indentAfter(id markdown.NodeID) int {
	next, ok := p.tree.Next(id)
	for n := len(p.afters); n > 0 && p.afters[n-1].next < next; n-- {
		p.afters = p.afters[:n-1]
	}
	if n := len(p.afters); n > 0 && p.afters[n-1].next == next {
		return p.afters[n-1].cols
	}
	cols := p.indentAt(next, ok)
	p.afters = append(p.afters, blockIndent{next, cols})
	return cols
}

// blockIndent is a result of [printer.indentAfter] for the node after a list.
type blockIndent struct {
	next markdown.NodeID
	cols int
}

// indentAt returns what [printer.indentAfter] returns for the node next, which
// ok reports is in the tree.
func (p *printer) indentAt(next markdown.NodeID, ok bool) int {
	t := p.tree
	for ok && (t.Kind(next) == markdown.BlankLine || isPrefix(t.Kind(next))) {
		next, ok = t.Next(next)
	}
	// A list item whose dialect span has a lazy line keeps the columns of its
	// input marker, so the list before it must not take its line.
	lazy := ok && t.Kind(next) == markdown.List && p.lazy[next+1]
	if !ok || t.Kind(next) != markdown.HTMLBlock && !lazy && !p.isSpan(next) {
		return 0
	}
	leaf := next + 1
	cols := t.SplitTab(leaf)
	for _, c := range t.RestOfLine(leaf)[min(cols, 1):] {
		switch c {
		case ' ':
			cols++
		case '\t':
			cols += 4 - cols%4
		default:
			return cols
		}
	}
	return cols
}

// content writes leaf id, whose own columns start at input column start,
// after the line's prefix and the columns between the prefix and the leaf.
func (p *printer) content(id markdown.NodeID, start int) {
	t := p.tree
	k, b := t.Kind(id), t.Raw(id)
	if p.replace != nil {
		b, p.replace = p.replace, nil
	}
	if k == markdown.ThematicRun && p.inSpan == 0 {
		b = p.thematicRun()
	}
	if p.lineStart {
		if p.written && p.inSpan == 0 && (p.leafKind == markdown.Paragraph || p.leafKind == markdown.Heading) &&
			k != markdown.SetextUnderline {
			p.continuation(id)
		}
		p.lead = p.pad > 0 || p.indent >= 0 && start > p.indent || t.SplitTab(id) > 0 || b[0] == ' ' || b[0] == '\t'
		p.writePrefix(false)
		p.lineStart = false
		p.lazyStart, p.lazyKept = len(p.out), p.matched
		p.writeSpaces(p.pad)
		p.pad = 0
		if p.head == headSingle && !p.headDone {
			// The columns of the input's markers and spaces are not written.
			p.indent = -1
			p.write(append(bytes.Repeat([]byte{'#'}, p.headLevel), ' '))
			p.headStart = len(p.out)
		}
	}
	if p.indent >= 0 {
		p.writeSpaces(start - p.indent)
		p.indent = -1
	}
	if v := t.SplitTab(id); v > 0 {
		p.writeSpaces(v)
		b = b[1:]
	}
	if k == markdown.TaskBox && p.inSpan == 0 {
		b = []byte("[ ]")
		if t.Raw(id)[1]|0x20 == 'x' {
			b = []byte("[x]")
		}
	}
	p.writeLF(b)
	p.written, p.afterBox = true, k == markdown.TaskBox
	p.backslash = len(b) > 0 && b[len(b)-1] == '\\' && k != markdown.Escape
}

// defPart is the part of a link reference definition that prints.
type defPart uint8

const (
	defNone        defPart = iota // no definition prints, or it is in a dialect span
	defLabel                      // the label, up to the colon
	defDestination                // the destination
	defTitle                      // after the destination, before a title
	defInTitle                    // the title
	defEnd                        // after the title
)

// titleQuotes returns the quotes around the title of link reference
// definition id: '"', or '\” when the title has '"', or parentheses when it
// has both, or the input's quotes when it also has a parenthesis. A title
// decodes the same in each (spec 4.7).
func (p *printer) titleQuotes(id markdown.NodeID) [2]byte {
	t := p.tree
	end, _ := t.Next(id)
	var title []byte
	var source [2]byte
	quotes := 0
	for i := id + 1; i < end; i++ {
		if !t.Kind(i).Leaf() {
			// A link in an image's text has its own title.
			next, _ := t.Next(i)
			i = next - 1
			continue
		}
		switch t.Kind(i) {
		case markdown.Title:
			title = append(title, t.Raw(i)...)
		case markdown.TitleQuote:
			if quotes < 2 {
				source[quotes] = t.Raw(i)[0]
				quotes++
			}
		}
	}
	switch {
	case bytes.IndexByte(title, '"') < 0:
		return [2]byte{'"', '"'}
	case bytes.IndexByte(title, '\'') < 0:
		return [2]byte{'\'', '\''}
	case !bytes.ContainsAny(title, "()"):
		return [2]byte{'(', ')'}
	}
	return source
}

// definitionLeaf prints leaf id of kind k, whose own columns start at column
// start, in the open link reference definition: the label as written, one
// space, the destination as written, and one space and the title in its
// quotes. The whitespace and line endings between them are not written.
func (p *printer) definitionLeaf(id markdown.NodeID, k markdown.Kind, start int) {
	switch p.def {
	case defNone:
	case defLabel:
		switch k {
		case markdown.Indent:
			p.indent = -1
		case markdown.VerbatimLineEnding:
			p.endLine()
		default:
			p.content(id, start)
			if k == markdown.Colon {
				p.def = defDestination
			}
		}
	case defDestination:
		if k == markdown.Whitespace || k == markdown.LineEnding || k == markdown.Indent {
			return
		}
		if !p.spaced {
			p.write(spaces[:1])
			p.spaced = true
		}
		p.indent = -1
		p.content(id, start)
		switch {
		case k == markdown.AngleBracket && !p.defAngle:
			p.defAngle = true
		case k == markdown.AngleBracket, k == markdown.Destination && !p.defAngle:
			p.def = defTitle
		}
	case defTitle:
		if k == markdown.TitleQuote {
			p.write(spaces[:1])
			p.indent, p.replace = -1, p.defQuotes[:1]
			p.content(id, start)
			p.def = defInTitle
		}
	case defInTitle:
		switch k {
		case markdown.VerbatimLineEnding:
			p.endLine()
		case markdown.TitleQuote:
			p.indent, p.replace = -1, p.defQuotes[1:]
			p.content(id, start)
			p.def = defEnd
		default:
			p.indent = -1
			p.content(id, start)
		}
	case defEnd:
	}
}

// delimiter returns the delimiter of node id of kind k, which is emphasis,
// strong emphasis or strikethrough: '_', "**" or "~~" (roadmap Decisions),
// or nil to keep the input's. A node keeps its input delimiters wherever the
// canonical delimiters could pair differently (spec 6.2), or could change a
// construct that starts next to them. Each check below gives its reason.
func (p *printer) delimiter(id markdown.NodeID, k markdown.Kind) []byte {
	t := p.tree
	raw, open := t.Raw(id), t.Raw(id+1)
	var closer []byte
	end, _ := t.Next(id)
	for i := end - 1; i > id; i-- {
		if t.Kind(i) == markdown.Delimiter {
			closer = t.Raw(i)
			break
		}
	}
	content := raw[len(open) : len(raw)-len(closer)]
	if t.Kind(id+2) == markdown.Autolink || len(content) >= 4 && bytes.EqualFold(content[:4], []byte("www.")) {
		// An extended www autolink depends on the byte before it (design 6.2).
		return nil
	}
	if p.inAutolinkWord(id) || autolinkWord(lastWord(content)) {
		// An underscore in the last two segments of a domain keeps an
		// extended autolink from forming (design 6.2), so '*' would make one,
		// of the word before the node or of the word that its content ends
		// with.
		return nil
	}
	before, after := t.Around(id)
	if before == '$' || after == '$' || len(content) > 0 && (content[0] == '$' || content[len(content)-1] == '$') {
		// No character next to '$' changes (appendix B, trap 16).
		return nil
	}
	switch k {
	case markdown.Emphasis:
		// After a '<' in text, '_' could start an attribute name of a tag.
		if bytes.ContainsAny(content, "*_") || !flanksLikeSpace(before) || !flanksLikeSpace(after) || p.inText('_') || p.inText('<') {
			return nil
		}
		if p.inUnder > 0 && (!spaceLike(before) || !spaceLike(after)) {
			// Inside emphasis that prints with '_', a '_' run that
			// punctuation flanks can open and close, so it could pair with
			// the runs around it (spec 6.2).
			return nil
		}
		return []byte{'_'}
	case markdown.Strong:
		// Next to '*' or '_', which can be the unused part of a delimiter run
		// (design 6.4), "**" could pair differently.
		if bytes.ContainsAny(content, "*_") || before == '*' || before == '_' || after == '*' || after == '_' || p.inText('*') {
			return nil
		}
		return []byte("**")
	default:
		// No '~' goes next to a "~~" delimiter (appendix B, trap 13), and
		// "~~" inside a strikethrough could close it.
		if bytes.IndexByte(content, '~') >= 0 || before == '~' || after == '~' || p.inText('~') || p.inStrike > 0 {
			return nil
		}
		return []byte("~~")
	}
}

// inAutolinkWord reports whether the word before node id holds the start of
// an extended autolink. The bytes of the node follow that word without a
// space, so they can be part of it.
func (p *printer) inAutolinkWord(id markdown.NodeID) bool {
	t := p.tree
	var word []byte
	for i := id - 1; i > p.textBlock && t.Kind(i).Leaf(); i-- {
		b := t.Raw(i)
		if j := bytes.LastIndexAny(b, " \t\n\r"); j >= 0 {
			word = append(bytes.Clone(b[j+1:]), word...)
			break
		}
		word = append(bytes.Clone(b), word...)
	}
	return autolinkWord(word)
}

// lastWord returns the bytes of b after its last whitespace.
func lastWord(b []byte) []byte {
	return b[bytes.LastIndexAny(b, " \t\n\r")+1:]
}

// autolinkWord reports whether word holds "://" or "www.", the start of an
// extended autolink (design 6.2).
func autolinkWord(word []byte) bool {
	return bytes.Contains(word, []byte("://")) || bytes.Contains(bytes.ToLower(word), []byte("www."))
}

// inText reports whether the last paragraph, heading or table cell that
// started has a text leaf with c, which is '*', '_', '~' or '<'. A canonical
// delimiter could pair with a run of c that is text, where the input's
// delimiter does not, and '_' next to a '<' could start an attribute name of
// a tag.
func (p *printer) inText(c byte) bool {
	t := p.tree
	if scope := p.textBlock; scope != p.delimScope {
		p.delimScope, p.delimText = scope, [4]bool{}
		end, _ := t.Next(scope)
		for j := scope + 1; j < end; j++ {
			if t.Kind(j) == markdown.Text {
				for k, d := range []byte("*_~<") {
					p.delimText[k] = p.delimText[k] || bytes.IndexByte(t.Raw(j), d) >= 0
				}
			}
		}
	}
	return p.delimText[strings.IndexByte("*_~<", c)]
}

// spaceLike reports whether c, the byte next to an emphasis delimiter, is
// the start or the end of the input, a space, a tab or a line ending.
func spaceLike(c byte) bool {
	switch c {
	case 0, ' ', '\t', '\n', '\r':
		return true
	}
	return false
}

// flanksLikeSpace reports whether c, the byte next to an emphasis
// delimiter, lets '_' flank as '*' does: 0 for the start or the end of the
// input, a space, a tab, a line ending, or ASCII punctuation other than '*',
// '_' and '\\'.
func flanksLikeSpace(c byte) bool {
	switch c {
	case 0, ' ', '\t', '\n', '\r':
		return true
	case '*', '_', '\\':
		return false
	}
	return strings.IndexByte("!\"#$%&'()+,-./:;<=>?@[]^`{|}~", c) >= 0
}

// codeSpan returns the bytes of code span id in the canonical style, or nil
// to keep its input bytes when it spans lines or holds a cell pipe escape.
// The fence is the shortest run of backticks that its value does not hold,
// with a space inside each end when the value starts or ends with a
// backtick, or starts and ends with a space and is not only spaces (spec 6.1,
// appendix B, trap 9).
func (p *printer) codeSpan(id markdown.NodeID) []byte {
	t := p.tree
	end, _ := t.Next(id)
	var value []byte
	for i := id + 1; i < end; i++ {
		switch t.Kind(i) {
		case markdown.CodeText:
			value = append(value, t.Raw(i)...)
		case markdown.CodeFence:
		default:
			return nil
		}
	}
	allSpaces := len(bytes.Trim(value, " ")) == 0
	if len(value) >= 2 && value[0] == ' ' && value[len(value)-1] == ' ' && !allSpaces {
		value = value[1 : len(value)-1]
	}
	runs := make(map[int]bool)
	for run, i := 0, 0; i <= len(value); i++ {
		if i < len(value) && value[i] == '`' {
			run++
			continue
		}
		runs[run] = true
		run = 0
	}
	n := 1
	for runs[n] {
		n++
	}
	fence := bytes.Repeat([]byte{'`'}, n)
	out := append([]byte(nil), fence...)
	pad := len(value) > 0 && (value[0] == '`' || value[len(value)-1] == '`' ||
		value[0] == ' ' && value[len(value)-1] == ' ' && len(bytes.Trim(value, " ")) > 0)
	if pad {
		out = append(out, ' ')
	}
	out = append(out, value...)
	if pad {
		out = append(out, ' ')
	}
	return append(out, fence...)
}

// tailPart is the part of an inline link or image that prints.
type tailPart uint8

const (
	tailNone        tailPart = iota // no inline link prints, or it keeps its bytes
	tailText                        // the link text, up to its closing bracket
	tailOpen                        // the opening parenthesis
	tailDestination                 // before and in the destination
	tailTitle                       // after the destination, before a title
	tailInTitle                     // the title
	tailEnd                         // after the title
)

// tailLeaf prints leaf id of kind k, whose own columns start at column
// start, that is a child of the inline link or image in: its text, and a
// tail of the destination as written and one space and the title in its
// quotes. The whitespace, indentation and line endings between them are not
// written.
func (p *printer) tailLeaf(in *inlineFrame, id markdown.NodeID, k markdown.Kind, start int) {
	skip := k == markdown.Whitespace || k == markdown.LineEnding || k == markdown.Indent
	switch in.tail {
	case tailNone:
	case tailText:
		p.content(id, start)
		if k == markdown.Bracket {
			in.brackets++
			if in.brackets == 2 {
				in.tail = tailOpen
			}
		}
	case tailOpen:
		p.content(id, start)
		in.tail = tailDestination
	case tailDestination:
		switch {
		case skip:
		case k == markdown.Paren:
			p.indent = -1
			p.content(id, start)
			in.tail = tailEnd
		default:
			p.indent = -1
			p.content(id, start)
			if k == markdown.AngleBracket && !in.angle {
				in.angle = true
			} else if k == markdown.Destination && !in.angle || k == markdown.AngleBracket {
				in.tail = tailTitle
			}
		}
	case tailTitle:
		switch {
		case skip:
		case k == markdown.TitleQuote:
			p.write(spaces[:1])
			p.indent, p.replace = -1, in.quotes[:1]
			p.content(id, start)
			in.tail = tailInTitle
		default:
			// After a backslash that is not an escape, the parenthesis would
			// be an escape.
			if k == markdown.Paren && p.backslash {
				p.write(spaces[:1])
			}
			p.indent = -1
			p.content(id, start)
			in.tail = tailEnd
		}
	case tailInTitle:
		switch k {
		case markdown.VerbatimLineEnding:
			p.endLine()
		case markdown.TitleQuote:
			p.indent, p.replace = -1, in.quotes[1:]
			p.content(id, start)
			in.tail = tailEnd
		default:
			p.indent = -1
			p.content(id, start)
		}
	case tailEnd:
		if !skip {
			p.indent = -1
			p.content(id, start)
		}
	}
}

// codeFence returns the fence of code block id: backticks, or tildes when its
// info string has a backtick, one more than the longest run of that
// character in the code and at least 3 (roadmap Decisions). It also reports
// whether the input's code block has a fence line.
func (p *printer) codeFence(id markdown.NodeID) ([]byte, bool) {
	t := p.tree
	end, _ := t.Next(id)
	char, fenced := byte('`'), false
	for i := id + 1; i < end; i++ {
		switch t.Kind(i) {
		case markdown.FenceMarker:
			fenced = true
		case markdown.InfoString:
			if bytes.IndexByte(t.Raw(i), '`') >= 0 {
				char = '~'
			}
		}
	}
	longest := 2
	for i := id + 1; i < end; i++ {
		if t.Kind(i) != markdown.CodeText {
			continue
		}
		run := 0
		for _, c := range t.Raw(i) {
			run++
			if c != char {
				run = 0
			}
			longest = max(longest, run)
		}
	}
	return bytes.Repeat([]byte{char}, longest+1), fenced
}

// codeLeaf prints leaf id of kind k, whose own columns start at column
// start, in the open code block: the info string of the opening fence line,
// and the lines of code as their value.
func (p *printer) codeLeaf(id markdown.NodeID, k markdown.Kind, start int) {
	switch {
	case p.opening && k == markdown.InfoString:
		p.indent = -1
		if p.tree.Raw(id)[0] == p.fence[0] {
			// Without a space, the fence would take the info string's first
			// characters.
			p.write(spaces[:1])
		}
		p.content(id, start)
	case p.opening && k == markdown.LineEnding:
		p.opening = false
		p.endLine()
	case p.opening:
	case k == markdown.CodeText:
		// The code's value leaves out the input's indentation.
		p.indent = -1
		p.content(id, start)
	case k == markdown.VerbatimLineEnding:
		p.lineOpen = false
		p.endLine()
	case k == markdown.FenceMarker:
		// The input's closing fence line.
		p.lineOpen = false
	}
}

// thematicRun returns the thematic break to write before the prefix of its
// line: "---", or "***" where "---" would be a setext underline after a
// paragraph line or text after a definition line, the opener of front matter
// (appendix B, trap 7), or a longer break with the bullet of the list item
// whose marker line it is on (trap 4).
func (p *printer) thematicRun() []byte {
	if p.afterText || len(p.out) == 0 && len(p.stack) == 2 {
		return []byte("***")
	}
	for i := len(p.stack) - 1; i >= 0; i-- {
		if f := &p.stack[i]; f.container {
			if f.kind == markdown.ListItem && !f.started && f.marker[0] == '-' {
				return []byte("***")
			}
			break
		}
	}
	return []byte("---")
}

// continuation sets the prefix and the indentation of a paragraph line whose
// first written leaf is id. The line gets the prefixes of every open
// container, unless it is lazy in the input, those prefixes are longer than
// the line (appendix B, trap 2), and it starts no block as a lazy line. It
// gets 4 columns of indentation when its content would otherwise start a
// block (trap 1).
func (p *printer) continuation(id markdown.NodeID) {
	// A hard break of spaces can print as a backslash, so the decisions read
	// the line without a last backslash and with one, and a second format
	// decides the same.
	line := bytes.TrimSuffix(bytes.TrimRight(p.tree.RestOfLine(id), " \t"), []byte{'\\'})
	withBreak := append(bytes.Clone(line), '\\')
	interrupts := func(lazy bool) bool {
		return markdown.InterruptsParagraph(line, lazy) || markdown.InterruptsParagraph(withBreak, lazy)
	}
	p.indent = -1
	if p.matched < p.prefixes && !interrupts(true) {
		// The line stays lazy when its canonical prefix is longer than its
		// printed content, which endLine knows (appendix B, trap 2). With
		// the prefix, the line gets the padding of trap 1.
		p.lazyLine, p.lazyPad = true, interrupts(false)
		return
	}
	p.matched = p.prefixes
	if interrupts(false) {
		p.pad = 4
	}
}

func (p *printer) endLine() {
	p.indent = -1
	if p.lineStart {
		p.writePrefix(true)
	}
	p.prefixLazyLine()
	p.write(lineFeed)
	p.lineStart = true
}

// prefixLazyLine writes the prefixes that the open lazy line does not have
// before its content, unless its full canonical prefix is longer than that
// content (appendix B, trap 2). The content is a measure of the printer's own
// output, so a second format decides the same (design 12).
func (p *printer) prefixLazyLine() {
	if !p.lazyLine {
		return
	}
	p.lazyLine = false
	if p.full || len(p.out)-p.lazyStart < p.prefixLen {
		return
	}
	prefix, n := make([]byte, 0, p.prefixLen), 0
	for i := range p.stack {
		if !p.stack[i].container {
			continue
		}
		if n >= p.lazyKept {
			prefix = append(prefix, p.stack[i].rest()...)
		}
		n++
	}
	if p.lazyPad {
		prefix = append(prefix, spaces[:4]...)
	}
	p.out = append(p.out[:p.lazyStart], append(prefix, p.out[p.lazyStart:]...)...)
	p.matched = p.prefixes
}

func (p *printer) exit() {
	t := p.tree
	f := &p.stack[len(p.stack)-1]
	if f.container && !f.started {
		// An empty container is its marker alone.
		start := len(p.out)
		f.blankFirst = false
		p.writePrefix(false)
		p.trimSpaces(start)
		p.write(lineFeed)
	}
	g := *f
	p.stack = p.stack[:len(p.stack)-1]
	if g.span {
		p.inSpan--
	}
	if g.inline >= 0 {
		in := p.inlines[g.inline]
		if in.label {
			p.inLabel--
		}
		if in.under {
			p.inUnder--
		}
		p.inlines = p.inlines[:g.inline]
	}
	if g.kind == markdown.Strikethrough {
		p.inStrike--
	}
	switch {
	case g.kind == markdown.CodeBlock && p.fence != nil:
		// Close the code, also after a last line without a line ending.
		if !p.lineStart || p.lineOpen {
			p.endLine()
		}
		// A fence line is never lazy.
		p.matched = p.prefixes
		p.writePrefix(false)
		p.write(p.fence)
		p.lineStart = false
		p.endLine()
		p.span, p.blanks, p.lastLeaf, p.open, p.bracket = g.span, 0, g.kind, false, false
		p.fence = nil
	case g.kind == markdown.TableCell && p.table != nil:
		c := &p.table.cells[len(p.table.cells)-1]
		c.end = len(p.out)
		if !p.full {
			c.width = markdown.DisplayWidth(p.out[c.start:c.end])
		}
	case isLeafBlock(g.kind):
		if p.table != nil {
			p.printTable()
		}
		if g.kind == markdown.LinkReferenceDefinition {
			p.def = defNone
		}
		if g.kind == markdown.Heading {
			if p.head == headSingle && !p.headDone {
				p.endHeading()
			}
			p.head = headNone
		}
		// A last line of code or HTML at the end of the input without a line
		// ending is a line of the value, also when it holds only syntax
		// (design 8.2).
		if !p.lineStart || !p.inputLine && (g.kind == markdown.CodeBlock || g.kind == markdown.HTMLBlock) {
			p.endLine()
		}
		p.open = g.kind == markdown.CodeBlock && p.fences == 1 || g.kind == markdown.HTMLBlock && !t.HTMLBlockClosed(g.id)
		p.span, p.blanks, p.lastLeaf = g.span, 0, g.kind
		p.bracket = g.kind == markdown.Paragraph && bytes.HasPrefix(bytes.TrimLeft(t.Raw(g.id), " \t"), []byte("["))
	case g.kind == markdown.BlockQuote:
		p.open, p.span = false, p.span || g.span
		p.quoteGap, p.quoteParent = p.lastLeaf == markdown.Paragraph, len(p.stack)-1
	case g.kind == markdown.List:
		p.span = p.span || g.span
		parent := &p.stack[len(p.stack)-1]
		parent.listOrdered, parent.listAlt, parent.listBullet = g.ordered, g.alt, g.bullet
	case isBlock(g.kind):
		p.span = p.span || g.span
	case g.kind == markdown.Document:
		if !p.lineStart {
			p.endLine()
		}
	}
}

// table is a table that prints: the lines that the printer wrote for it, which
// it aligns when the table ends.
type table struct {
	start  int                  // the offset in out where the table starts
	aligns []markdown.Alignment // the alignment of each column
	lines  []tableLine
	cells  []tableCell
}

// tableLine is a line of a table in out: its container prefixes from prefix
// to start, then its cells from index cells, or the delimiter row.
type tableLine struct {
	prefix, start int
	cells         int
	delimiter     bool
}

// tableCell is the content of a table cell in out, with its display width.
type tableCell struct {
	start, end, width int
}

// lineCells returns the cells of line i.
func (tb *table) lineCells(i int) []tableCell {
	end := len(tb.cells)
	if i+1 < len(tb.lines) {
		end = tb.lines[i+1].cells
	}
	return tb.cells[tb.lines[i].cells:end]
}

// startTableLine writes the container prefixes of a line of the table that
// prints, unless the line has started. delimiter reports whether the line is
// the delimiter row.
func (p *printer) startTableLine(delimiter bool) {
	if !p.lineStart {
		return
	}
	tb := p.table
	prefix := len(p.out)
	p.indent, p.lead, p.matched = -1, false, p.prefixes
	p.writePrefix(false)
	p.lineStart = false
	tb.lines = append(tb.lines, tableLine{prefix: prefix, start: len(p.out), cells: len(tb.cells), delimiter: delimiter})
}

// printTable writes the table that ends in place of the lines that the
// printer wrote for it: with outer pipes, a space inside each pipe, and
// delimiter cells of at least 3 dashes. Its columns are aligned by display
// width and its short rows get empty cells, unless that adds more bytes than
// the table has without them, or the table has as many missing cells as
// cmark-gfm's cap (appendix B, trap 14).
func (p *printer) printTable() {
	tb := p.table
	p.table, p.lineStart = nil, true
	if p.full {
		return
	}
	cols := len(tb.aligns)
	widths := make([]int, cols)
	for c := range widths {
		widths[c] = 3
	}
	rows, present := 0, 0
	for i, l := range tb.lines {
		if l.delimiter {
			continue
		}
		cells := tb.lineCells(i)
		rows++
		present += min(len(cells), cols)
		for c, cell := range cells[:min(len(cells), cols)] {
			widths[c] = max(widths[c], cell.width)
		}
	}
	short, aligned := 0, 0
	for i, l := range tb.lines {
		n := l.start - l.prefix + 2 // the prefixes, the first pipe and the line feed
		short += n
		aligned += n
		if l.delimiter {
			for _, w := range widths {
				short += 6
				aligned += w + 3
			}
			continue
		}
		cells := tb.lineCells(i)
		for c, cell := range cells {
			short += cell.end - cell.start + 3
			aligned += cell.end - cell.start + 3
			if c < cols {
				aligned += widths[c] - cell.width
			}
		}
		for c := len(cells); c < cols; c++ {
			aligned += widths[c] + 3
		}
	}
	align := aligned-short <= short && cols*rows-present < markdown.MaxMissingCells
	src := bytes.Clone(p.out[tb.start:])
	p.out = p.out[:tb.start]
	for i, l := range tb.lines {
		p.write(src[l.prefix-tb.start : l.start-tb.start])
		p.write([]byte{'|'})
		if l.delimiter {
			for c, a := range tb.aligns {
				w := 3
				if align {
					w = widths[c]
				}
				first, last := byte('-'), byte('-')
				if a == markdown.AlignLeft || a == markdown.AlignCenter {
					first = ':'
				}
				if a == markdown.AlignRight || a == markdown.AlignCenter {
					last = ':'
				}
				p.write([]byte{' ', first})
				p.write(bytes.Repeat([]byte{'-'}, w-2))
				p.write([]byte{last, ' ', '|'})
			}
			p.write(lineFeed)
			continue
		}
		cells := tb.lineCells(i)
		for c, cell := range cells {
			before, after := 0, 0
			if align && c < cols {
				pad := widths[c] - cell.width
				switch tb.aligns[c] {
				case markdown.AlignRight:
					before = pad
				case markdown.AlignCenter:
					before = pad / 2
				}
				after = pad - before
			}
			p.write(spaces[:1])
			p.writeSpaces(before)
			p.write(src[cell.start-tb.start : cell.end-tb.start])
			p.writeSpaces(after)
			p.write([]byte(" |"))
		}
		for c := len(cells); align && c < cols; c++ {
			p.write(spaces[:1])
			p.writeSpaces(widths[c])
			p.write([]byte(" |"))
		}
		p.write(lineFeed)
	}
}

func isPrefix(k markdown.Kind) bool {
	return k == markdown.QuoteMarker || k == markdown.ListMarker || k == markdown.ItemIndent || k == markdown.FootnoteIndent
}

func isLeafBlock(k markdown.Kind) bool {
	switch k {
	case markdown.Paragraph, markdown.Heading, markdown.ThematicBreak, markdown.CodeBlock, markdown.HTMLBlock,
		markdown.LinkReferenceDefinition, markdown.Table, markdown.FrontMatter:
		return true
	}
	return false
}

func isBlock(k markdown.Kind) bool {
	switch k {
	case markdown.BlockQuote, markdown.List, markdown.ListItem, markdown.FootnoteDefinition:
		return true
	}
	return isLeafBlock(k)
}

// headForm is how the open heading prints.
type headForm uint8

const (
	headNone   headForm = iota // no heading is open, or it is in a dialect span
	headSingle                 // an ATX heading
	headMulti                  // a setext heading of more than one line
)

// multiLine reports whether heading id has a line ending in its content: a
// setext heading whose leaves before its underline hold more than one line
// ending, as a soft break, a hard break or a code span over lines does.
func (p *printer) multiLine(id markdown.NodeID) bool {
	t := p.tree
	end, _ := t.Next(id)
	endings := 0
	for i := id + 1; i < end; i++ {
		switch k := t.Kind(i); {
		case k == markdown.SetextUnderline:
			return endings > 1
		case k.Leaf():
			raw := t.Raw(i)
			endings += bytes.Count(raw, lineFeed) + bytes.Count(raw, []byte{'\r'}) - bytes.Count(raw, []byte("\r\n"))
		}
	}
	return false
}

// endHeading ends the line of a heading that prints as ATX: an empty heading
// is its markers alone, and content that is number signs, or ends with them
// after a space or a tab, gets a closing sequence (appendix B, trap 6).
func (p *printer) endHeading() {
	p.headDone = true
	marker := bytes.Repeat([]byte{'#'}, p.headLevel)
	if p.lineStart {
		p.writePrefix(false)
		p.lineStart = false
		p.write(marker)
		return
	}
	if p.full {
		return
	}
	content := p.out[p.headStart:]
	if len(content) == 0 || content[len(content)-1] != '#' {
		return
	}
	if rest := bytes.TrimRight(content, "#"); len(rest) == 0 || rest[len(rest)-1] == ' ' || rest[len(rest)-1] == '\t' {
		p.write(spaces[:1])
		p.write(marker)
	}
}
