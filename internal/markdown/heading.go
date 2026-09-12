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

// setextUnderline returns the end of the setext heading underline at
// src[i:end], a non-blank line after its indentation, or 0: a run of '=' or
// '-' with only spaces and tabs after it.
func setextUnderline(src []byte, i, end uint32) uint32 {
	c := src[i]
	if c != '=' && c != '-' {
		return 0
	}
	j := i
	for j < end && src[j] == c {
		j++
	}
	if trimSpaceRight(src, j, end) != j {
		return 0
	}
	return j
}

// HeadingLevel returns the level of heading id, from 1 to 6.
func (t *Tree) HeadingLevel(id NodeID) int {
	i := t.nodes[id].link - 1
	for t.nodes[i].kind == LineEnding || t.nodes[i].kind == Whitespace {
		i--
	}
	if n := t.nodes[i]; n.kind == SetextUnderline {
		if t.src[n.start] == '=' {
			return 1
		}
		return 2
	}
	for i = uint32(id) + 1; t.nodes[i].kind != ATXMarker; i++ {
	}
	return int(t.nodes[i].end - t.nodes[i].start)
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
