package markdown

import (
	"bytes"
	"strings"
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
// (CM 594 to 610). With pipes, a "\|" pair is a '|' (design 8.2).
func autolinkEnd(src []byte, i, end uint32, pipes bool) uint32 {
	for _, e := range [...]uint32{uriEnd(src, i+1, end), emailEnd(src, i+1, end, pipes)} {
		if e > 0 && e < end && src[e] == '>' {
			return e + 1
		}
	}
	return 0
}

// uriEnd returns the end of the absolute URI at src[i:end], or 0: a scheme of
// 2 to 32 characters, ':', and characters that are not ASCII control
// characters, spaces, '<' or '>'.
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
	for j++; j < end && src[j] > ' ' && src[j] != '<' && src[j] != '>' && src[j] != 0x7f; j++ {
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

// AutolinkEmail reports whether autolink id is an email address. An
// absolute URI has a ':', and an email address has none.
func (t *Tree) AutolinkEmail(id NodeID) bool {
	m := t.nodes[id+2]
	return bytes.IndexByte(t.src[m.start:m.end], ':') < 0
}
