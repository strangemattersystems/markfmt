package markdown

// inlineParser runs the inline phase on the lines of one block (design 6).
// It writes pieces to a scratch buffer, then appends them to the builder in
// one pass, so a later decision can change a piece that is already written.
type inlineParser struct {
	b     *builder
	src   []byte
	defs  *definitions
	lines []pendingLine
	arena []prefixLeaf
	label []byte // a normalized label

	pieces     []piece
	k          int    // the line being scanned
	start      uint32 // start of the first piece
	lineStart  uint32 // start of the content of the line being scanned, after its Indent leaf
	delims     []delimiter
	brackets   []bracket
	seq        int // push sequence number of the last bracket
	linkFormed int // sequence number of the opener of the last link

	ticks    []uint32 // one past the start of the last backtick run of each length that a search passed
	ticksAll bool     // a backtick search reached the end of the block

	failed [len(closers)]uint32 // one past the start of the last failed search for each raw HTML closer

	// pipes makes each "\|" pair a CellPipeEscape: in a table cell, and in the
	// paragraph split off above a table (design 8.2). inlines clears it.
	pipes bool
}

// pos is a position in the lines of a block: offset i of line k, at most the
// end of the line.
type pos struct {
	k int
	i uint32
}

// piece is a leaf in the scratch buffer. It starts at the end of the piece
// before it.
type piece struct {
	kind  Kind
	virt  uint8
	open  Kind  // the span that opens before the leaf, or Document for none
	flags uint8 // the flags of that span, or of a CellPipeEscape leaf
	close bool  // the innermost span closes after the leaf
	join  bool  // the leaf of the piece before extends over this piece
	held  bool  // a later decision can change the piece, so no text joins it while scanning
	end   uint32
	owner uint32 // the owner of a prefix leaf
}

// inlines appends the inline content of lines: the Indent leaf of each line,
// its inlines, and a break and the prefix leaves before each later line. The
// prefix leaves of the first line and the line ending of the last line are
// the caller's.
func (s *inlineParser) inlines(lines []pendingLine) {
	s.begin(lines)
	for {
		l := s.lines[s.k].rest
		if i := s.end(); i < l.end {
			s.scan(i, l.end)
			continue
		}
		s.trailingSpace()
		if s.k+1 == len(s.lines) {
			break
		}
		if s.pieces[len(s.pieces)-1].kind == HardBreakMarker {
			s.push(piece{kind: LineEnding, close: true, end: l.eol})
		} else {
			s.push(piece{kind: LineEnding, open: SoftBreak, close: true, end: l.eol})
		}
		s.k++
		s.startLine()
	}
	s.processEmphasis(-1)
	s.emit()
	s.pipes = false
}

// begin starts the pieces of lines at the Indent leaf of the first line.
func (s *inlineParser) begin(lines []pendingLine) {
	s.lines, s.pieces, s.delims, s.brackets, s.k = lines, s.pieces[:0], s.delims[:0], s.brackets[:0], 0
	s.seq, s.linkFormed = 0, 0
	s.start = lines[0].rest.start
	s.ticks, s.ticksAll, s.failed = s.ticks[:0], false, [len(closers)]uint32{}
	s.startLine()
}

// mark is a position in the pieces that a failed construct rewinds to.
type mark struct {
	pieces, k int
	lineStart uint32
}

func (s *inlineParser) mark() mark {
	return mark{len(s.pieces), s.k, s.lineStart}
}

func (s *inlineParser) reset(m mark) {
	s.pieces, s.k, s.lineStart = s.pieces[:m.pieces], m.k, m.lineStart
}

// nextLine pushes the line ending of the line as a piece of kind k, and the
// prefix and Indent leaves of the next line. It reports false on the last
// line.
func (s *inlineParser) nextLine(k Kind) bool {
	if s.k+1 == len(s.lines) {
		return false
	}
	s.push(piece{kind: k, end: s.lines[s.k].rest.eol})
	s.k++
	s.startLine()
	return true
}

