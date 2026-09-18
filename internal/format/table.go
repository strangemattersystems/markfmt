package format

import (
	"bytes"

	"github.com/strangemattersystems/markfmt/internal/markdown"
)

// pipe is the first byte of a line of a table that prints with pipes.
var pipe = []byte("|")

// tablePipes reports whether table id prints its cells between pipes: a
// dialect span in it keeps its bytes, and so does whitespace that a canonical
// table would drop (design 12).
func (p *printer) tablePipes(id markdown.NodeID) bool {
	return !p.isSpan(id) && !p.spanInside(id) && !p.oddSpace(id)
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

// tableIndentHides reports whether the first row of table id starts with
// indentation that keeps its line from starting a block. A table that prints
// with pipes has no such row.
func (p *printer) tableIndentHides(id markdown.NodeID) bool {
	end, _ := p.tree.Next(id)
	leaf := id + 1
	for leaf < end && !p.tree.Kind(leaf).Leaf() {
		leaf++
	}
	return leaf < end && p.tree.Kind(leaf) == markdown.Indent && p.indentHides(leaf) && !p.tablePipes(id)
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
