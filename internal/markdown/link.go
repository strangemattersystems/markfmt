package markdown

// bracket is a link or image opener on the bracket stack (design 6.3).
type bracket struct {
	piece   int // the opener piece: the '[', or the '!' of an image
	bottom  int // the top of the delimiter stack when the opener was pushed
	seq     int // the opener's push sequence number
	closers int // the ']' that closeBracket handled before the opener was pushed
	image   bool
}

func (b bracket) text() int {
	if b.image {
		return b.piece + 2
	}
	return b.piece + 1
}

// openBracket pushes the '[' at i, or the '![' when image is true, as a bracket
// and held pieces: the '!' and the '[' of an image, which a footnote reference
// splits, and a '^' after the '[', which a footnote reference makes its caret
// (design 6.3).
func (s *inlineParser) openBracket(i uint32, image bool) {
	s.seq++
	s.brackets = append(s.brackets, bracket{piece: len(s.pieces), bottom: len(s.delims) - 1, seq: s.seq, closers: s.closers, image: image})
	if image {
		s.push(piece{kind: Text, held: true, end: i + 1})
		i++
	}
	s.push(piece{kind: Text, held: true, end: i + 1})
	if i+1 < s.lines[s.k].rest.end && s.src[i+1] == '^' {
		s.push(piece{kind: Text, held: true, end: i + 2})
	}
}

// closeBracket pushes the ']' at i, on a line that ends at end, with the
// inline link or image that it closes, or as text (CM 482 to 526). A link
// makes every link opener before it inactive in O(1): an opener pushed before
// the last link's opener is inactive (design 6.3).
func (s *inlineParser) closeBracket(i, end uint32) {
	n := len(s.brackets)
	if n == 0 {
		s.text(i + 1)
		return
	}
	b := s.brackets[n-1]
	s.brackets = s.brackets[:n-1]
	inner := s.closers > b.closers
	s.closers++
	if b.image || b.seq > s.linkFormed {
		m := s.mark()
		s.push(piece{kind: Bracket, end: i + 1})
		form, ok := InlineLink, false
		if i+1 < end && s.src[i+1] == '(' {
			tail := s.mark()
			if ok = s.linkTail(); !ok {
				s.reset(tail)
			}
		}
		if !ok {
			form, ok = s.reference(b)
		}
		if ok {
			kind := Image
			if !b.image {
				kind, s.linkFormed = Link, b.seq
			}
			opener := &s.pieces[b.piece]
			opener.kind, opener.open, opener.flags = Bracket, kind, uint8(form)
			if b.image {
				s.pieces[b.piece+1].kind, s.pieces[b.piece+1].join = Bracket, true
			}
			s.pieces[len(s.pieces)-1].close = true
			s.processEmphasis(b.bottom)
			return
		}
		s.reset(m)
		if s.footnoteReference(b, i, inner) {
			return
		}
	}
	s.text(i + 1)
}

// footnoteNote is a footnote reference that the scan completed: its opener
// and closer pieces, and its label bytes inside the label of another reference.
type footnoteNote struct {
	open, close, size int
}

