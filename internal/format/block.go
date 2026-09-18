package format

import (
	"bytes"

	"github.com/strangemattersystems/markfmt/internal/markdown"
)

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
		case k == markdown.Table && f.lastChild == markdown.Paragraph && p.blanks == 0 &&
			(p.tableIndentHides(id) || p.interrupts(id, false) || p.startsBlock(id)):
			// The table split the paragraph above it, so its first row is a line
			// of that paragraph. After a blank line the line would start a block,
			// or the indentation that hides its block start would start code.
			n = 0
		case (k == markdown.Paragraph || k == markdown.LinkReferenceDefinition) &&
			f.lastChild == markdown.LinkReferenceDefinition && p.blanks == 0 &&
			(p.interrupts(id, false) || p.startsBlock(id)):
			// The paragraph continues the definition's lines: after a blank
			// line its first line would start a block. A line that would
			// interrupt the paragraph gets padding.
			// A lazy line that starts a block needs the padding as well, but
			// one that interrupts only as a table delimiter row does not: the
			// padding would put it in the containers that it does not match,
			// where it would make the definition line a header.
			n = 0
			if p.interrupts(id, false) && (!p.lazyFirst(id) || p.startsBlock(id)) {
				p.pad = 4
			}
		case (k == markdown.Paragraph || k == markdown.LinkReferenceDefinition) && p.lazyFirst(id):
			// A blank line would end the lazy line's continuation, and the
			// containers that it does not match would end with it (appendix
			// B, trap 2).
			n = 0
		case (f.kind == markdown.List || f.kind == markdown.ListItem) && f.tight:
			// A blank line would make the list loose.
			n = 0
		case k == markdown.LinkReferenceDefinition && f.lastChild == markdown.LinkReferenceDefinition && f.kind != markdown.ListItem:
			// In a loose list item, the blank line can be what makes the list
			// loose (spec 5.3).
			n = 0
		}
		if n == 0 && p.quoteGap && p.quoteParent == parent && !p.interrupts(id, true) {
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
	if k != markdown.List {
		// Only a list that follows a list can have its marker read as a line
		// of that list's last item.
		f.lastList = 0
	}
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
	if t.Kind(id) == markdown.Table && p.inSpan == 0 && p.tablePipes(id) {
		// The table prints its cells between pipes, so its first line starts
		// with one, which starts no block.
		return pipe
	}
	if t.Kind(id) == markdown.CodeBlock && !p.isSpan(id) {
		// Code outside a dialect span prints as a fence, whatever the form of
		// its input, and the decisions above read the line that prints.
		fence, _ := p.codeFence(id)
		return fence
	}
	i := id + 1
	for !t.Kind(i).Leaf() || t.Kind(i) == markdown.Indent {
		i++
	}
	return bytes.Trim(t.RestOfLine(i), " \t")
}

// interrupts reports whether the first line of block id, as it prints, would
// interrupt a paragraph above it, and startsBlock whether it would start a
// block. A hard break of spaces can print as a backslash, so both read the
// line in either form, and a second format decides the same.
func (p *printer) interrupts(id markdown.NodeID, lazy bool) bool {
	line, withBreak := p.firstLines(id)
	return markdown.InterruptsParagraph(line, lazy) || markdown.InterruptsParagraph(withBreak, lazy)
}

func (p *printer) startsBlock(id markdown.NodeID) bool {
	line, withBreak := p.firstLines(id)
	return markdown.StartsBlock(line) || markdown.StartsBlock(withBreak)
}

func (p *printer) firstLines(id markdown.NodeID) (line, withBreak []byte) {
	line = bytes.TrimSuffix(p.firstLine(id), []byte{'\\'})
	p.firstLineBreak = append(append(p.firstLineBreak[:0], line...), '\\')
	return line, p.firstLineBreak
}

// indentAfter returns the columns of indentation of the first line of the
// block after node id, when that block is an HTML block or a block that
// prints as written, both of which keep their indentation. Otherwise it returns 0. Nested lists that end
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

// blockIndent is a result of indentAfter for the node after a list.
type blockIndent struct {
	next markdown.NodeID
	cols int
}

// indentAt returns what indentAfter returns for the node next, which
// ok reports is in the tree.
func (p *printer) indentAt(next markdown.NodeID, ok bool) int {
	t := p.tree
	for ok && (t.Kind(next) == markdown.BlankLine || isPrefix(t.Kind(next))) {
		next, ok = t.Next(next)
	}
	if !ok || t.Kind(next) != markdown.HTMLBlock && !p.raw[next] {
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

// indentHides reports whether the indentation at leaf id keeps the rest of
// its line from starting a block: a heading, an HTML block or a fence.
func (p *printer) indentHides(id markdown.NodeID) bool {
	line := bytes.TrimLeft(p.tree.RestOfLine(id), " \t")
	return markdown.InterruptsParagraph(line, false) || markdown.StartsBlock(line)
}

// hardBreakAfter reports whether a hard break follows leaf id, so that the
// printer ends its line with a backslash or two spaces.
func (p *printer) hardBreakAfter(id markdown.NodeID) bool {
	next, ok := p.tree.Next(id)
	return ok && p.tree.Kind(next) == markdown.HardBreak
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
			if f.kind == markdown.ListItem && !f.started && f.sign() == '-' {
				return []byte("***")
			}
			break
		}
	}
	return []byte("---")
}

// afterBlanks returns the node after node id, its blank lines and its prefix
// leaves, and false at the end of the tree.
//
// Block quotes that end together ask this of the same run, so the last run
// passed is cached. Without it, exits at depth d walk the run d times.
func (p *printer) afterBlanks(id markdown.NodeID) (markdown.NodeID, bool) {
	t := p.tree
	next, ok := t.Next(id)
	from := next
	for ok && (t.Kind(next) == markdown.BlankLine || isPrefix(t.Kind(next))) {
		if p.skipOK && next == p.skipFrom {
			next, ok = p.skipTo, p.skipToOK
			break
		}
		next, ok = t.Next(next)
	}
	p.skipFrom, p.skipTo, p.skipToOK, p.skipOK = from, next, ok, true
	return next, ok
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
