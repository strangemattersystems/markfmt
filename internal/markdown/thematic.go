package markdown

// isThematicBreak reports whether src[i:end], a line after its indentation,
// is a thematic break: three or more of one marker, '*', '-' or '_', with
// only spaces and tabs between and after them.
func isThematicBreak(src []byte, i, end uint32) bool {
	c := src[i]
	if c != '*' && c != '-' && c != '_' {
		return false
	}
	n := 0
	for ; i < end; i++ {
		switch src[i] {
		case c:
			n++
		case ' ', '\t':
		default:
			return false
		}
	}
	return n >= 3
}
