package markdown

// bracket is a link or image opener on the bracket stack (design 6.3).
type bracket struct {
	piece  int // the opener piece
	bottom int // the top of the delimiter stack when the opener was pushed
	seq    int // the opener's push sequence number
	image  bool
}

// openBracket pushes the '[' at i, or the '![' when image is true, as a piece
// and a bracket.
func (s *inlineParser) openBracket(i uint32, image bool) {
	end := i + 1
	if image {
		end++
	}
	s.seq++
	s.brackets = append(s.brackets, bracket{piece: len(s.pieces), bottom: len(s.delims) - 1, seq: s.seq, image: image})
	s.push(piece{kind: Text, held: true, end: end})
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
			s.pieces[len(s.pieces)-1].close = true
			s.processEmphasis(b.bottom)
			return
		}
		s.reset(m)
	}
	s.text(i + 1)
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
	return form, b.seq == s.seq && s.defined(b.piece+1, closer)
}

// defined reports whether the label bytes of the pieces from index from to
// index to, normalized, are a defined label. The label cap is checked before
// the label is read: at most 999 bytes, or at most 3,996 bytes and 999
// characters (design 6.7).
func (s *inlineParser) defined(from, to int) bool {
	size := 0
	for j := from; j < to; j++ {
		switch k := s.pieces[j].kind; k {
		case Indent:
		case LineEnding, VerbatimLineEnding:
			size++
		default:
			if _, prefix := k.owner(); !prefix {
				size += int(s.pieces[j].end - s.startOf(j))
			}
		}
	}
	if size > 3996 {
		return false
	}
	if size > 999 {
		chars := 0
		for j := from; j < to; j++ {
			for _, c := range s.src[s.startOf(j):s.pieces[j].end] {
				if c&0xC0 != 0x80 {
					chars++
				}
			}
		}
		if chars > 999 {
			return false
		}
	}
	f := labelFolder{dst: s.label[:0]}
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
	f := labelFolder{dst: dst, start: len(dst)}
	first := 1 // the label follows this many of the link's own brackets
	if t.LinkForm(id) == FullReference {
		first = 3
	}
	brackets, nested := 0, uint32(0)
	for i := uint32(id) + 1; i < t.nodes[id].link; i++ {
		switch m := t.nodes[i]; {
		case m.kind.class() == classStructure:
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
// it is blank: it has only spaces, tabs and line endings.
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
			s.pushIf(LinkLabel, j)
			if !s.nextLine(VerbatimLineEnding) {
				return false, false
			}
			j, chars = s.end()-1, chars+1
			continue
		}
		switch c := s.src[j]; {
		case c == ']':
			s.pushIf(LinkLabel, j)
			s.push(piece{kind: Bracket, end: j + 1})
			return true, blank
		case c == '[':
			return false, false
		case c == '\\' && j+1 < end && isASCIIPunct(s.src[j+1]):
			j++
			chars++
			blank = false
		case !isSpaceOrTab(c):
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
				s.pushIf(Destination, j)
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
		if c <= ' ' || c == 0x7f || c == ')' && depth == 0 {
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
	s.push(piece{kind: Destination, end: j})
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
			s.pushIf(Title, j)
			if !s.nextLine(VerbatimLineEnding) {
				return false
			}
			j = s.end() - 1
			continue
		}
		switch b := s.src[j]; {
		case b == closing:
			s.pushIf(Title, j)
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
// ending, and the prefix and Indent leaves of the next line. It reports
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
		case m.kind == Destination:
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
		case quotes == 1 && (m.kind == Title || m.kind == VerbatimLineEnding):
			dst = t.AppendValue(dst, NodeID(i))
		}
	}
	return dst
}
