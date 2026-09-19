package format

import (
	"bytes"

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
	raw  map[markdown.NodeID]bool // the blocks of the document that print as the input holds them
	out  []byte
	max  int  // the output limit
	full bool // a write would have passed max, so out is incomplete

	layout markdown.Layout
	breaks map[markdown.NodeID][2]bool // the lists that prescan finds
	stack  []frame                     // the open structure nodes, the document first

	lineStart bool          // the output is at the start of a line
	inputLine bool          // the next leaf starts a line of the input
	lineBegin bool          // the leaf that the printer reads starts a line of the input
	matched   int           // the containers that the input line of the output line matched
	indent    int           // the input column where columns that are not written yet start, or -1
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
	delimText  [len(delimBytes)]bool

	// The last run of blank lines and prefix leaves that afterBlanks passed:
	// from its first node to the node after it.
	skipFrom markdown.NodeID
	skipTo   markdown.NodeID
	skipToOK bool
	skipOK   bool

	lineOffset     int    // where the open line of the output starts
	lineBreak      []byte // a paragraph line with a backslash, for the break decisions
	firstLineBreak []byte // a block's first line with a backslash, for the same decisions
	runs           []bool // the backtick run lengths of the code span being printed

	wordScope  markdown.NodeID // the text block that wordSpaces and wordStarts describe
	wordSpaces []uint32        // the offsets of whitespace in it
	wordStarts []span          // the offsets of each "://" and "www." in it

	table *table // the table that prints, or nil

	afters  []blockIndent // the results of indentAfter that a later list can ask for, the last node first
	inlines []inlineFrame // the state of the open inline nodes, the outermost first

	// The code block that is open: the fence that it writes around its lines,
	// the fence markers of the input, whether the printer is on the input's
	// opening fence line, and whether a line of code has not ended.
	fence             []byte
	fences            int
	opening, lineOpen bool

	// The link reference definition that is open: the part that prints,
	// whether the space before its destination is written, whether its
	// destination is in angle brackets, and its title quotes.
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
	afterRaw    bool          // the last block printed as written, so the blank lines after it are the input's
	lastLeaf    markdown.Kind // the kind of the last leaf block, or BlankLine after a blank line that follows a block that prints as written
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
	tight     bool          // a list that is not loose, or an item of one
	children  int           // the blocks that started in it
	inline    int32         // the index of an inline node's state in inlines, or -1

	// A list numbers its items from start, and uses its second marker with
	// alt. Its second item decides lazy numbering.
	ordered, alt, lazy bool
	bullet             byte
	start              int
	minIndent          int // the columns that the list's items must continue on at least
	indent             int // the columns that a list item of the input continues on

	// The last list that ended in the node.
	listOrdered, listAlt bool
	listBullet           byte
	outContent           int // the output column where the content of this container starts
	lastList             int // the output column where the items of the last child list continue, or 0

	// A block quote, list item or footnote definition writes marker on its
	// first line and the rest of its marker on its other lines. With
	// blankFirst, its first line has only the marker, as in the input.
	container  bool
	started    bool
	blankFirst bool
	keepBlank  bool // the item keeps its blank marker line
	marker     []byte
}

// document prints the tree.
func (p *printer) document() {
	t := p.tree
	// The output of the corpora is at most 3 times the input, and most of it
	// is about its size, so out starts there and rarely grows.
	_, size := t.NodeSpan(0)
	p.out = make([]byte, 0, min(int(size), p.max))
	p.layout = t.Layout()
	// A block of the document that holds a dialect span prints as the input
	// holds it: its bytes are its meaning where GitHub and CommonMark read it
	// differently, and the canonical containers around a span would move the
	// columns that its lines are read in.
	for _, span := range t.DialectSpans() {
		if block, ok := topLevelBlock(t, span); ok && !p.raw[block] {
			if p.raw == nil {
				p.raw = make(map[markdown.NodeID]bool)
			}
			p.raw[block] = true
		}
	}
	var depth int
	p.breaks, depth = p.prescan()
	p.stack = make([]frame, 0, depth)
	p.lineStart, p.inputLine, p.indent = true, true, -1
	c := t.Walk()
	for e, ok := c.Next(); ok && !p.full; e, ok = c.Next() {
		switch {
		case e.Exit:
			p.exit()
		case p.raw[e.ID]:
			p.rawBlock(&c, e.ID)
		default:
			p.node(e.ID)
		}
	}
}