// footnoteReference makes the bracket text of opener b, which the ']' at i
// closes, a footnote reference, and reports whether it did: the text decodes
// to '^' and more (design 6.3 step 4). The delimiters inside it go, and its
// pieces become label pieces; a reference that completed inside it is passed
// in one step. It resolves when no ']' was handled inside it (inner is false),
// its label has at most 1,000 label bytes, and its normalized label is a
// footnote label (design 6.7).
func (s *inlineParser) footnoteReference(b bracket, i uint32, inner bool) bool {
	open := b.text() - 1
	caret := open + 1
	if caret+1 >= len(s.pieces) || !s.isCaret(caret) {
		return false
	}
	s.delims = s.delims[:b.bottom+1]
	if b.bottom >= 0 {
		s.delims[b.bottom].next = -1
	}
	k := len(s.notes)
	for k > 0 && s.notes[k-1].open > open {
		k--
	}
	f := labelFolder{dst: s.label[:0]}
	size, note, prev := 0, k, Caret
	for j := caret + 1; j < len(s.pieces); j++ {
		if note < len(s.notes) && s.notes[note].open == j {
			n := s.notes[note]
			for _, p := range [...]int{n.open, n.open + 1, n.open + 2, n.close} {
				if x := &s.pieces[p]; p != n.open+2 || x.kind == FootnoteLabel {
					*x = piece{kind: FootnoteLabel, join: s.pieces[p-1].kind == FootnoteLabel, end: x.end}
				}
			}
			size += n.size
			j, note, prev = n.close, note+1, FootnoteLabel
			continue
		}
		x := &s.pieces[j]
		bytes := s.src[s.startOf(j):x.end]
		kind, flags := x.kind, uint8(0)
		switch _, prefix := kind.owner(); {
		case prefix, kind == Indent:
		case kind == LineEnding, kind == VerbatimLineEnding:
			kind = VerbatimLineEnding
			size++
		case kind == CellPipeEscape:
			flags = uint8(FootnoteLabel)
			size += len(bytes) - 1
		default:
			kind = FootnoteLabel
			size += len(bytes)
		}
		if !inner && size <= 1000 {
			f.leaf(kind, bytes)
		}
		*x = piece{kind: kind, virt: x.virt, flags: flags, join: kind == FootnoteLabel && prev == FootnoteLabel, end: x.end, owner: x.owner}
		prev = kind
	}
	s.label = f.dst
	caretSize := int(s.pieces[caret].end - s.startOf(caret))
	s.pieces[open] = piece{kind: Bracket, open: FootnoteReference, end: s.pieces[open].end}
	if !inner && size <= 1000 && len(f.dst) > 0 && s.defs.footnoteDefined[string(f.dst)] {
		s.pieces[open].flags = 1
	}
	s.pieces[caret] = piece{kind: Caret, end: s.pieces[caret].end}
	s.push(piece{kind: Bracket, close: true, end: i + 1})
	s.notes = append(s.notes[:k], footnoteNote{open: open, close: len(s.pieces) - 1, size: size + caretSize + 2})
	return true
}

func (s *inlineParser) isCaret(j int) bool {
	b := s.src[s.startOf(j):s.pieces[j].end]
	switch s.pieces[j].kind {
	case Text:
		return string(b) == "^"
	case Escape:
		return b[1] == '^'
	case EntityRef:
		return string(appendEntityValue(s.entity[:0], b)) == "^"
	}
	return false
}

// reference pushes the label of the reference link or image that the last
// piece, a ']', closes for opener b, and returns its form: a full reference
// whose label is defined, or a collapsed or shortcut reference whose bracket
// text is a defined label (CM 527 to 571). A bracket text followed by a label
// that is not blank is never a shortcut reference (CM 569). A blank label
// makes a collapsed reference, as cmark reads it. With no definitions, no
// lookup runs.
func (s *inlineParser) reference(b bracket) (LinkForm, bool) {
	if len(s.defs.labels) == 0 {
		return InlineLink, false
	}
	closer := len(s.pieces) - 1
	m := s.mark()
	form := CollapsedReference
	switch found, blank := s.linkLabel(); {
	case found && !blank:
		return FullReference, s.defined(m.pieces+1, len(s.pieces)-1)
	case !found:
		s.reset(m)
		form = ShortcutReference
	}
	// A bracket text with an unescaped bracket is not a label, and a bracket
	// pushed after the opener shows one in O(1) (CM 546 to 548).
	return form, b.seq == s.seq && s.defined(b.text(), closer)
}

// labelBytes returns the bytes of piece j that count toward the size of a
// label: none for indentation and prefix leaves, which are not content, one
// byte for a line ending, and one byte less for a cell pipe escape, whose
// pair writes one byte (design 6.7). Only the byte count and the character
// count of the result are used, not its bytes.
func (s *inlineParser) labelBytes(j int) []byte {
	b := s.src[s.startOf(j):s.pieces[j].end]
	switch k := s.pieces[j].kind; k {
	case Indent:
		return nil
	case LineEnding, VerbatimLineEnding:
		return lineFeed[:1:1]
	case CellPipeEscape:
		return b[:len(b)-1]
	default:
		if _, prefix := k.owner(); prefix {
			return nil
		}
		return b
	}
}

