package markdown

import (
	"bytes"
	"strings"
)

// kind1Names are the tag names of HTML block kind 1.
var kind1Names = []string{"pre", "script", "style", "textarea"}

// kind6Names are the tag names of HTML block kind 6 in CommonMark 0.31.2.
// GitHub's list has "source" and not "search" (testdata/dialect.md).
var kind6Names = map[string]bool{
	"address": true, "article": true, "aside": true, "base": true, "basefont": true,
	"blockquote": true, "body": true, "caption": true, "center": true, "col": true,
	"colgroup": true, "dd": true, "details": true, "dialog": true, "dir": true,
	"div": true, "dl": true, "dt": true, "fieldset": true, "figcaption": true,
	"figure": true, "footer": true, "form": true, "frame": true, "frameset": true,
	"h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
	"head": true, "header": true, "hr": true, "html": true, "iframe": true,
	"legend": true, "li": true, "link": true, "main": true, "menu": true,
	"menuitem": true, "nav": true, "noframes": true, "ol": true, "optgroup": true,
	"option": true, "p": true, "param": true, "search": true, "section": true,
	"summary": true, "table": true, "tbody": true, "td": true, "tfoot": true,
	"th": true, "thead": true, "title": true, "tr": true, "track": true, "ul": true,
}

// htmlBlockStart returns the kind, from 1 to 7, of the HTML block whose start
// condition the line src[i:end] meets after its indentation, or 0.
func htmlBlockStart(src []byte, i, end uint32) uint8 {
	s := src[i:end]
	if len(s) < 2 || s[0] != '<' {
		return 0
	}
	for _, name := range kind1Names {
		if n := len(name) + 1; len(s) >= n && strings.EqualFold(string(s[1:n]), name) &&
			(len(s) == n || isTagSpace(s[n]) || s[n] == '>') {
			return 1
		}
	}
	switch {
	case bytes.HasPrefix(s, []byte("<!--")):
		return 2
	case s[1] == '?':
		return 3
	case s[1] == '!' && len(s) > 2 && isASCIILetter(s[2]):
		// CommonMark 0.31.2 takes any ASCII letter; GitHub only an uppercase
		// letter (testdata/dialect.md).
		return 4
	case bytes.HasPrefix(s, []byte("<![CDATA[")):
		return 5
	}
	name, k := htmlTagName(s)
	if kind6Names[strings.ToLower(string(name))] &&
		(k == len(s) || isTagSpace(s[k]) || s[k] == '>' || bytes.HasPrefix(s[k:], []byte("/>"))) {
		return 6
	}
	// cmark also takes an open tag named pre, script, style or textarea, which
	// the spec text excludes, and FF but not VT after the tag.
	if n := htmlTagLen(s); n > 0 && len(bytes.Trim(s[n:], " \t\f")) == 0 {
		return 7
	}
	return 0
}

// htmlTagName returns the tag name that the kind 6 start condition reads after
// '<' or "</" at the start of s, and the offset after it.
func htmlTagName(s []byte) ([]byte, int) {
	j := 1
	if len(s) > 1 && s[1] == '/' {
		j = 2
	}
	k := j
	for k < len(s) && k-j <= 10 && (isASCIILetter(s[k]) || '0' <= s[k] && s[k] <= '9') {
		k++
	}
	return s[j:k], k
}

