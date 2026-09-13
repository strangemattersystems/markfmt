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
		if i+1 < end && s.src[i+1] == '(' && s.linkTail() {
			kind := Image
			if !b.image {
				kind, s.linkFormed = Link, b.seq
			}
			s.pieces[b.piece].kind, s.pieces[b.piece].open = Bracket, kind
			s.pieces[len(s.pieces)-1].close = true
			s.processEmphasis(b.bottom)
			return
		}
		s.reset(m)
	}
	s.text(i + 1)
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
// ending, and ']' (design 6.7). It reports whether there is one.
func (s *inlineParser) linkLabel() bool {
	j := s.end()
	if c, ok := s.byteAt(pos{s.k, j}); !ok || c != '[' {
		return false
	}
	s.push(piece{kind: Bracket, end: j + 1})
	chars, blank := 0, true
	for j++; chars <= 999; j++ {
		end := s.lines[s.k].rest.end
		if j == end {
			s.pushIf(LinkLabel, j)
			if !s.nextLine(VerbatimLineEnding) {
				return false
			}
			j, chars = s.end()-1, chars+1
			continue
		}
		switch c := s.src[j]; {
		case c == ']':
			s.pushIf(LinkLabel, j)
			s.push(piece{kind: Bracket, end: j + 1})
			return !blank
		case c == '[':
			return false
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
	return false
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