// defined reports whether the label bytes of the pieces from index from to
// index to, normalized, are a defined label. The label cap is checked before
// the label is read: at most 999 bytes, or at most 3,996 bytes and 999
// characters (design 6.7).
func (s *inlineParser) defined(from, to int) bool {
	size := 0
	for j := from; j < to; j++ {
		size += len(s.labelBytes(j))
	}
	if size > 3996 {
		return false
	}
	if size > 999 {
		chars := 0
		for j := from; j < to; j++ {
			for _, c := range s.labelBytes(j) {
				if c&0xC0 != 0x80 {
					chars++
				}
			}
		}
		if chars > 999 {
			return false
		}
	}
	f := labelFolder{dst: s.label[:0], link: true}
	for j := from; j < to; j++ {
		f.leaf(s.pieces[j].kind, s.src[s.startOf(j):s.pieces[j].end])
	}
	s.label = f.dst
	return len(f.dst) > 0 && s.defs.defined[string(f.dst)]
}

// LinkForm is the form of a link or an image (design 10.2).
type LinkForm uint8

const (
	InlineLink LinkForm = iota
	FullReference
	CollapsedReference
	ShortcutReference
)

// LinkForm returns the form of link or image id.
func (t *Tree) LinkForm(id NodeID) LinkForm {
	return LinkForm(t.nodes[id].flags)
}

// AppendLinkLabel appends the normalized label of reference link or image id
// to dst: the label of a full reference, or the bracket text of a collapsed
// or shortcut reference (design 6.7).
func (t *Tree) AppendLinkLabel(dst []byte, id NodeID) []byte {
	f := labelFolder{dst: dst, start: len(dst), link: true}
	first := 1 // the label follows this many of the link's own brackets
	if t.LinkForm(id) == FullReference {
		first = 3
	}
	brackets, nested := 0, uint32(0)
	for i := uint32(id) + 1; i < t.nodes[id].link; i++ {
		switch m := t.nodes[i]; {
		case m.kind.class() == classStructure:
			// A structure before the label holds none of its bytes. Reading it
			// again for each nested reference costs O(n^2).
			if brackets < first {
				i = m.link - 1
				continue
			}
			nested = max(nested, m.link)
		case m.kind == Bracket && i >= nested:
			if brackets++; brackets > first {
				return f.dst
			}
		case brackets == first:
			f.leaf(m.kind, t.src[m.start:m.end])
		}
	}
	return f.dst
}

// linkTail pushes the inline link tail at the position: '(', optional
// whitespace, an optional destination, a title after whitespace, optional
// whitespace, and ')'. It reports whether there is one.
func (s *inlineParser) linkTail() bool {
	s.push(piece{kind: Paren, end: s.end() + 1})
	s.spaceLine()
	if _, ok := s.expect(pos{s.k, s.end()}, ')'); !ok {
		if !s.linkDestination() {
			return false
		}
		spaced := s.spaceLine()
		m := s.mark()
		if spaced && s.linkTitle() {
			s.spaceLine()
		} else {
			s.reset(m)
		}
		if _, ok := s.expect(pos{s.k, s.end()}, ')'); !ok {
			return false
		}
	}
	s.push(piece{kind: Paren, end: s.end() + 1})
	return true
}

// linkLabel pushes the link label at the position: '[', up to 999 characters
// with no unescaped bracket and at least one that is not a space, tab or line
// ending, and ']' (design 6.7). It reports whether there is one, and whether
// it is blank: it has only spaces, tabs, VT, FF and line endings, as cmark
// reads it.
func (s *inlineParser) linkLabel() (found, blank bool) {
	j := s.end()
	if c, ok := s.byteAt(pos{s.k, j}); !ok || c != '[' {
		return false, false
	}
	s.push(piece{kind: Bracket, end: j + 1})
	chars := 0
	blank = true
	for j++; chars <= 999; j++ {
		end := s.lines[s.k].rest.end
		if j == end {
			s.pushContent(LinkLabel, j)
			if !s.nextLine(VerbatimLineEnding) {
				return false, false
			}
			j, chars = s.end()-1, chars+1
			continue
		}
		switch c := s.src[j]; {
		case c == ']':
			s.pushContent(LinkLabel, j)
			s.push(piece{kind: Bracket, end: j + 1})
			return true, blank
		case c == '[':
			return false, false
		case c == '\\' && j+1 < end && isASCIIPunct(s.src[j+1]):
			j++
			chars++
			blank = false
		case !isSpaceOrTab(c) && c != '\v' && c != '\f':
			blank = false
		}
		if s.src[j]&0xC0 != 0x80 {
			chars++
		}
	}
	return false, false
}

