package markdown

// Alignment is the alignment of a table column.
type Alignment uint8

const (
	AlignNone Alignment = iota
	AlignLeft
	AlignCenter
	AlignRight
)

// cellHeader is the flag of a cell of a header row. The two low bits of the
// flags of a cell are its alignment.
const cellHeader = 4

// markerAlignment returns the alignment that the delimiter row cell b gives
// its column: a ':' at its start is left, at its end right, at both center.
func markerAlignment(b []byte) Alignment {
	switch left, right := b[0] == ':', b[len(b)-1] == ':'; {
	case left && right:
		return AlignCenter
	case left:
		return AlignLeft
	case right:
		return AlignRight
	}
	return AlignNone
}

// TableColumns returns the number of columns of table id: the cells of its
// delimiter row.
func (t *Tree) TableColumns(id NodeID) int {
	return len(t.appendAlignments(nil, id))
}

// appendAlignments appends to dst the alignment of each column of table id,
// from the cells of its delimiter row, which follows its header row.
func (t *Tree) appendAlignments(dst []Alignment, id NodeID) []Alignment {
	rows := 0
	for i := uint32(id) + 1; i < t.nodes[id].link && int(i) < len(t.nodes); i++ {
		switch m := t.nodes[i]; m.kind {
		case TableRow:
			if rows++; rows > 1 || m.link <= i {
				return dst
			}
			i = m.link - 1
		case TableDelimiter:
			if m.start < m.end && int(m.end) <= len(t.src) {
				dst = append(dst, markerAlignment(t.src[m.start:m.end]))
			}
		}
	}
	return dst
}

// CellAlignment returns the alignment of table cell id.
func (t *Tree) CellAlignment(id NodeID) Alignment {
	return Alignment(t.nodes[id].flags &^ cellHeader)
}

// CellHeader reports whether table cell id is in the header row.
func (t *Tree) CellHeader(id NodeID) bool {
	return t.nodes[id].flags&cellHeader != 0
}

// maxCells is the most cells that cmark-gfm reads in a table row: it counts
// them in a uint16. A line with more is not a row.
const maxCells = 65535

// MaxMissingCells is the most missing cells of a table after which cmark-gfm
// and GitHub read another row (GitHub API, 2026-09-13).
const MaxMissingCells = 0x80000

// cellFlags returns the flags of the cell at index col of the row at index row
// of a table whose columns have alignments aligns. A cell beyond the column
// count has no alignment.
func cellFlags(aligns []Alignment, row, col int) uint8 {
	var f uint8
	if col < len(aligns) {
		f = uint8(aligns[col])
	}
	if row == 0 {
		f |= cellHeader
	}
	return f
}

// isTableSpace reports whether c is a space, a tab, a vertical tab or a form
// feed, which cmark-gfm's table scanners skip and its cells trim.
func isTableSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\v' || c == '\f'
}

func skipTableSpace(src []byte, i, end uint32) uint32 {
	for i < end && isTableSpace(src[i]) {
		i++
	}
	return i
}

// pipeEnd returns the end of the '|' at i and the spaces after it, or i when
// no '|' is at i.
func pipeEnd(src []byte, i, end uint32) uint32 {
	if i == end || src[i] != '|' {
		return i
	}
	return skipTableSpace(src, i+1, end)
}

// cellEnd returns the end of the table cell that starts at i in src[:end]:
// the first '|' that does not follow a '\', or end. cmark-gfm's cell scanner
// takes the longest match, so a '|' after a '\' is always in the cell.
func cellEnd(src []byte, i, end uint32) uint32 {
	for j := i; j < end; j++ {
		if src[j] == '|' && (j == i || src[j-1] != '\\') {
			return j
		}
	}
	return end
}

// rowCells returns the number of cells of the table row in src[i:end], a line
// from its first byte that is not a space or a tab, as cmark-gfm reads a row:
// an optional '|', then cells, each ended by a '|' or by the end of the line,
// where a '|' takes the spaces after it. A line with no cell, or with more
// than maxCells cells, is not a row and gives 0.
func rowCells(src []byte, i, end uint32) int {
	n := 0
	for j := pipeEnd(src, i, end); j < end; {
		if n++; n > maxCells {
			return 0
		}
		k := cellEnd(src, j, end)
		if k == end {
			break
		}
		j = pipeEnd(src, k, end)
	}
	return n
}

// appendDelimiterRow appends to dst the alignment of each cell of the
// delimiter row in src[i:end], a line from its first byte that is not a space
// or a tab, and reports whether the line is one: an optional '|', then cells of
// optional spaces, an optional ':', one or more '-', an optional ':' and
// optional spaces, separated by '|', then an optional '|' and spaces
// (cmark-gfm's table_start). A line with more than maxCells cells is not one.
func appendDelimiterRow(dst []Alignment, src []byte, i, end uint32) ([]Alignment, bool) {
	j := i
	if j < end && src[j] == '|' {
		j++
	}
	for {
		j = skipTableSpace(src, j, end)
		start := j
		if j < end && src[j] == ':' {
			j++
		}
		if j == end || src[j] != '-' || len(dst) == maxCells {
			return dst, false
		}
		for j < end && src[j] == '-' {
			j++
		}
		if j < end && src[j] == ':' {
			j++
		}
		dst = append(dst, markerAlignment(src[start:j]))
		if j = skipTableSpace(src, j, end); j == end {
			return dst, true
		}
		if src[j] != '|' {
			return dst, false
		}
		if j = skipTableSpace(src, j+1, end); j == end {
			return dst, true
		}
	}
}

