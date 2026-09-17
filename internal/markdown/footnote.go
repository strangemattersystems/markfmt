package markdown

import "strings"

// footnoteStart returns the end of the label and the end of the start of the
// footnote definition at first, the first byte of the rest of a line that is
// not a space or a tab, before end, or 0 and 0: "[^", one or more bytes that are
// not ']', a space, a tab, CR, LF or NUL, "]:", then spaces and tabs, as
// cmark-gfm's footnote_definition scanner reads it (design 9.2).
func footnoteStart(src []byte, first, end uint32) (labelEnd, contentStart uint32) {
	if end-first < 5 || src[first] != '[' || src[first+1] != '^' {
		return 0, 0
	}
	j := first + 2
	for j < end && strings.IndexByte("] \t\r\n\x00", src[j]) < 0 {
		j++
	}
	if j == first+2 || j+1 >= end || src[j] != ']' || src[j+1] != ':' {
		return 0, 0
	}
	k := j + 2
	for k < end && isSpaceOrTab(src[k]) {
		k++
	}
	return j, k
}

// startFootnote opens a footnote definition that starts at first, whose label
// ends at labelEnd and whose content starts at contentStart, appends its
// [Indent], label and [Whitespace] leaves, and records its label in the
// footnote label list.
func (p *blockParser) startFootnote(first, labelEnd, contentStart uint32) {
	p.b.open(FootnoteDefinition)
	p.push(container{kind: FootnoteDefinition, node: p.b.top()})
	p.b.leafIf(Indent, first)
	p.b.leaf(Bracket, first+1)
	p.b.leaf(Caret, first+2)
	p.b.leaf(FootnoteLabel, labelEnd)
	p.b.leaf(Bracket, labelEnd+1)
	p.b.leaf(Colon, labelEnd+2)
	p.b.leafIf(Whitespace, contentStart)
	for i := p.pos; i < contentStart; i++ {
		p.col = nextColumn(p.src[i], p.col)
	}
	p.pos, p.used = contentStart, 0
	f := labelFolder{dst: p.label[:0]}
	f.write(p.src[first+2 : labelEnd])
	p.label = f.dst
	p.defs.addFootnote(p.label, p.pass1)
}

// continueFootnote reports whether footnote definition c continues on the rest
// of the line: on 4 columns of indentation, which it consumes as a
// [FootnoteIndent] leaf, or on a line with no byte before its line ending.
// cmark-gfm tests the whole line, so a line of spaces and a line with a prefix
// end it (design 5.1, 9.2).
func (p *blockParser) continueFootnote(c container) bool {
	if _, indent := p.indentation(); indent < 4 {
		return p.l.start == p.l.end
	}
	start, virt := p.pos, splitVirt(p.col, p.used)
	p.consumeColumns(4)
	if p.pos > start {
		p.prefix = append(p.prefix, prefixLeaf{kind: FootnoteIndent, virt: virt, end: p.pos, owner: c.node})
	}
	return true
}

// FootnoteDefinitionLabel returns the label of footnote definition id, as
// written: GitHub writes it into element ids (design 9.2).
func (t *Tree) FootnoteDefinitionLabel(id NodeID) []byte {
	for i := uint32(id) + 1; i < t.nodes[id].link; i++ {
		if m := t.nodes[i]; m.kind == FootnoteLabel {
			return t.src[m.start:m.end]
		}
	}
	return nil
}

// FootnoteReferenceResolved reports whether footnote reference id resolves to
// a footnote definition.
func (t *Tree) FootnoteReferenceResolved(id NodeID) bool {
	return t.nodes[id].flags == 1
}

// AppendFootnoteReferenceLabel appends the label of footnote reference id to
// dst: the label bytes after its caret (design 6.7), normalized when normalize
// is true.
func (t *Tree) AppendFootnoteReferenceLabel(dst []byte, id NodeID, normalize bool) []byte {
	f := labelFolder{dst: dst, start: len(dst)}
	caret := false
	for i := uint32(id) + 1; i+1 < t.nodes[id].link; i++ {
		m := t.nodes[i]
		b := t.src[m.start:m.end]
		switch _, prefix := m.kind.owner(); {
		case m.kind == Caret:
			caret = true
		case !caret, normalize:
			if caret {
				f.leaf(m.kind, b)
			}
		case prefix, m.kind == Indent:
		case m.kind == VerbatimLineEnding:
			f.dst = append(f.dst, '\n')
		case m.kind == CellPipeEscape:
			f.dst = append(append(f.dst, b[:len(b)-2]...), '|')
		default:
			f.dst = append(f.dst, b...)
		}
	}
	return f.dst
}
