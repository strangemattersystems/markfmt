package markdown

// atxHeading is the layout of an ATX heading line after its indentation.
// The marker ends at markerEnd, the text is src[textStart:textEnd], and the
// closing sequence is src[closeStart:closeEnd]. Whitespace fills the gaps.
type atxHeading struct {
	markerEnd, textStart, textEnd, closeStart, closeEnd uint32
}

// parseATXHeading returns the layout of the ATX heading at src[i:end], a line
// after its indentation, or false.
func parseATXHeading(src []byte, i, end uint32) (atxHeading, bool) {
	j := i
	for j < end && src[j] == '#' {
		j++
	}
	if n := j - i; n == 0 || n > 6 || j < end && !isSpaceOrTab(src[j]) {
		return atxHeading{}, false
	}
	e := trimSpaceRight(src, j, end)
	h := atxHeading{markerEnd: j, closeStart: e, closeEnd: e}
	c := e
	for c > j && src[c-1] == '#' {
		c--
	}
	if c < e && isSpaceOrTab(src[c-1]) {
		h.closeStart = c
		e = trimSpaceRight(src, j, c)
	}
	h.textEnd = e
	h.textStart = j
	for h.textStart < e && isSpaceOrTab(src[h.textStart]) {
		h.textStart++
	}
	return h, true
}

// HeadingLevel returns the level of heading id, from 1 to 6.
func (t *Tree) HeadingLevel(id NodeID) int {
	for i := id + 1; ; i++ {
		if n := t.nodes[i]; n.kind == ATXMarker {
			return int(n.end - n.start)
		}
	}
}

func isSpaceOrTab(c byte) bool {
	return c == ' ' || c == '\t'
}

// trimSpaceRight returns the end of src[i:end] without its trailing spaces
// and tabs.
func trimSpaceRight(src []byte, i, end uint32) uint32 {
	for end > i && isSpaceOrTab(src[end-1]) {
		end--
	}
	return end
}
