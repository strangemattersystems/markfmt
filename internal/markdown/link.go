package markdown

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