// scan pushes the pieces of the inline that starts at i, on a line that ends
// at end. A construct can end on a later line.
func (s *inlineParser) scan(i, end uint32) {
	switch s.src[i] {
	case '\\':
		switch {
		case i+1 == end && s.k+1 < len(s.lines):
			s.push(piece{kind: HardBreakMarker, open: HardBreak, end: i + 1})
		case s.pipes && i+1 < end && s.src[i+1] == '|':
			s.push(piece{kind: CellPipeEscape, flags: uint8(Text), end: i + 2})
		case s.pipes && i+2 < end && s.src[i+1] == '\\' && s.src[i+2] == '|':
			s.push(piece{kind: CellPipeEscape, flags: uint8(Text), end: i + 3})
		case i+1 < end && isASCIIPunct(s.src[i+1]):
			s.push(piece{kind: Escape, end: i + 2})
		default:
			s.text(i + 1)
		}
	case '&':
		if j := entityEnd(s.src, i, end); j > 0 {
			s.push(piece{kind: EntityRef, end: j})
		} else {
			s.text(i + 1)
		}
	case '`':
		s.codeSpan(i, end)
	case '*', '_', '~':
		s.delimiterRun(i, end)
	case '[':
		s.openBracket(i, false)
	case '!':
		if i+1 < end && s.src[i+1] == '[' {
			s.openBracket(i, true)
		} else {
			s.text(i + 1)
		}
	case ']':
		s.closeBracket(i, end)
	case '<':
		if !s.autolink(i, end) && !s.rawHTML(i, end) {
			s.text(i + 1)
		}
	default:
		s.text(s.textEnd(i, end))
	}
}

// startLine pushes the prefix leaves of the line after the first, and the
// Indent leaf of the line.
func (s *inlineParser) startLine() {
	pl := s.lines[s.k]
	if s.k > 0 {
		for _, x := range s.arena[pl.prefixFirst : pl.prefixFirst+pl.prefixN] {
			s.push(piece{kind: x.kind, virt: x.virt, end: x.end, owner: x.owner})
		}
	}
	virt := splitVirt(int(pl.col), int(pl.used))
	i := pl.rest.start
	for i < pl.rest.end && isSpaceOrTab(s.src[i]) {
		i++
	}
	if i > s.end() {
		s.push(piece{kind: Indent, virt: virt, end: i})
	}
	s.lineStart = i
}

// trailingSpace ends the line with its trailing spaces and tabs as a
// TrailingSpace leaf, or as the HardBreakMarker of a hard break when the
// line has a later line and ends with two spaces (CM 633).
func (s *inlineParser) trailingSpace() {
	l, n := s.lines[s.k].rest, len(s.pieces)
	if n == 0 || s.pieces[n-1].kind != Text || s.pieces[n-1].end != l.end {
		return
	}
	start, i := s.startOf(n-1), l.end
	for i > start && isSpaceOrTab(s.src[i-1]) {
		i--
	}
	switch i {
	case l.end:
		return
	case start:
		s.pieces = s.pieces[:n-1]
	default:
		s.pieces[n-1].end = i
	}
	if s.k+1 < len(s.lines) && l.end-i >= 2 && s.src[l.end-1] == ' ' && s.src[l.end-2] == ' ' {
		s.push(piece{kind: HardBreakMarker, open: HardBreak, end: l.end})
		return
	}
	s.push(piece{kind: TrailingSpace, end: l.end})
}

var inlineTriggers = [256]bool{'\\': true, '&': true, '`': true, '<': true, '*': true, '_': true, '~': true, '[': true, ']': true, '!': true}

// verbatim pushes pieces of kind text up to p, with a VerbatimLineEnding,
// the prefix leaves and the Indent leaf at each line boundary.
func (s *inlineParser) verbatim(p pos, text Kind) {
	for s.k < p.k {
		s.pushContent(text, s.lines[s.k].rest.end)
		s.nextLine(VerbatimLineEnding)
	}
	s.pushContent(text, p.i)
}

// byteAt returns the byte at p, or false at the end of its line.
func (s *inlineParser) byteAt(p pos) (byte, bool) {
	if p.i < s.lines[p.k].rest.end {
		return s.src[p.i], true
	}
	return 0, false
}

