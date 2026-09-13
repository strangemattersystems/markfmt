package markdown

// The flags of a list item that is a task.
const (
	taskFlag    = 1
	checkedFlag = 2
)

// taskFlags returns the flags that a task box with the character c between its
// brackets gives its list item: 'x' or 'X' is checked.
func taskFlags(c byte) uint8 {
	if c|0x20 == 'x' {
		return taskFlag | checkedFlag
	}
	return taskFlag
}

// ListItemTask reports whether list item id is a task, and whether the task
// is checked.
func (t *Tree) ListItemTask(id NodeID) (task, checked bool) {
	f := t.nodes[id].flags
	return f&taskFlag != 0, f&checkedFlag != 0
}

// isTaskBox reports whether src[i:end], the first line of a paragraph from its
// first byte that is not a space or a tab, starts with a task box: '[', a
// space, a tab, 'x' or 'X', and ']', then spaces or tabs and another byte
// (design 6.5).
func isTaskBox(src []byte, i, end uint32) bool {
	if end-i < 5 || src[i] != '[' || src[i+2] != ']' || !isSpaceOrTab(src[i+3]) {
		return false
	}
	if c := src[i+1]; !isSpaceOrTab(c) && c|0x20 != 'x' {
		return false
	}
	for j := i + 4; j < end; j++ {
		if !isSpaceOrTab(src[j]) {
			return true
		}
	}
	return false
}

// taskBox pushes the task box at the start of the first line and the spaces
// and tabs after it, and records the character between its brackets in box,
// unless its brackets form a link (design 6.5). The brackets are scanned as a
// link opener and closer, so that a defined label wins.
func (s *inlineParser) taskBox() {
	i, end := s.lineStart, s.lines[0].rest.end
	if !isTaskBox(s.src, i, end) {
		return
	}
	n := len(s.pieces)
	s.openBracket(i, false)
	s.push(piece{kind: Text, held: true, end: i + 2})
	s.closeBracket(i+2, end)
	if s.pieces[n].open == Link {
		return
	}
	for k := range 3 {
		s.pieces[n+k] = piece{kind: TaskBox, join: k > 0, end: i + count(k) + 1}
	}
	j := i + 3
	for isSpaceOrTab(s.src[j]) {
		j++
	}
	s.push(piece{kind: Whitespace, end: j})
	s.box = s.src[i+1]
}

// taskBoxAgrees reports whether list item i, which has flags, has a task box
// that gives them: its first child that is not a link reference definition is
// a paragraph that starts with the box.
func (t *Tree) taskBoxAgrees(i int) bool {
	for j := i + 1; j < int(t.nodes[i].link) && j < len(t.nodes); j++ {
		switch m := t.nodes[j]; {
		case m.kind == LinkReferenceDefinition:
			if int(m.link) <= j {
				return false
			}
			j = int(m.link) - 1
		case m.kind == Paragraph:
			if j+1 < len(t.nodes) && t.nodes[j+1].kind == Indent {
				j++
			}
			box := j + 1
			return box < len(t.nodes) && t.nodes[box].kind == TaskBox && t.nodes[box].end-t.nodes[box].start == 3 &&
				int(t.nodes[box].end) <= len(t.src) && taskFlags(t.src[t.nodes[box].start+1]) == t.nodes[i].flags
		case m.kind.class() == classStructure:
			return false
		}
	}
	return false
}