// linkDestination pushes the link destination at the position: '<',
// characters with no unescaped '<' or '>', and '>' on one line; or characters
// that are not spaces or ASCII control characters, with parentheses balanced
// and nested at most 32 deep. It reports whether there is one.
func (s *inlineParser) linkDestination() bool {
	j, end := s.end(), s.lines[s.k].rest.end
	if j == end {
		return false
	}
	if s.src[j] == '<' {
		s.push(piece{kind: AngleBracket, end: j + 1})
		for j++; j < end; j++ {
			switch s.src[j] {
			case '>':
				s.pushContent(Destination, j)
				s.push(piece{kind: AngleBracket, end: j + 1})
				return true
			case '<':
				return false
			case '\\':
				if j+1 < end && isASCIIPunct(s.src[j+1]) {
					j++
				}
			}
		}
		return false
	}
	start, depth := j, 0
	for ; j < end; j++ {
		c := s.src[j]
		if c == '\\' && j+1 < end && isASCIIPunct(s.src[j+1]) {
			j++
			continue
		}
		// cmark ends a destination at these and takes every other control
		// character, which the spec text excludes (design 8.4).
		if c == ' ' || c == '\t' || c == '\v' || c == '\f' || c == ')' && depth == 0 {
			break
		}
		switch c {
		case '(':
			if depth++; depth > 32 {
				return false
			}
		case ')':
			depth--
		}
	}
	if j == start || depth > 0 {
		return false
	}
	s.pushContent(Destination, j)
	return true
}

// linkTitle pushes the link title at the position, over any number of lines:
// characters between two double quotes, between two apostrophes, or between
// '(' and ')' with no unescaped '('. It reports whether there is one.
func (s *inlineParser) linkTitle() bool {
	j := s.end()
	c, ok := s.byteAt(pos{s.k, j})
	closing := c
	switch {
	case !ok:
		return false
	case c == '(':
		closing = ')'
	case c != '"' && c != '\'':
		return false
	}
	s.push(piece{kind: TitleQuote, end: j + 1})
	for j++; ; j++ {
		end := s.lines[s.k].rest.end
		if j == end {
			s.pushContent(Title, j)
			if !s.nextLine(VerbatimLineEnding) {
				return false
			}
			j = s.end() - 1
			continue
		}
		switch b := s.src[j]; {
		case b == closing:
			s.pushContent(Title, j)
			s.push(piece{kind: TitleQuote, end: j + 1})
			return true
		case b == '(' && c == '(':
			return false
		case b == '\\' && j+1 < end && isASCIIPunct(s.src[j+1]):
			j++
		}
	}
}

// spaceLine pushes the spaces and tabs at the position, up to one line
// ending, and the prefix and [Indent] leaves of the next line. It reports
// whether it pushed any.
func (s *inlineParser) spaceLine() bool {
	n := len(s.pieces)
	if !s.lineEnd() || !s.nextLine(LineEnding) {
		return len(s.pieces) > n
	}
	return true
}

// lineEnd pushes the spaces and tabs at the position, and reports whether
// the line ends after them.
func (s *inlineParser) lineEnd() bool {
	j, end := s.end(), s.lines[s.k].rest.end
	for j < end && isSpaceOrTab(s.src[j]) {
		j++
	}
	s.pushIf(Whitespace, j)
	return j == end
}

// AppendDestination appends the value of the destination of link, image or
// link reference definition id to dst.
func (t *Tree) AppendDestination(dst []byte, id NodeID) []byte {
	for i := uint32(id) + 1; i < t.nodes[id].link; i++ {
		switch m := t.nodes[i]; {
		case m.kind.class() == classStructure:
			i = m.link - 1
		case m.kind == Destination, m.kind == CellPipeEscape && Kind(m.flags) == Destination:
			dst = t.AppendValue(dst, NodeID(i))
		}
	}
	return dst
}

// AppendTitle appends the value of the title of link, image or link
// reference definition id to dst.
func (t *Tree) AppendTitle(dst []byte, id NodeID) []byte {
	quotes := 0
	for i := uint32(id) + 1; i < t.nodes[id].link; i++ {
		switch m := t.nodes[i]; {
		case m.kind.class() == classStructure:
			i = m.link - 1
		case m.kind == TitleQuote:
			quotes++
		case quotes == 1 && (m.kind == Title || m.kind == VerbatimLineEnding || m.kind == CellPipeEscape):
			dst = t.AppendValue(dst, NodeID(i))
		}
	}
	return dst
}
