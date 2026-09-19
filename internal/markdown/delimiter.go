package markdown

import (
	"unicode"
	"unicode/utf8"
)

// delimiter is a run of '*', '_' or '~' on the delimiter stack. The run has
// one piece per character. Closers use its characters from the left, and
// openers from the right.
type delimiter struct {
	piece             int // the piece of the first character
	used, left        int // characters that closers used, and characters not used
	length            int
	char              byte
	canOpen, canClose bool
	prev, next        int // neighbours on the stack, or -1
}

// delimiterRun pushes the run of '*', '_' or '~' at i, on a line that ends at
// end: one piece per character, and a delimiter when the run can open or close
// emphasis (CM 350 to 363) or strikethrough. Only a run of one or two tildes is
// a delimiter (cmark-gfm).
func (s *inlineParser) delimiterRun(i, end uint32) {
	c, j := s.src[i], i+1
	for j < end && s.src[j] == c {
		j++
	}
	before, after := ' ', ' '
	if i > s.lineStart {
		before, _ = utf8.DecodeLastRune(s.src[s.lineStart:i])
	}
	if j < end {
		after, _ = decodeRune(s.src[j:end])
	}
	if before == 0 {
		before = utf8.RuneError
	}
	left := !isUnicodeSpace(after) && (!isUnicodePunct(after) || isUnicodeSpace(before) || isUnicodePunct(before))
	right := !isUnicodeSpace(before) && (!isUnicodePunct(before) || isUnicodeSpace(after) || isUnicodePunct(after))
	canOpen, canClose := left, right
	if c == '_' {
		canOpen = left && (!right || isUnicodePunct(before))
		canClose = right && (!left || isUnicodePunct(after))
	}
	if !canOpen && !canClose || c == '~' && j-i > 2 {
		s.text(j)
		return
	}
	n := len(s.delims)
	if n > 0 {
		s.delims[n-1].next = n
	}
	s.delims = append(s.delims, delimiter{
		piece: len(s.pieces), left: int(j - i), length: int(j - i), char: c,
		canOpen: canOpen, canClose: canClose, prev: n - 1, next: -1,
	})
	for ; i < j; i++ {
		s.push(piece{kind: Text, held: true, end: i + 1})
	}
}

// processEmphasis matches the delimiters above index bottom into emphasis,
// strong emphasis and strikethrough, and removes them from the stack: the
// "process emphasis" procedure of the CommonMark spec. The bound on the
// search for an opener, by character, closer length modulo 3 and whether the
// closer can open, keeps it linear. cmark-gfm bounds a '~' closer by its
// length modulo 3 only.
func (s *inlineParser) processEmphasis(bottom int) {
	var openersBottom [2][6]int
	tildesBottom := [3]int{bottom, bottom, bottom}
	for c := range openersBottom {
		for k := range openersBottom[c] {
			openersBottom[c][k] = bottom
		}
	}
	cur := -1
	if bottom+1 < len(s.delims) {
		cur = bottom + 1
	}
	for cur >= 0 {
		closer := &s.delims[cur]
		if !closer.canClose {
			cur = closer.next
			continue
		}
		c, k := 0, closer.length%3
		if closer.char == '_' {
			c = 1
		}
		if closer.canOpen {
			k += 3
		}
		lowest := &openersBottom[c][k]
		if closer.char == '~' {
			lowest = &tildesBottom[closer.length%3]
		}
		o := closer.prev
		for o > *lowest && !s.opens(&s.delims[o], closer) {
			o = s.delims[o].prev
		}
		if o <= *lowest {
			*lowest = max(*lowest, closer.prev)
			next := closer.next
			if !closer.canOpen {
				s.unlink(cur)
			}
			cur = next
			continue
		}
		opener := &s.delims[o]
		if closer.char == '~' {
			s.strike(opener, closer)
		} else {
			s.match(opener, closer)
		}
		for d := closer.prev; d != o; d = s.delims[d].prev {
			s.unlink(d)
		}
		if opener.left == 0 {
			s.unlink(o)
		}
		if closer.left == 0 {
			next := closer.next
			s.unlink(cur)
			cur = next
		}
	}
	s.delims = s.delims[:bottom+1]
	if bottom >= 0 {
		s.delims[bottom].next = -1
	}
}

// opens reports whether delimiter d can open emphasis that closer closes.
// When either can both open and close, the run lengths must not sum to a
// multiple of 3, unless both are multiples of 3 (CM 411, 412).
func (s *inlineParser) opens(d, closer *delimiter) bool {
	return d.canOpen && d.char == closer.char &&
		(!d.canClose && !closer.canOpen || (d.length+closer.length)%3 != 0 || d.length%3 == 0 && closer.length%3 == 0)
}

// match makes strong emphasis from two characters of opener and closer when
// both have two left, and emphasis from one otherwise.
func (s *inlineParser) match(opener, closer *delimiter) {
	use, kind := 1, Emphasis
	if opener.left >= 2 && closer.left >= 2 {
		use, kind = 2, Strong
	}
	a := opener.piece + opener.used + opener.left - use
	b := closer.piece + closer.used
	for k := range use {
		s.pieces[a+k].kind, s.pieces[a+k].join = Delimiter, k > 0
		s.pieces[b+k].kind, s.pieces[b+k].join = Delimiter, k > 0
	}
	s.pieces[a].open = kind
	s.pieces[b+use-1].close = true
	opener.left -= use
	closer.left, closer.used = closer.left-use, closer.used+use
}

// strike makes strikethrough from the '~' runs of opener and closer when their
// lengths are equal, and leaves both as text otherwise. Either way it uses all
// their characters.
func (s *inlineParser) strike(opener, closer *delimiter) {
	if opener.length == closer.length {
		for k := range opener.length {
			s.pieces[opener.piece+k].kind, s.pieces[opener.piece+k].join = Delimiter, k > 0
			s.pieces[closer.piece+k].kind, s.pieces[closer.piece+k].join = Delimiter, k > 0
		}
		s.pieces[opener.piece].open = Strikethrough
		s.pieces[closer.piece+closer.length-1].close = true
	}
	opener.left, closer.left = 0, 0
}

// unlink removes delimiter d from the stack. Its characters that no closer or
// opener used stay text.
func (s *inlineParser) unlink(d int) {
	x := s.delims[d]
	if x.prev >= 0 {
		s.delims[x.prev].next = x.next
	}
	if x.next >= 0 {
		s.delims[x.next].prev = x.prev
	}
}

// isUnicodeSpace reports whether r is Unicode whitespace for flanking: Zs,
// tab, line feed, form feed or carriage return.
func isUnicodeSpace(r rune) bool {
	return r == '\t' || r == '\n' || r == '\f' || r == '\r' || unicode.Is(unicode.Zs, r)
}

// isUnicodePunct reports whether r is Unicode punctuation for flanking: P or
// S. U+FFFD is So (testdata/dialect.md).
func isUnicodePunct(r rune) bool {
	return unicode.IsPunct(r) || unicode.IsSymbol(r)
}
