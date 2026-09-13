package markdown

// inlineParser runs the inline phase on the lines of one block (design 6).
// It writes pieces to a scratch buffer, then appends them to the builder in
// one pass, so a later decision can change a piece that is already written.
type inlineParser struct {
	b     *builder
	src   []byte
	lines []pendingLine
	arena []prefixLeaf

	pieces []piece
	k      int    // the line being scanned
	start  uint32 // start of the first piece
}

// piece is a leaf in the scratch buffer. It starts at the end of the piece
// before it.
type piece struct {
	kind  Kind
	virt  uint8
	open  Kind // the span that opens before the leaf, or Document for none
	close bool // the innermost span closes after the leaf
	end   uint32
	owner uint32 // the owner of a prefix leaf
}

// inlines appends the inline content of lines: the Indent leaf of each line,
// its inlines, and a break and the prefix leaves before each later line. The
// prefix leaves of the first line and the line ending of the last line are
// the caller's.
func (s *inlineParser) inlines(lines []pendingLine) {
	s.lines, s.pieces, s.k = lines, s.pieces[:0], 0
	s.start = lines[0].rest.start
	s.startLine()
	for {
		l := s.lines[s.k].rest
		for i := s.end(); i < l.end; i = s.end() {
			switch s.src[i] {
			case '\\':
				if i+1 == l.end && s.k+1 < len(s.lines) {
					s.push(piece{kind: HardBreakMarker, open: HardBreak, end: i + 1})
					continue
				}
				s.text(i + 1)
			default:
				s.text(s.textEnd(i, l.end))
			}
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
	s.emit()
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

// textEnd returns the end of the run of bytes from i that no inline construct
// starts in, before end.
func (s *inlineParser) textEnd(i, end uint32) uint32 {
	for i++; i < end && s.src[i] != '\\'; i++ {
	}
	return i
}

// text pushes a Text piece to end, joined with a Text piece before it.
func (s *inlineParser) text(end uint32) {
	if n := len(s.pieces); n > 0 && s.pieces[n-1].kind == Text && !s.pieces[n-1].close {
		s.pieces[n-1].end = end
		return
	}
	s.push(piece{kind: Text, end: end})
}

func (s *inlineParser) push(x piece) {
	s.pieces = append(s.pieces, x)
}

// end returns the end of the last piece.
func (s *inlineParser) end() uint32 {
	return s.startOf(len(s.pieces))
}

// startOf returns the start of piece j.
func (s *inlineParser) startOf(j int) uint32 {
	if j == 0 {
		return s.start
	}
	return s.pieces[j-1].end
}

// emit appends the pieces to the builder, with their spans. Adjacent Text
// pieces become one leaf.
func (s *inlineParser) emit() {
	for j, x := range s.pieces {
		if x.open != Document {
			s.b.open(x.open)
		}
		if x.kind == Text && !x.close && j+1 < len(s.pieces) && s.pieces[j+1].kind == Text && s.pieces[j+1].open == Document {
			continue
		}
		s.b.split = x.virt
		if _, ok := x.kind.owner(); ok {
			s.b.prefix(x.kind, x.end, x.owner)
		} else {
			s.b.leaf(x.kind, x.end)
		}
		if x.close {
			s.b.close()
		}
	}
}