func (s *inlineParser) expect(p pos, c byte) (pos, bool) {
	if b, ok := s.byteAt(p); ok && b == c {
		return pos{p.k, p.i + 1}, true
	}
	return pos{}, false
}

// skipSpace returns the position after the spaces and tabs at p, with at
// most one line ending, and whether it moved.
func (s *inlineParser) skipSpace(p pos) (pos, bool) {
	q := p
	for c, ok := s.byteAt(q); ok && isSpaceOrTab(c); c, ok = s.byteAt(q) {
		q.i++
	}
	if _, ok := s.byteAt(q); !ok && q.k+1 < len(s.lines) {
		q = pos{q.k + 1, s.lines[q.k+1].rest.start}
		for c, ok := s.byteAt(q); ok && isSpaceOrTab(c); c, ok = s.byteAt(q) {
			q.i++
		}
	}
	return q, q != p
}

// textEnd returns the end of the run of bytes from i that no inline construct
// starts in, before end.
func (s *inlineParser) textEnd(i, end uint32) uint32 {
	for i++; i < end && !inlineTriggers[s.src[i]]; i++ {
	}
	return i
}

// text pushes a Text piece to end, joined with a Text piece before it.
func (s *inlineParser) text(end uint32) {
	if n := len(s.pieces); n > 0 && s.pieces[n-1].kind == Text && !s.pieces[n-1].close && !s.pieces[n-1].held {
		s.pieces[n-1].end = end
		return
	}
	s.push(piece{kind: Text, end: end})
}

func (s *inlineParser) push(x piece) {
	s.pieces = append(s.pieces, x)
}

// pushContent pushes content pieces of kind k to end, on the line being
// scanned. With pipes, each "\|" pair is a CellPipeEscape piece of group k,
// and in a destination or a title, which decode escapes, an unescaped '\'
// before the pair joins it (design 8.2).
func (s *inlineParser) pushContent(k Kind, end uint32) {
	decodes := k == Destination || k == Title
	for i := s.end(); s.pipes && i+1 < end; i++ {
		if s.src[i] != '\\' {
			continue
		}
		var n uint32
		switch {
		case s.src[i+1] == '|':
			n = 2
		case decodes && i+2 < end && s.src[i+1] == '\\' && s.src[i+2] == '|':
			n = 3
		case decodes && isASCIIPunct(s.src[i+1]):
			i++
			continue
		default:
			continue
		}
		s.pushIf(k, i)
		s.push(piece{kind: CellPipeEscape, flags: uint8(k), end: i + n})
		i += n - 1
	}
	s.pushIf(k, end)
}

// pushIf pushes a piece of kind k to end, unless the last piece ends there.
func (s *inlineParser) pushIf(k Kind, end uint32) {
	if end > s.end() {
		s.push(piece{kind: k, end: end})
	}
}

func (s *inlineParser) end() uint32 {
	return s.startOf(len(s.pieces))
}

func (s *inlineParser) startOf(j int) uint32 {
	if j == 0 {
		return s.start
	}
	return s.pieces[j-1].end
}

// emit appends the pieces to the builder, with their spans. A piece with
// join, and a Text piece after a Text piece, extend the leaf before them.
func (s *inlineParser) emit() {
	for j, x := range s.pieces {
		if x.open != Document {
			s.b.open(x.open)
			if x.flags != 0 {
				s.b.flag(x.flags)
			}
		}
		if j+1 < len(s.pieces) {
			if y := s.pieces[j+1]; !x.close && y.open == Document && (y.join || x.kind == Text && y.kind == Text) {
				continue
			}
		}
		s.b.split = x.virt
		if _, ok := x.kind.owner(); ok {
			s.b.prefix(x.kind, x.end, x.owner)
		} else {
			s.b.leaf(x.kind, x.end)
		}
		if x.kind == CellPipeEscape {
			s.b.flagLeaf(x.flags)
		}
		if x.close {
			s.b.close()
		}
	}
}
