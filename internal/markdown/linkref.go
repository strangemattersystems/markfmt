package markdown

// commitDefinitions appends the link reference definitions at the start of
// the pending paragraph lines, and removes their lines (design 5.4). It
// reports whether a pending line remains.
func (p *blockParser) commitDefinitions() bool {
	s := &p.inline
	s.arena = p.arena
	k := 0
	for k < len(p.pending) {
		s.begin(p.pending[k:])
		if !s.definition() {
			break
		}
		p.appendPendingPrefix(p.pending[k])
		p.b.open(LinkReferenceDefinition)
		s.emit()
		p.b.close()
		k += s.k + 1
	}
	p.pending = p.pending[k:]
	return len(p.pending) > 0
}

// AppendLabel appends the normalized label of link reference definition id
// to dst (design 6.7).
func (t *Tree) AppendLabel(dst []byte, id NodeID) []byte {
	f := labelFolder{dst: dst, start: len(dst)}
	brackets := 0
	for _, m := range t.nodes[id+1 : t.nodes[id].link] {
		switch {
		case m.kind == Bracket:
			if brackets++; brackets == 2 {
				return f.dst
			}
		case brackets == 1 && m.kind == LinkLabel:
			f.write(t.src[m.start:m.end])
		case brackets == 1 && m.kind == VerbatimLineEnding:
			f.write([]byte{'\n'})
		}
	}
	return f.dst
}

// definition pushes the leaves of the link reference definition at the
// start of the lines: a label, ':', a destination and an optional title, then
// the end of a line. It reports whether there is one.
func (s *inlineParser) definition() bool {
	if !s.linkLabel() {
		return false
	}
	if _, ok := s.expect(pos{s.k, s.end()}, ':'); !ok {
		return false
	}
	s.push(piece{kind: Colon, end: s.end() + 1})
	s.spaceLine()
	if !s.linkDestination() {
		return false
	}
	m := s.mark()
	if !s.spaceLine() || !s.linkTitle() || !s.lineEnd() {
		// A failed title rewinds to the end of the destination (CM 209, 210).
		s.reset(m)
		if !s.lineEnd() {
			return false
		}
	}
	s.pushIf(LineEnding, s.lines[s.k].rest.eol)
	return true
}
