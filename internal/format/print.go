package format

import (
	"bytes"

	"github.com/strangemattersystems/markfmt/internal/markdown"
)

// printer writes a tree in the canonical style.
type printer struct {
	tree *markdown.Tree
	out  []byte
	max  int  // the output limit
	full bool // a write would have passed max, so out is incomplete
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
		p.write([]byte{'\n'})
		b = b[i+1:]
		if len(b) > 0 && b[0] == '\n' {
			b = b[1:]
		}
	}
}

// document prints the tree.
func (p *printer) document() {
	t := p.tree
	c := t.Walk()
	for e, ok := c.Next(); ok; e, ok = c.Next() {
		//exhaustive:enforce
		switch t.Kind(e.ID) {
		case markdown.Document, markdown.FrontMatter, markdown.BlockQuote, markdown.List, markdown.ListItem,
			markdown.Paragraph, markdown.ThematicBreak, markdown.Heading, markdown.CodeBlock, markdown.HTMLBlock,
			markdown.LinkReferenceDefinition, markdown.SoftBreak, markdown.HardBreak, markdown.CodeSpan,
			markdown.Autolink, markdown.RawHTML, markdown.Emphasis, markdown.Strong, markdown.Link, markdown.Image,
			markdown.Strikethrough, markdown.Table, markdown.TableRow, markdown.TableCell,
			markdown.FootnoteDefinition, markdown.FootnoteReference:
		case markdown.Text, markdown.CodeText, markdown.VerbatimLineEnding, markdown.InfoString, markdown.HTMLText,
			markdown.LinkLabel, markdown.Destination, markdown.Title, markdown.FrontMatterText, markdown.Escape,
			markdown.EntityRef, markdown.AutolinkText, markdown.CellPipeEscape, markdown.FootnoteLabel,
			markdown.BOM, markdown.LineEnding, markdown.BlankLine, markdown.Indent, markdown.ThematicRun,
			markdown.ATXMarker, markdown.ATXClose, markdown.Whitespace, markdown.CodeIndent, markdown.FenceMarker,
			markdown.SetextUnderline, markdown.QuoteMarker, markdown.ListMarker, markdown.ItemIndent,
			markdown.Bracket, markdown.Colon, markdown.AngleBracket, markdown.TitleQuote, markdown.FrontMatterFence,
			markdown.TrailingSpace, markdown.HardBreakMarker, markdown.CodeFence, markdown.Delimiter, markdown.Paren,
			markdown.TablePipe, markdown.TableDelimiter, markdown.TaskBox, markdown.FootnoteIndent, markdown.Caret:
			p.writeLF(t.Raw(e.ID))
		}
	}
}