// tableLine adds the line to a table, and reports whether it did: as the
// delimiter row of a table whose header row is the last line of the open
// paragraph, or as a row of the open table. first is the first byte of the
// rest of the line that is not a space or a tab, after indent columns.
func (p *blockParser) tableLine(first uint32, indent int) bool {
	l := p.l
	switch p.leaf.kind {
	case paragraphLeaf:
		if indent >= 4 || p.leaf.tried {
			return false
		}
		aligns, ok := appendDelimiterRow(p.aligns[:0], p.src, first, l.end)
		p.aligns = aligns
		if !ok {
			return false
		}
		header := p.pending[len(p.pending)-1].rest
		if rowCells(p.src, p.skipSpace(header.start, header.end), header.end) != len(aligns) {
			// A paragraph tries a header once, as cmark-gfm does.
			p.leaf.tried = true
			return false
		}
		p.startTable(first)
		return true
	case tableLeaf:
		if rowCells(p.src, first, l.end) == 0 || p.leaf.columns*p.leaf.rows-p.leaf.cells > MaxMissingCells {
			return false
		}
		p.appendPrefix()
		p.tableRow(p.rest())
		return true
	case noLeaf, indentedCodeLeaf, fencedCodeLeaf, htmlLeaf:
	}
	return false
}

// startTable appends the lines of the open paragraph before its last line as a
// paragraph, with no definition parse and with the cell pipe rule, then opens a
// table whose header row is the last line and appends the delimiter row that
// starts at first.
func (p *blockParser) startTable(first uint32) {
	n := len(p.pending) - 1
	if n > 0 {
		p.appendLines(Paragraph, p.pending[:n], true)
		p.b.close()
	}
	header := p.pending[n]
	p.appendPendingPrefix(header)
	p.b.open(Table)
	p.leaf = leafBlock{kind: tableLeaf, columns: len(p.aligns)}
	p.b.split = splitVirt(int(header.col), int(header.used))
	p.tableRow(header.rest)
	p.clearPending()
	p.appendPrefix()
	p.delimiterRow(first)
}

// tableRow appends the table row in l, a line with its indentation.
func (p *blockParser) tableRow(l line) {
	p.b.open(TableRow)
	i := p.skipSpace(l.start, l.end)
	p.b.leafIf(Indent, i)
	i = p.tablePipe(i, l.end)
	cells := 0
	for i < l.end {
		k := cellEnd(p.src, i, l.end)
		p.tableCell(i, k, cells)
		cells++
		if k == l.end {
			break
		}
		i = p.tablePipe(k, l.end)
	}
	p.b.leafIf(LineEnding, l.eol)
	p.b.close()
	p.leaf.rows++
	p.leaf.cells += min(cells, p.leaf.columns)
}

// tableCell appends the table cell in src[i:k], at index col of its row: its
// content, trimmed of spaces, and its inlines.
func (p *blockParser) tableCell(i, k uint32, col int) {
	p.b.open(TableCell)
	if f := cellFlags(p.aligns, p.leaf.rows, col); f != 0 {
		p.b.flag(f)
	}
	start, end := skipTableSpace(p.src, i, k), k
	for end > start && isTableSpace(p.src[end-1]) {
		end--
	}
	p.b.leafIf(Whitespace, start)
	switch {
	case start == end:
	case p.pass1:
		p.b.leaf(Text, end)
	default:
		p.cell[0] = pendingLine{rest: line{start: start, end: end, eol: end}}
		p.inline.pipes = true
		p.inline.inlines(p.cell[:])
	}
	p.b.leafIf(Whitespace, k)
	p.b.close()
}

// tablePipe appends the '|' at i, if any, and the spaces after it, and returns
// their end.
func (p *blockParser) tablePipe(i, end uint32) uint32 {
	j := pipeEnd(p.src, i, end)
	if j > i {
		p.b.leaf(TablePipe, i+1)
		p.b.leafIf(Whitespace, j)
	}
	return j
}

func (p *blockParser) delimiterRow(first uint32) {
	l := p.l
	p.b.leafIf(Indent, first)
	for i := p.tablePipe(first, l.end); i < l.end; {
		j := skipTableSpace(p.src, i, l.end)
		p.b.leafIf(Whitespace, j)
		k := j
		if k < l.end && p.src[k] == ':' {
			k++
		}
		for k < l.end && p.src[k] == '-' {
			k++
		}
		if k < l.end && p.src[k] == ':' {
			k++
		}
		p.b.leaf(TableDelimiter, k)
		j = skipTableSpace(p.src, k, l.end)
		p.b.leafIf(Whitespace, j)
		i = p.tablePipe(j, l.end)
		if i == j {
			break
		}
	}
	p.b.leafIf(LineEnding, l.eol)
}
