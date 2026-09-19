package markdown

import (
	"bytes"
	"strings"
	"unicode"
	"unicode/utf8"
)

// autolink pushes the angle autolink at i, on a line that ends at end, and
// reports whether there is one.
func (s *inlineParser) autolink(i, end uint32) bool {
	j := autolinkEnd(s.src, i, end, s.pipes)
	if j == 0 {
		return false
	}
	s.push(piece{kind: AngleBracket, open: Autolink, end: i + 1})
	s.pushContent(AutolinkText, j-1)
	s.push(piece{kind: AngleBracket, close: true, end: j})
	return true
}

// autolinkEnd returns the end of the angle autolink at src[i:end], which
// starts with '<', or 0: '<', an absolute URI or an email address, and '>'
// (CM 594 to 610). With pipes, a "\|" pair is a '|'.
func autolinkEnd(src []byte, i, end uint32, pipes bool) uint32 {
	for _, e := range [...]uint32{uriEnd(src, i+1, end), emailEnd(src, i+1, end, pipes)} {
		if e > 0 && e < end && src[e] == '>' {
			return e + 1
		}
	}
	return 0
}

// uriEnd returns the end of the absolute URI at src[i:end], or 0: a scheme of
// 2 to 32 characters, ':', and characters that are not U+0001 to U+001F,
// spaces, '<' or '>'. cmark takes DEL, which the spec text excludes.
func uriEnd(src []byte, i, end uint32) uint32 {
	if i == end || !isASCIILetter(src[i]) {
		return 0
	}
	j := i + 1
	for j < end && j-i <= 32 && (isASCIIAlphanumeric(src[j]) || src[j] == '+' || src[j] == '.' || src[j] == '-') {
		j++
	}
	if n := j - i; n < 2 || n > 32 || j == end || src[j] != ':' {
		return 0
	}
	// A NUL is U+FFFD, not a control character (spec 2.3).
	for j++; j < end && (src[j] > ' ' || src[j] == 0) && src[j] != '<' && src[j] != '>'; j++ {
	}
	return j
}

// emailEnd returns the end of the email address at src[i:end], or 0: a local
// part, '@', and labels of 1 to 63 ASCII letters, digits and '-', which start
// and end with a letter or digit, separated by '.'. With pipes, a "\|" pair
// is a '|' of the local part.
func emailEnd(src []byte, i, end uint32, pipes bool) uint32 {
	j := i
	for j < end {
		if pipes && src[j] == '\\' && j+1 < end && src[j+1] == '|' {
			j += 2
			continue
		}
		if !isASCIIAlphanumeric(src[j]) && strings.IndexByte(".!#$%&'*+/=?^_`{|}~-", src[j]) < 0 {
			break
		}
		j++
	}
	if j == i || j == end || src[j] != '@' {
		return 0
	}
	for {
		j++
		start := j
		if j == end || !isASCIIAlphanumeric(src[j]) {
			return 0
		}
		for j < end && j-start < 63 && (isASCIIAlphanumeric(src[j]) || src[j] == '-') {
			j++
		}
		if src[j-1] == '-' {
			return 0
		}
		if j == end || src[j] != '.' {
			return j
		}
	}
}

// AutolinkAngle reports whether autolink id is an angle autolink, and not an
// extended autolink.
func (t *Tree) AutolinkAngle(id NodeID) bool {
	return t.nodes[id+1].kind == AngleBracket
}

// AutolinkEmail reports whether autolink id is an email address. An absolute
// URI has a ':', and an email address has none. An extended autolink is a www
// autolink, a URL, or an email address, which has a ':' when it starts with
// "mailto:" or "xmpp:".
func (t *Tree) AutolinkEmail(id NodeID) bool {
	if t.AutolinkAngle(id) {
		m := t.nodes[id+2]
		return bytes.IndexByte(t.src[m.start:m.end], ':') < 0
	}
	text := t.AppendAutolinkText(nil, id)
	return !bytes.HasPrefix(text, []byte("www.")) && bytes.IndexByte(text, ':') < 0
}

// AppendAutolinkText appends the text of autolink id to dst: the values of its
// content leaves.
func (t *Tree) AppendAutolinkText(dst []byte, id NodeID) []byte {
	for i := id + 1; i < NodeID(t.nodes[id].link); i++ {
		if t.nodes[i].kind.class() == classContent {
			dst = t.AppendValue(dst, i)
		}
	}
	return dst
}

