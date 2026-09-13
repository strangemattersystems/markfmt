package format

import (
	"bytes"
	"strconv"

	"github.com/strangemattersystems/markfmt/internal/markdown"
)

var (
	lineFeed = []byte{'\n'}
	spaces   = bytes.Repeat([]byte{' '}, 64)
)

// printer writes a tree in the canonical style.
type printer struct {
	tree *markdown.Tree
	out  []byte
	max  int  // the output limit
	full bool // a write would have passed max, so out is incomplete

	layout markdown.Layout
	spans  []markdown.NodeID // the dialect spans that the walk has not passed
	stack  []frame           // the open structure nodes, the document first

	lineStart bool          // the output is at the start of a line
	inputLine bool          // the next leaf starts a line of the input
	matched   int           // the containers that the input line of the output line matched
	indent    int           // the input column where columns that are not written yet start, or -1
	inSpan    int           // the open dialect spans
	written   bool          // the last leaf block that started has written content
	leafKind  markdown.Kind // the kind of the last leaf block that started
	pad       int           // spaces to write after the prefix of the next line
	backslash bool          // the last byte written is a backslash that is not an escape
	bracket0  bool          // the open paragraph starts with '[', so it could start with a link reference definition
	afterText bool          // the open block follows a paragraph or definition line without a blank line
	lead      bool          // the next leaf starts with columns that list item padding would take
	afterBox  bool          // the last leaf written is a task box
	replace   []byte        // bytes that content writes in place of the next leaf's bytes

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
	lastLeaf    markdown.Kind // the kind of the last leaf block
	bracket     bool          // the last leaf block is a paragraph that starts with '['
	quoteGap    []byte        // a blank line of a block quote that ends with a paragraph
	quoteParent int           // the index of that block quote's parent in stack
}

// frame is an open structure node.
type frame struct {
	id        markdown.NodeID
	kind      markdown.Kind
	children  int           // the blocks that started in it
	lastChild markdown.Kind // the kind of the last of them
	span      bool          // a dialect span
	tight     bool          // a list that is not loose, or an item of one
	fences    int           // the fence markers of a code block

	// A list numbers its items from start, and uses its second marker with
	// alt. Its second item decides lazy numbering.
	ordered, alt, lazy bool
	start              int
	minIndent          int // the columns that the list's items must continue on at least
	indent             int // the columns that a list item of the input continues on

	// The last list that ended in the node.
	listOrdered, listAlt bool

	// A block quote, list item or footnote definition writes marker on its
	// first line and rest on its other lines. With blankFirst, its first
	// line has only the marker, as in the input.
	container    bool
	started      bool
	blankFirst   bool
	marker, rest []byte
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
	p.lineStart, p.inputLine, p.indent = true, true, -1
	c := t.Walk()
	for e, ok := c.Next(); ok; e, ok = c.Next() {
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
	f := frame{id: id, kind: k, span: p.spanAt(id)}
	var prev frame
	if parent >= 0 {
		prev = p.stack[parent]
		if isBlock(k) {
			p.separate(parent, id, k, f.span)
		}
	}
	switch k {
	case markdown.BlockQuote:
		f.container, f.marker, f.rest = true, []byte("> "), []byte("> ")
		p.lastLeaf = markdown.Document
	case markdown.ListItem:
		f.container, f.tight, f.indent = true, p.stack[parent].tight, p.layout.ItemIndent()
	case markdown.FootnoteDefinition:
		f.container = true
		f.marker = []byte("[^" + string(t.FootnoteDefinitionLabel(id)) + "]: ")
		f.rest = spaces[:4]
	case markdown.List:
		f.tight = !t.ListLoose(id)
		f.start, f.ordered = t.ListStart(id)
		// Adjacent sibling lists of one type alternate their markers
		// (appendix B, trap 3).
		f.alt = prev.children > 0 && prev.lastChild == markdown.List && prev.listOrdered == f.ordered && !prev.listAlt
		// The block after the list must not continue its last item.
		f.minIndent = p.indentAfter(id) + 1
	}
	if isLeafBlock(k) {
		p.written, p.leafKind = false, k
		p.bracket0 = k == markdown.Paragraph && bytes.HasPrefix(p.firstLine(id), []byte("["))
	}
	if f.span {
		p.inSpan++
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
		case p.span || span:
			// Kept syntax (design 12).
			n = p.blanks
		case k == markdown.Table && f.lastChild == markdown.Paragraph && p.blanks == 0 && p.bracket:
			// The table split the paragraph off, which after a blank line
			// would start with a link reference definition.
			n = 0
		case k == markdown.Paragraph && f.lastChild == markdown.LinkReferenceDefinition && p.blanks == 0 &&
			markdown.InterruptsParagraph(p.firstLine(id), false):
			// The paragraph continues the definition's lines: after a blank
			// line its first line would start a block.
			n, p.pad = 0, 4
		case (f.kind == markdown.List || f.kind == markdown.ListItem) && f.tight:
			// A blank line would make the list loose.
			n = 0
		case k == markdown.LinkReferenceDefinition && f.lastChild == markdown.LinkReferenceDefinition:
			n = 0
		}
		if n == 0 && p.quoteGap != nil && p.quoteParent == parent && !markdown.InterruptsParagraph(p.firstLine(id), true) {
			// Without a blank line in the block quote, the block would
			// continue the quote's paragraph as a lazy line.
			p.write(p.quoteGap)
		}
		for range n {
			p.writePrefix(true)
			p.write(lineFeed)
		}
		p.afterText = n == 0 && (f.lastChild == markdown.Paragraph || f.lastChild == markdown.LinkReferenceDefinition)
	}
	f.children++
	f.lastChild = k
	p.blanks, p.open, p.span, p.quoteGap = 0, false, false, nil
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
	start, n := len(p.out), 0
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
			p.write(f.marker)
			f.started = true
			// Padding would take the columns that the content starts with.
			if f.blankFirst && f.kind != markdown.BlockQuote && p.lead {
				p.trimSpaces(start)
				p.write(lineFeed)
				start = len(p.out)
				for j := range p.stack[:i+1] {
					if p.stack[j].container {
						p.write(p.stack[j].rest)
					}
				}
			}
		case blank || n < p.matched:
			p.write(f.rest)
		default:
			break prefixes
		}
		n++
	}
	if blank {
		p.trimSpaces(start)
	}
}