// htmlTagLen returns the length of the open tag or closing tag at the start
// of s, a line, or 0.
func htmlTagLen(s []byte) int {
	i := 1
	closing := len(s) > 1 && s[1] == '/'
	if closing {
		i = 2
	}
	if i >= len(s) || !isASCIILetter(s[i]) {
		return 0
	}
	j := i + 1
	for j < len(s) && (isASCIILetter(s[j]) || '0' <= s[j] && s[j] <= '9' || s[j] == '-') {
		j++
	}
	if closing {
		j = skipTagSpace(s, j)
		if j < len(s) && s[j] == '>' {
			return j + 1
		}
		return 0
	}
	for {
		k := skipTagSpace(s, j)
		if k == j || k == len(s) || !isAttrNameStart(s[k]) {
			j = k
			break
		}
		for k++; k < len(s) && (isAttrNameStart(s[k]) || '0' <= s[k] && s[k] <= '9' || s[k] == '.' || s[k] == '-'); k++ {
		}
		j = k
		v := skipTagSpace(s, k)
		if v == len(s) || s[v] != '=' {
			continue
		}
		v = skipTagSpace(s, v+1)
		switch {
		case v == len(s):
			return 0
		case s[v] == '"' || s[v] == '\'':
			e := bytes.IndexByte(s[v+1:], s[v])
			if e < 0 {
				return 0
			}
			j = v + 1 + e + 1
		default:
			e := v
			for e < len(s) && strings.IndexByte(" \t\v\f\"'=<>`", s[e]) < 0 {
				e++
			}
			if e == v {
				return 0
			}
			j = e
		}
	}
	if j < len(s) && s[j] == '/' {
		j++
	}
	if j < len(s) && s[j] == '>' {
		return j + 1
	}
	return 0
}

// htmlBlockEnds reports whether the line src[i:end] meets the end condition
// of an HTML block of kind k, from 1 to 5.
func htmlBlockEnds(src []byte, i, end uint32, k uint8) bool {
	s := src[i:end]
	switch k {
	case 1:
		for j := bytes.Index(s, []byte("</")); j >= 0; j = bytes.Index(s, []byte("</")) {
			s = s[j+2:]
			for _, name := range kind1Names {
				if len(s) > len(name) && strings.EqualFold(string(s[:len(name)]), name) && s[len(name)] == '>' {
					return true
				}
			}
		}
		return false
	case 2:
		return bytes.Contains(s, []byte("-->"))
	case 3:
		return bytes.Contains(s, []byte("?>"))
	case 4:
		return bytes.IndexByte(s, '>') >= 0
	default:
		return bytes.Contains(s, []byte("]]>"))
	}
}

// htmlKindAgrees reports whether the flags of HTML block i give the kind
// whose start condition its first line meets.
func (t *Tree) htmlKindAgrees(i int) bool {
	if i+1 >= len(t.nodes) || t.nodes[i+1].kind != HTMLText {
		return false
	}
	m := t.nodes[i+1]
	if m.start >= m.end || int(m.end) > len(t.src) {
		return false
	}
	j := m.start
	for j < m.end && isSpaceOrTab(t.src[j]) {
		j++
	}
	return j < m.end && htmlBlockStart(t.src, j, m.end) == t.nodes[i].flags
}

// HTMLBlockClosed reports whether HTML block id ends at its end condition,
// so that a blank line after it is not its content: a block of kind 6 or 7,
// or a block of kind 1 to 5 whose last line meets the end condition.
func (t *Tree) HTMLBlockClosed(id NodeID) bool {
	n := t.nodes[id]
	if n.flags >= 6 {
		return true
	}
	for i := n.link - 1; i > uint32(id); i-- {
		if m := t.nodes[i]; m.kind == HTMLText {
			return htmlBlockEnds(t.src, m.start, m.end, n.flags)
		}
	}
	return false
}

// AppendHTML appends the value of HTML block id to dst.
func (t *Tree) AppendHTML(dst []byte, id NodeID) []byte {
	return t.appendVerbatim(dst, id, HTMLText)
}

func isAttrNameStart(c byte) bool {
	return isASCIILetter(c) || c == '_' || c == ':'
}

// isTagSpace reports whether c is whitespace in an HTML tag: a space, a tab,
// VT or FF, as cmark and commonmark.js read it.
func isTagSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\v' || c == '\f'
}

func skipTagSpace(s []byte, i int) int {
	for i < len(s) && isTagSpace(s[i]) {
		i++
	}
	return i
}
