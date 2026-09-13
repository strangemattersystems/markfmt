package markdown

// scanThematicBreak reports whether src[i:end], a non-blank line after its
// indentation, is a thematic break: three or more of one marker, '*', '-' or
// '_', with only spaces and tabs between and after them. When it is not, it
// also returns an offset such that a scan from any marker of src[i:end] before
// that offset fails too, or 0 (design 5.1).
func scanThematicBreak(src []byte, i, end uint32) (bool, uint32) {
	c := src[i]
	if c != '*' && c != '-' && c != '_' {
		return false, i + 1
	}
	n := 0
	for j := i; j < end; j++ {
		switch src[j] {
		case c:
			n++
		case ' ', '\t':
		case '*', '-', '_':
			// A scan from j itself may still succeed.
			return false, j
		default:
			return false, j + 1
		}
	}
	return n >= 3, 0
}