// appendEmails appends to dst the start and the end of each extended email
// autolink in text, the decoded text of a run, as cmark-gfm's postprocess_text
// finds them: an '@' after letters, digits, '.', '+', '-' and '_',
// which "mailto:" or "xmpp:" can precede when no letter or digit precedes it,
// then letters, digits, '-', '_', '/' after "xmpp:", and '.' before a letter or
// digit, with at least one such '.', ending with a letter or '.', and then
// autolinkDelim. An '@' in the domain restarts the search from it, with the
// protocol and the dots found so far.
func appendEmails(dst []uint32, text []byte) []uint32 {
	start, offset := 0, 0
	for offset < len(text)-start {
		i := bytes.IndexByte(text[start+offset:], '@')
		if i < 0 {
			break
		}
		maxRewind, xmpp, dots, found := i, false, 0, false
		var rewind, linkEnd int
	scan:
		for {
			at := start + offset + maxRewind
			if rewind = emailRewind(text, at, maxRewind, &xmpp); rewind == 0 {
				offset += maxRewind + 1
				break
			}
			for linkEnd = 1; linkEnd < len(text)-at; linkEnd++ {
				c := text[at+linkEnd]
				switch {
				case isASCIIAlphanumeric(c), c == '-', c == '_', c == '/' && xmpp:
					continue
				case c == '@':
					offset += maxRewind + 1
					maxRewind = linkEnd - 1
					continue scan
				case c == '.' && linkEnd < len(text)-at-1 && isASCIIAlphanumeric(text[at+linkEnd+1]):
					dots++
					continue
				}
				break
			}
			found = true
			break
		}
		if !found {
			continue
		}
		at := start + offset + maxRewind
		if last := text[at+linkEnd-1]; linkEnd < 2 || dots == 0 || !isASCIILetter(last) && last != '.' {
			offset += maxRewind + linkEnd
			continue
		}
		if linkEnd = int(autolinkDelim(text, count(at), count(at+linkEnd))) - at; linkEnd == 0 {
			offset += maxRewind + 1
			continue
		}
		dst = append(dst, count(at-rewind), count(at+linkEnd))
		start += offset + maxRewind + linkEnd
		offset = 0
	}
	return dst
}

// emailRewind returns how many of the maxRewind bytes before the '@' at
// text[at] belong to an email address: letters, digits, '.', '+', '-' and '_',
// and "mailto:" or "xmpp:" with no letter or digit before it. It sets *xmpp
// when it passes "xmpp:".
func emailRewind(text []byte, at, maxRewind int, xmpp *bool) int {
	for rewind := range maxRewind {
		switch c := text[at-rewind-1]; {
		case isASCIIAlphanumeric(c), strings.IndexByte(".+-_", c) >= 0,
			c == ':' && validProtocol(text, at, rewind, maxRewind, "mailto:"):
		case c == ':' && validProtocol(text, at, rewind, maxRewind, "xmpp:"):
			*xmpp = true
		default:
			return rewind
		}
	}
	return maxRewind
}

// validProtocol reports whether protocol, which ends with ':', ends before the
// rewind bytes before the '@' at text[at], within the maxRewind bytes before
// it, with no letter or digit before it, as cmark-gfm's validate_protocol
// checks.
func validProtocol(text []byte, at, rewind, maxRewind int, protocol string) bool {
	n := len(protocol)
	if n > maxRewind-rewind {
		return false
	}
	p := at - rewind - n
	return string(text[p:at-rewind]) == protocol && (n == maxRewind-rewind || !isASCIIAlphanumeric(text[p-1]))
}

// wwwAutolink pushes the extended www autolink at i, on a line that ends at
// end, and reports whether there is one, as cmark-gfm's www_match finds it: no
// bracket is open, the line starts at i or the byte before i is a space, '*',
// '_', '~' or '(', then "www." and a domain with a dot.
func (s *inlineParser) wwwAutolink(i, end uint32) bool {
	if len(s.brackets) > 0 || !bytes.HasPrefix(s.src[i:end], []byte("www.")) ||
		i > s.lineStart && !isSpaceChar(s.src[i-1]) && strings.IndexByte("*_~(", s.src[i-1]) < 0 {
		return false
	}
	j, ok := domainEnd(s.src, i, end, s.contentEnd, false)
	if !ok {
		return false
	}
	k := autolinkDelim(s.src, i, linkEnd(s.src, j, end))
	if k == i {
		return false
	}
	s.pushAutolink(k)
	return true
}

