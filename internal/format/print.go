package format

import (
	"bytes"

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

	lineStart bool // the output is at the start of a line
	inputLine bool // the next leaf starts a line of the input
	matched   int  // the containers that the input line of the output line matched
	indent    int  // the input column where columns that are not written yet start, or -1
	inSpan    int  // the open dialect spans
	written   bool // the last leaf block that started has written content

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

	// A block quote, list item or footnote definition writes marker on its
	// first line and rest on its other lines. With blankFirst, its first
	// line has only the marker, as in the input.
	container    bool
	started      bool
	blankFirst   bool
	indent       int // the columns that a list item continues on
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
	if parent >= 0 && isBlock(k) {
		p.separate(parent, k, f.span)
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
	}
	if isLeafBlock(k) {
		p.written = false
	}
	if f.span {
		p.inSpan++
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

// separate writes the blank lines before a block of kind k that starts in
// the frame at index parent. span reports whether the block is a dialect
// span.
func (p *printer) separate(parent int, k markdown.Kind, span bool) {
	f := &p.stack[parent]
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
		case (f.kind == markdown.List || f.kind == markdown.ListItem) && f.tight:
			// A blank line would make the list loose.
			n = 0
		case k == markdown.LinkReferenceDefinition && f.lastChild == markdown.LinkReferenceDefinition:
			n = 0
		}
		if n == 0 && p.quoteGap != nil && p.quoteParent == parent {
			// Without a blank line in the block quote, the block would
			// continue the quote's paragraph as a lazy line.
			p.write(p.quoteGap)
		}
		for range n {
			p.writePrefix(true)
			p.write(lineFeed)
		}
	}
	f.children++
	f.lastChild = k
	p.blanks, p.open, p.span, p.quoteGap = 0, false, false, nil
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
			if f.blankFirst {
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
	case k == markdown.BOM, k == markdown.QuoteMarker, k == markdown.ItemIndent, k == markdown.FootnoteIndent,
		top.kind == markdown.Document, top.kind == markdown.List:
		// The printer writes container syntax from its stack.
	case top.container:
		// The start of a footnote definition.
		p.indent = end
		if (k == markdown.LineEnding || k == markdown.VerbatimLineEnding) && top.children == 0 {
			top.blankFirst = true
		}
	case k == markdown.LineEnding, k == markdown.VerbatimLineEnding:
		p.endLine()
	case k == markdown.Indent && (top.kind == markdown.Paragraph || top.kind == markdown.Heading) && !p.written && p.inSpan == 0:
		// Indentation before a paragraph or a heading is not meaning.
		p.indent = -1
	case (k == markdown.Indent || k == markdown.CodeIndent) && p.inSpan == 0:
		// Indentation is its columns, whatever tabs it holds (design 4.3):
		// content writes them.
	case len(p.out) == 0 && k == markdown.ThematicRun && string(t.Raw(id)) == "---":
		// The first block never looks like front matter (appendix B, trap 7).
		p.lineStart = false
		p.write([]byte("***"))
	case len(p.out) == 0 && k == markdown.Text && string(t.Raw(id)) == "+++":
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

// listMarker sets the marker and the rest of list item f from its ListMarker
// leaf id, whose own columns start at column start, so that the item keeps
// its indentation, marker and padding.
func (p *printer) listMarker(f *frame, id markdown.NodeID, start int) {
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
	f.marker = append(append(bytes.Clone(spaces[:lead-start]), chars...), spaces[:max(f.indent-(lead-start)-len(chars), 1)]...)
	f.rest = bytes.Repeat(spaces[:1], len(f.marker))
}

// content writes leaf id, whose own columns start at input column start,
// after the line's prefix and the columns between the prefix and the leaf.
func (p *printer) content(id markdown.NodeID, start int) {
	t := p.tree
	if p.lineStart {
		p.writePrefix(false)
		p.lineStart = false
	}
	if p.indent >= 0 {
		p.writeSpaces(start - p.indent)
		p.indent = -1
	}
	b := t.Raw(id)
	if v := t.SplitTab(id); v > 0 {
		p.writeSpaces(v)
		b = b[1:]
	}
	p.writeLF(b)
	p.written = true
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
		if !p.lineStart {
			p.endLine()
		}
		p.open = g.kind == markdown.CodeBlock && g.fences == 1 || g.kind == markdown.HTMLBlock && !t.HTMLBlockClosed(g.id)
		p.span, p.blanks, p.lastLeaf = g.span, 0, g.kind
		p.bracket = g.kind == markdown.Paragraph && bytes.HasPrefix(bytes.TrimLeft(t.Raw(g.id), " \t"), []byte("["))
	case g.kind == markdown.BlockQuote:
		p.open, p.span = false, p.span || g.span
		p.quoteGap, p.quoteParent = gap, len(p.stack)-1
	case isBlock(g.kind):
		p.span = p.span || g.span
	case g.kind == markdown.Document:
		if !p.lineStart {
			p.endLine()
		}
	}
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
