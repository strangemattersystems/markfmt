package markdown

import (
	"bytes"
	"fmt"
	"unicode/utf8"
)

// Kept returns the kept syntax of tree in document order: the rows of each
// dialect span and the blank lines before and after it, the bytes of each
// [Escape], [EntityRef] and [CellPipeEscape] leaf, the raw bytes of each label,
// each NUL and invalid UTF-8 sequence in a content leaf, and each ordered list
// with lazy numbering.
func Kept(tree *Tree) []string {
	events := keptEvents(tree)
	texts := make([]string, len(events))
	for i, e := range events {
		texts[i] = e.text
	}
	return texts
}

// KeptMismatch returns a [MismatchError] at the first event of kept syntax
// where a and b differ, or nil.
func KeptMismatch(a, b *Tree) error {
	ea, eb := keptEvents(a), keptEvents(b)
	for i := range max(len(ea), len(eb)) {
		if i < len(ea) && i < len(eb) && ea[i].text == eb[i].text {
			continue
		}
		at := NodeID(0)
		if i < len(ea) {
			at = ea[i].at
		}
		return &MismatchError{At: at, msg: fmt.Sprintf("markdown: different kept syntax at event %d", i)}
	}
	return nil
}

type keptEvent struct {
	at   NodeID
	text string
}

func keptEvents(tree *Tree) []keptEvent {
	var events []keptEvent
	spans := tree.spans
	for i, n := range tree.nodes {
		id := NodeID(i)
		b := tree.src[n.start:n.end]
		if len(spans) > 0 && spans[0].id == uint32(i) {
			events = append(events, keptEvent{id, fmt.Sprintf("span %b, blank lines %d and %d", spans[0].rows, blankLinesBefore(tree, i), blankLinesAfter(tree, int(n.link)))})
			spans = spans[1:]
		}
		switch n.kind {
		case Escape, EntityRef, CellPipeEscape:
			events = append(events, keptEvent{id, n.kind.String() + " " + string(b)})
		case LinkReferenceDefinition, FootnoteReference:
			events = append(events, keptEvent{id, "label " + string(rawLabel(tree, id, 1))})
		case FootnoteDefinition:
			events = append(events, keptEvent{id, "label " + string(tree.FootnoteDefinitionLabel(id))})
		case Link, Image:
			switch tree.LinkForm(id) {
			case FullReference:
				events = append(events, keptEvent{id, "label " + string(rawLabel(tree, id, 3))})
			case CollapsedReference, ShortcutReference:
				events = append(events, keptEvent{id, "label " + string(rawLabel(tree, id, 1))})
			case InlineLink:
			}
		case List:
			if lazyNumbering(tree, id) {
				events = append(events, keptEvent{id, "lazy numbering"})
			}
		}
		// Only a NUL or an invalid sequence is kept, and most text has none,
		// which utf8.Valid finds faster than a decode of each rune.
		if n.kind.class() != classContent || utf8.Valid(b) && bytes.IndexByte(b, 0) < 0 {
			continue
		}
		for len(b) > 0 {
			r, size := decodeRune(b)
			if r == '�' && !bytes.HasPrefix(b, []byte("�")) {
				events = append(events, keptEvent{id, "invalid " + string(b[:size])})
			}
			b = b[size:]
		}
	}
	return events
}

// rawLabel returns the bytes of the leaves of node id after its own bracket
// first and before the next one, without prefix, Indent and Caret leaves, and
// with LF line endings.
func rawLabel(tree *Tree, id NodeID, first int) []byte {
	var label []byte
	brackets, nested := 0, uint32(0)
	for i := uint32(id) + 1; i < tree.nodes[id].link && brackets <= first; i++ {
		m := tree.nodes[i]
		_, prefix := m.kind.owner()
		switch {
		case m.kind.class() == classStructure && brackets < first:
			// A structure before the label holds none of it, and reading it
			// again for each nested reference is O(n^2).
			i = m.link - 1
		case m.kind.class() == classStructure:
			nested = max(nested, m.link)
		case m.kind == Bracket && i >= nested:
			brackets++
		case brackets == first && !prefix && m.kind != Indent && m.kind != Caret:
			label = append(label, tree.src[m.start:m.end]...)
		}
	}
	return bytes.ReplaceAll(bytes.ReplaceAll(label, []byte("\r\n"), []byte("\n")), []byte("\r"), []byte("\n"))
}

// lazyNumbering reports whether list id is ordered, starts at 1, and has a
// second item numbered 1.
func lazyNumbering(tree *Tree, id NodeID) bool {
	if start, ordered := tree.ListStart(id); !ordered || start != 1 {
		return false
	}
	for i := tree.nodes[id+1].link; i < tree.nodes[id].link; i++ {
		if tree.nodes[i].kind != ListItem {
			continue
		}
		marker := tree.nodes[i+1]
		j := marker.start
		for isSpaceOrTab(tree.src[j]) {
			j++
		}
		m, _ := parseListMarker(tree.src, j, marker.end)
		return m.start == 1
	}
	return false
}

// blankLinesBefore counts the BlankLine leaves between node id and the block
// before it, across prefix leaves. Blank lines at the start of a container
// have no meaning, so they count as none, and the blank rest of a marker line
// is no blank line.
func blankLinesBefore(tree *Tree, id int) int {
	n := 0
	for i := id - 1; i >= 0; i-- {
		m := tree.nodes[i]
		_, prefix := m.kind.owner()
		switch {
		case m.kind == BlankLine:
			if !markerRest(tree, i) {
				n++
			}
		case prefix:
		case m.kind.class() == classStructure && int(m.link) > id:
			return 0
		case m.kind.class() == classStructure:
			return n
		case m.kind != Colon && m.kind != Whitespace:
			return n
		default:
			// The leaf starts a container that holds node id when that
			// container is its parent: the label line of a footnote
			// definition, which ends with its colon and the spaces after it.
			for j := i - 1; j >= 0; j-- {
				if s := tree.nodes[j]; s.kind.class() == classStructure && int(s.link) > i {
					if int(s.link) > id {
						return 0
					}
					return n
				}
			}
			return n
		}
	}
	return 0
}

// markerRest reports whether BlankLine i is the rest of the first line of a
// list item or a footnote definition.
func markerRest(tree *Tree, i int) bool {
	j := i - 1
	for j >= 0 && (tree.nodes[j].kind.class() == classStructure || tree.nodes[j].kind == Whitespace) {
		j--
	}
	return j >= 0 && (tree.nodes[j].kind == ListMarker || tree.nodes[j].kind == Colon)
}

// blankLinesAfter counts the BlankLine leaves from node i to the next node,
// across prefix leaves. Blank lines at the end of the input have no meaning,
// so they count as none.
func blankLinesAfter(tree *Tree, i int) int {
	n := 0
	for ; i < len(tree.nodes); i++ {
		m := tree.nodes[i]
		_, prefix := m.kind.owner()
		switch {
		case m.kind == BlankLine:
			n++
		case !prefix:
			return n
		}
	}
	return 0
}