// urlAutolink pushes the extended URL autolink whose ':' is at i, on a line
// that ends at end, and reports whether there is one, as cmark-gfm's url_match
// finds it: no bracket is open, the letters before i are http, https or ftp in
// any case, then "://", a host character and a domain. The last [Text] piece
// gives the letters back.
func (s *inlineParser) urlAutolink(i, end uint32) bool {
	if len(s.brackets) > 0 || end-i < 4 || s.src[i+1] != '/' || s.src[i+2] != '/' {
		return false
	}
	start := i
	for start > s.lineStart && isASCIILetter(s.src[start-1]) {
		start--
	}
	scheme := s.src[start:i]
	if !bytes.EqualFold(scheme, []byte("http")) && !bytes.EqualFold(scheme, []byte("https")) && !bytes.EqualFold(scheme, []byte("ftp")) ||
		!isHostChar(s.src, i+3, end) {
		return false
	}
	j, ok := domainEnd(s.src, i+3, end, s.contentEnd, true)
	if !ok {
		return false
	}
	k := autolinkDelim(s.src, i, linkEnd(s.src, j, end))
	// The letters of a scheme follow a byte that is not a letter, which ends a
	// Text piece only when a piece that is not Text ends there.
	n := len(s.pieces) - 1
	if k == i || n < 0 || s.pieces[n].kind != Text || s.pieces[n].held || s.pieces[n].close || s.startOf(n) > start {
		return false
	}
	if s.startOf(n) == start {
		s.pieces = s.pieces[:n]
	} else {
		s.pieces[n].end = start
	}
	s.pushAutolink(k)
	return true
}

// pushAutolink pushes an extended autolink from the end of the last piece to
// end. Its text is not decoded, so it is [Text] pieces, and [CellPipeEscape]
// pieces with pipes.
func (s *inlineParser) pushAutolink(end uint32) {
	n := len(s.pieces)
	s.pushContent(Text, end)
	s.pieces[n].open = Autolink
	s.pieces[len(s.pieces)-1].close = true
}

// isSpaceChar reports whether c is a space, a tab, a line feed, a vertical
// tab, a form feed or a carriage return, as cmark's isspace does.
func isSpaceChar(c byte) bool {
	return c == ' ' || '\t' <= c && c <= '\r'
}

// isHostChar reports whether the character that starts at src[i], before end,
// is valid in a domain, as cmark-gfm's is_valid_hostchar reads it: a whole
// UTF-8 character that is not a Unicode space and not punctuation, which is
// ASCII punctuation or Unicode P. A byte inside a character is not valid.
func isHostChar(src []byte, i, end uint32) bool {
	r, n := utf8.DecodeRune(src[i:end])
	switch {
	case r == utf8.RuneError && n <= 1:
		return false
	case r < utf8.RuneSelf:
		return !isASCIIPunct(src[i]) && !isUnicodeSpace(r)
	}
	return !unicode.IsPunct(r) && !isUnicodeSpace(r)
}

// domainEnd returns the end of the domain that starts at src[start], on a line
// that ends at end, in a block whose content ends at limit, and whether there
// is one, as cmark-gfm's check_domain reads it. The scan starts after start,
// reads the byte after a '\', and stops at a byte that is not '-', '_', '.' or
// a host character, or at the last byte of the content, which it never reads.
// '_' in one of the last two segments rejects a domain with at most 10 dots,
// and a domain that is not short needs a dot.
func domainEnd(src []byte, start, end, limit uint32, short bool) (uint32, bool) {
	var dots, under1, under2 int
	j := start + 1
scan:
	for ; j < end && j+1 < limit; j++ {
		if src[j] == '\\' && j+2 < limit {
			if j++; j == end {
				break
			}
		}
		switch c := src[j]; {
		case c == '_':
			under2++
		case c == '.':
			under1, under2 = under2, 0
			dots++
		case c == 0:
			// cmark-gfm reads NUL as U+FFFD, and stops at its second byte.
			j++
			break scan
		case c != '-' && !isHostChar(src, j, end):
			break scan
		}
	}
	if (under1 > 0 || under2 > 0) && dots <= 10 {
		return 0, false
	}
	return j, short || dots > 0
}

// linkEnd returns the first space or '<' at or after j, before end.
func linkEnd(src []byte, j, end uint32) uint32 {
	for j < end && !isSpaceChar(src[j]) && src[j] != '<' {
		j++
	}
	return j
}

// autolinkDelim returns the end of the extended autolink in src[start:end] after
// cmark-gfm's autolink_delim: the link ends before a '<', and loses a trailing
// '?', '!', '.', ',', ':', '*', '_', '~', an apostrophe or '"', a trailing ')'
// while it has more ')' than '(', and a trailing "&letters;" or ';'.
func autolinkDelim(src []byte, start, end uint32) uint32 {
	opening, closing := 0, 0
	for i := start; i < end; i++ {
		if src[i] == '<' {
			end = i
			break
		}
		switch src[i] {
		case '(':
			opening++
		case ')':
			closing++
		}
	}
	for end > start {
		switch src[end-1] {
		case ')':
			if closing <= opening {
				return end
			}
			closing--
			end--
		case '?', '!', '.', ',', ':', '*', '_', '~', '\'', '"':
			end--
		case ';':
			k := end - 1
			for k > start+1 && isASCIILetter(src[k-1]) {
				k--
			}
			if k < end-1 && src[k-1] == '&' {
				end = k - 1
			} else {
				end--
			}
		default:
			return end
		}
	}
	return end
}