// rawBlock writes block id of the document as the input holds it, with LF
// line endings, and walks the cursor past its nodes. The blank lines of the
// input before and after it stay: canonical ones could join it to the block
// next to it.
func (p *printer) rawBlock(c *markdown.Cursor, id markdown.NodeID) {
	t := p.tree
	k := t.Kind(id)
	p.layout.Visit(id)
	p.separate(len(p.stack)-1, id, k, true)
	p.writeLF(t.Raw(id))
	if !bytes.HasSuffix(p.out, lineFeed) {
		p.write(lineFeed)
	}
	for e, ok := c.Next(); ok && (!e.Exit || e.ID != id); e, ok = c.Next() {
		if !e.Exit {
			p.layout.Visit(e.ID)
		}
	}
	p.lineStart, p.inputLine, p.indent = true, true, -1
	p.afterRaw, p.blanks, p.lastLeaf, p.open, p.quoteGap = true, 0, k, false, false
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
	f := frame{id: id, kind: k, inline: -1}
	var prev frame
	if parent >= 0 {
		prev = p.stack[parent]
		if isBlock(k) {
			p.separate(parent, id, k, false)
		}
	}
	switch k {
	case markdown.BlockQuote:
		f.container, f.marker = true, quotePrefix
		p.lastLeaf = markdown.Document
	case markdown.ListItem:
		// An empty container ends with no leaf block, so a block quote that
		// ends with it ends with no paragraph.
		f.container, f.tight, f.indent = true, p.stack[parent].tight, p.layout.ItemIndent()
		p.lastLeaf = markdown.Document
	case markdown.FootnoteDefinition:
		f.container = true
		f.marker = []byte("[^" + string(t.FootnoteDefinitionLabel(id)) + "]: ")
		p.lastLeaf = markdown.Document
	case markdown.List:
		f.tight = !t.ListLoose(id)
		f.start, f.ordered = t.ListStart(id)
		// Adjacent sibling lists of one type alternate their markers.
		f.alt = prev.children > 0 && prev.lastChild == markdown.List && prev.listOrdered == f.ordered && !prev.listAlt
		if !f.ordered {
			f.bullet = p.bullet(id, prev)
		}
		// The block after the list must not continue its last item, and an
		// item that keeps the columns of its input marker sets where the
		// content of every item of the list starts.
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
	if k == markdown.Paragraph || k == markdown.Heading || k == markdown.TableCell {
		p.textBlock = id
	}
	if k == markdown.Table && p.tablePipes(id) {
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
	if k == markdown.CodeBlock {
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
			// The raw text is the label, which Kept keeps.
			in.label = true
			p.inLabel++
		}
		if before, after := t.Around(id); k == markdown.CodeSpan && p.inLabel == 0 &&
			before != '$' && after != '$' {
			// A code span next to '$' is GitHub math.
			in.code = p.codeSpan(id)
		}
		if (k == markdown.Link || k == markdown.Image) && t.LinkForm(id) == markdown.InlineLink && p.inLabel == 0 {
			in.tail, in.quotes = tailText, p.titleQuotes(id)
		}
		if (k == markdown.Emphasis || k == markdown.Strong || k == markdown.Strikethrough) && p.inLabel == 0 {
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
	if k == markdown.LinkReferenceDefinition {
		p.def, p.spaced, p.defAngle, p.defQuotes = defLabel, false, false, p.titleQuotes(id)
	}
	if k == markdown.Heading {
		p.head, p.headLevel, p.headDone = headSingle, t.HeadingLevel(id), false
		if p.multiLine(id) {
			// A setext heading of more than one line stays setext.
			p.head = headMulti
		}
	}
	p.stack = append(p.stack, f)
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
		case p.afterRaw:
			// A blank line after a block that prints as written is kept
			// syntax, with the prefixes of the containers that its line
			// matched: the next block can be in fewer containers than it is.
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
	case k == markdown.Indent && top.kind == markdown.TableRow && !p.written && p.indentHides(id):
		// A table that prints as written keeps this indentation: without it
		// the row's line would start a block, which ends the paragraph above
		// the table. separate writes no blank line before such a table, so
		// the indentation cannot start code either.
	case k == markdown.Indent && (top.kind == markdown.Paragraph || top.kind == markdown.Heading ||
		top.kind == markdown.ThematicBreak || top.kind == markdown.TableRow) && !p.written:
		// Indentation before a paragraph, a heading, a thematic break or the
		// first row of a table is not meaning. A table that prints as written
		// would start indented code with it.
		p.indent = -1
	case k == markdown.Indent || k == markdown.CodeIndent:
		// Indentation is its columns, whatever tabs it holds: content writes
		// them.
	case k == markdown.Whitespace && p.afterBox:
		p.write(spaces[:1])
	case k == markdown.TrailingSpace && p.inLabel == 0:
		// After a backslash that is not an escape, the line ending would
		// make a hard break (spec 6.7). The content of a heading that prints
		// as ATX ends on its line, where no break forms, and the heading
		// drops the space when it is read again.
		if p.backslash && p.head != headSingle {
			p.write(spaces[:1])
		}
	case k == markdown.HardBreakMarker && p.inLabel == 0 && t.Raw(id)[0] != '\\':
		// A hard break is a backslash, except after a backslash that is not
		// an escape, which the backslash would escape, and in a paragraph
		// that starts with '[', where the backslash could be the destination
		// of a link reference definition. After a character of a delimiter
		// run, the break keeps its input form: a backslash is punctuation and
		// a line ending is whitespace, so the form decides whether the run
		// flanks. After '$', it keeps its form too. After a word with an
		// extended autolink, the backslash would join the autolink, which
		// ends at whitespace.
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
		case p.backslash || p.bracket0 || autolinkWordBytes(lastWord(p.out)):
			p.write(spaces[:2])
		default:
			p.write([]byte{'\\'})
		}
	case len(p.out) == 0 && len(p.stack) == 2 && k == markdown.Text &&
		(p.head != headSingle || p.headDone) && !p.hardBreakAfter(id) &&
		string(bytes.TrimRight(t.RestOfLine(id), " \t\r\n")) == "+++":
		// The first block never looks like front matter.
		// A heading that prints as ATX writes its markers first, so its line
		// never starts with the fence, and a hard break ends the line with a
		// backslash or two spaces, which no fence has.
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

// content writes leaf id, whose own columns start at input column start,
// after the line's prefix and the columns between the prefix and the leaf.
func (p *printer) content(id markdown.NodeID, start int) {
	t := p.tree
	k, b := t.Kind(id), t.Raw(id)
	if p.replace != nil {
		b, p.replace = p.replace, nil
	}
	if k == markdown.ThematicRun {
		b = p.thematicRun()
	}
	if p.lineStart {
		if p.written && (p.leafKind == markdown.Paragraph || p.leafKind == markdown.Heading) &&
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
	if k == markdown.TaskBox {
		b = []byte("[ ]")
		if t.Raw(id)[1]|0x20 == 'x' {
			b = []byte("[x]")
		}
	}
	p.writeLF(b)
	p.written, p.afterBox = true, k == markdown.TaskBox
	p.backslash = len(b) > 0 && b[len(b)-1] == '\\' && k != markdown.Escape
}

const (
	defNone        defPart = iota // no definition is open
	defLabel                      // the label, up to the colon
	defDestination                // the destination
	defTitle                      // after the destination, before a title
	defInTitle                    // the title
	defEnd                        // after the title
)

const (
	tailNone        tailPart = iota // no inline link prints, or it keeps its bytes
	tailText                        // the link text, up to its closing bracket
	tailOpen                        // the opening parenthesis
	tailDestination                 // before and in the destination
	tailTitle                       // after the destination, before a title
	tailInTitle                     // the title
	tailEnd                         // after the title
)

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
	quoteBlanks := false
	next, ok := p.afterBlanks(f.id)
	if f.container && p.blanks > 0 && ok && p.raw[next] {
		// The blank lines before a block that prints as written are kept
		// syntax. They are the container's, so they stay in it:
		// after it they would be blank lines of its parent, which can make a
		// list loose. At the end of an item they do not (spec 5.3).
		//
		// A blank line keeps the columns that its container continues on when
		// a container holds that one: without them the line would belong to
		// the block above it, and a list whose item holds a blank line
		// between two blocks is loose (spec 5.3). A quote writes its marker,
		// so its lines need no columns.
		columns := false
		if f.kind != markdown.BlockQuote {
			for i := len(p.stack) - 2; i >= 0; i-- {
				if c := &p.stack[i]; c.container {
					end, _ := t.Next(c.id)
					columns = next < end
					break
				}
			}
		}
		for range p.blanks {
			p.writePrefix(true)
			if columns {
				p.writeSpaces(f.outContent - p.column())
			}
			p.write(lineFeed)
		}
		// The blank lines end the paragraph of every quote that they are in,
		// so no gap line goes before the next block.
		p.blanks, quoteBlanks, p.lastLeaf = 0, true, markdown.BlankLine
	}
	g := *f
	p.stack = p.stack[:len(p.stack)-1]
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
		p.afterRaw, p.blanks, p.lastLeaf, p.open, p.bracket = false, 0, g.kind, false, false
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
		// ending is a line of the value, also when it holds only syntax.
		if !p.lineStart || !p.inputLine && (g.kind == markdown.CodeBlock || g.kind == markdown.HTMLBlock) {
			p.endLine()
		}
		p.open = g.kind == markdown.CodeBlock && p.fences == 1 || g.kind == markdown.HTMLBlock && !t.HTMLBlockClosed(g.id)
		p.afterRaw, p.blanks, p.lastLeaf = false, 0, g.kind
		p.bracket = g.kind == markdown.Paragraph && bytes.HasPrefix(bytes.TrimLeft(t.Raw(g.id), " \t"), []byte("["))
	case g.kind == markdown.BlockQuote:
		p.open = false
		p.quoteGap, p.quoteParent = p.lastLeaf == markdown.Paragraph && !quoteBlanks, len(p.stack)-1
	case g.kind == markdown.ListItem:
		// The list keeps the greatest column that an item of it continues on,
		// for the block that follows the list. An item with no block ends at a
		// blank line, so it cannot hold a marker below it.
		if list := &p.stack[len(p.stack)-1]; g.children > 0 {
			list.lastList = max(list.lastList, g.outContent)
		}
	case g.kind == markdown.List:
		parent := &p.stack[len(p.stack)-1]
		parent.listOrdered, parent.listAlt, parent.listBullet = g.ordered, g.alt, g.bullet
		parent.lastList = g.lastList
	case isBlock(g.kind):
	case g.kind == markdown.Document:
		if !p.lineStart {
			p.endLine()
		}
	}
}

const (
	headNone   headForm = iota // no heading is open
	headSingle                 // an ATX heading
	headMulti                  // a setext heading of more than one line
)