func (p *printer) leaf(id markdown.NodeID, k markdown.Kind, start, end int) {
	t := p.tree
	top := &p.stack[len(p.stack)-1]
	switch {
	case k == markdown.ListMarker:
		p.listMarker(top, id, start)
	case k == markdown.BlankLine:
		// A blank line before the first block of a container is the rest of
		// its marker line, or a blank line at its start.
		if top.container && top.children == 0 {
			top.blankFirst = true
		} else {
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
	case k == markdown.Indent && (top.kind == markdown.Paragraph || top.kind == markdown.Heading || top.kind == markdown.ThematicBreak) && !p.written && p.inSpan == 0:
		// Indentation before a paragraph, a heading or a thematic break is not
		// meaning.
		p.indent = -1
	case (k == markdown.Indent || k == markdown.CodeIndent) && p.inSpan == 0:
		// Indentation is its columns, whatever tabs it holds (design 4.3):
		// content writes them.
	case k == markdown.Whitespace && p.afterBox && p.inSpan == 0:
		p.write(spaces[:1])
	case k == markdown.TrailingSpace && p.inSpan == 0:
	case k == markdown.HardBreakMarker && p.inSpan == 0 && t.Raw(id)[0] != '\\':
		// A hard break is a backslash, except after a backslash that is not
		// an escape, which the backslash would escape (appendix B, trap 10),
		// and in a paragraph that starts with '[', where the backslash could
		// be the destination of a link reference definition.
		if p.backslash || p.bracket0 {
			p.write(spaces[:2])
		} else {
			p.write([]byte{'\\'})
		}
	case len(p.out) == 0 && len(p.stack) == 2 && k == markdown.Text && string(t.Raw(id)) == "+++":
		// The first block never looks like front matter (appendix B, trap 7).
		p.indent = -1
		p.write(spaces[:1])
		p.content(id, start)
	default:
		if k == markdown.FenceMarker {
			top.fences++
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
	case !list.ordered && list.alt:
		f.marker = []byte("*")
	case !list.ordered:
		f.marker = []byte("-")
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
	padding := max(list.minIndent-len(f.marker), 1)
	if padding > 4 {
		// No padding lets the item continue on so many columns: the item keeps
		// its indentation, marker and padding.
		f.marker = p.sourceMarker(f, id, start)
	} else {
		f.marker = append(f.marker, spaces[:padding]...)
	}
	f.rest = spaces[:len(f.marker)]
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

// indentAfter returns the columns of indentation of the first line of the
// block after node id, when that block is an HTML block or a code block,
// whose indentation the printer keeps. Otherwise it returns 0.
func (p *printer) indentAfter(id markdown.NodeID) int {
	t := p.tree
	next, ok := t.Next(id)
	for ok && (t.Kind(next) == markdown.BlankLine || isPrefix(t.Kind(next))) {
		next, ok = t.Next(next)
	}
	if !ok || t.Kind(next) != markdown.HTMLBlock && t.Kind(next) != markdown.CodeBlock {
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
	line := bytes.TrimRight(p.tree.RestOfLine(id), " \t")
	open, full := 0, 0
	for i := range p.stack {
		if p.stack[i].container {
			open++
			full += len(p.stack[i].rest)
		}
	}
	p.indent = -1
	if p.matched < open && full > len(line) && !markdown.InterruptsParagraph(line, true) {
		return
	}
	p.matched = open
	if markdown.InterruptsParagraph(line, false) {
		p.pad = 4
	}
}

func (p *printer) endLine() {
	p.indent = -1
	if p.lineStart {
		p.writePrefix(true)
	}
	p.write(lineFeed)
	p.lineStart = true
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
	var gap []byte
	if f.kind == markdown.BlockQuote && p.lastLeaf == markdown.Paragraph && !p.full {
		start := len(p.out)
		p.writePrefix(true)
		p.write(lineFeed)
		gap = bytes.Clone(p.out[start:])
		p.out = p.out[:start]
	}
	g := *f
	p.stack = p.stack[:len(p.stack)-1]
	if g.span {
		p.inSpan--
	}
	switch {
	case isLeafBlock(g.kind):
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
		p.open = g.kind == markdown.CodeBlock && g.fences == 1 || g.kind == markdown.HTMLBlock && !t.HTMLBlockClosed(g.id)
		p.span, p.blanks, p.lastLeaf = g.span, 0, g.kind
		p.bracket = g.kind == markdown.Paragraph && bytes.HasPrefix(bytes.TrimLeft(t.Raw(g.id), " \t"), []byte("["))
	case g.kind == markdown.BlockQuote:
		p.open, p.span = false, p.span || g.span
		p.quoteGap, p.quoteParent = gap, len(p.stack)-1
	case g.kind == markdown.List:
		p.span = p.span || g.span
		parent := &p.stack[len(p.stack)-1]
		parent.listOrdered, parent.listAlt = g.ordered, g.alt
	case isBlock(g.kind):
		p.span = p.span || g.span
	case g.kind == markdown.Document:
		if !p.lineStart {
			p.endLine()
		}
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

// multiLine reports whether heading id has a line break in its content.
func (p *printer) multiLine(id markdown.NodeID) bool {
	t := p.tree
	end, _ := t.Next(id)
	for i := id + 1; i < end; i++ {
		if k := t.Kind(i); k == markdown.SoftBreak || k == markdown.HardBreak {
			return true
		}
	}
	return false
}

// endHeading ends the line of a heading that prints as ATX: an empty heading
// is its markers alone, and content that ends with number signs after a
// space or a tab gets a closing sequence (appendix B, trap 6). No content is
// only number signs: such a line is an ATX heading.
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
	if rest := bytes.TrimRight(content, "#"); rest[len(rest)-1] == ' ' || rest[len(rest)-1] == '\t' {
		p.write(spaces[:1])
		p.write(marker)
	}
}
