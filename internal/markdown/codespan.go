package markdown

import "bytes"

// codeSpan pushes the code span that opens with the backtick run at i, on a
// line that ends at end, or the run as text when no later run of its length
// closes it (CM 338, 348).
func (s *inlineParser) codeSpan(i, end uint32) {
	open := backtickRunEnd(s.src, i, end)
	k, closer, ok := s.findBackticks(open, open-i)
	if !ok {
		s.text(open)
		return
	}
	s.push(piece{kind: CodeFence, open: CodeSpan, end: open})
	for s.k < k {
		l := s.lines[s.k].rest
		s.pushIf(CodeText, l.end)
		s.push(piece{kind: VerbatimLineEnding, end: l.eol})
		s.k++
		s.startLine()
	}
	s.pushIf(CodeText, closer)
	s.push(piece{kind: CodeFence, close: true, end: closer + open - i})
}

// findBackticks returns the line and the offset of the first backtick run of
// length n that starts at or after from, on the line being scanned or a later
// one. It records the last run of each length that it passes. After one
// search reaches the end of the block, a search that the record answers needs
// no scan, so the searches of a block are linear (design 6.8).
func (s *inlineParser) findBackticks(from, n uint32) (int, uint32, bool) {
	if s.ticksAll && (int(n) >= len(s.ticks) || s.ticks[n] <= from) {
		return 0, 0, false
	}
	for k, i := s.k, from; k < len(s.lines); k++ {
		l := s.lines[k].rest
		if k > s.k {
			i = l.start
		}
		for i < l.end {
			rel := bytes.IndexByte(s.src[i:l.end], '`')
			if rel < 0 {
				break
			}
			j := i + count(rel)
			i = backtickRunEnd(s.src, j, l.end)
			m := i - j
			for count(len(s.ticks)) <= m {
				s.ticks = append(s.ticks, 0)
			}
			s.ticks[m] = j + 1
			if m == n {
				return k, j, true
			}
		}
	}
	s.ticksAll = true
	return 0, 0, false
}

func backtickRunEnd(src []byte, i, end uint32) uint32 {
	for i < end && src[i] == '`' {
		i++
	}
	return i
}

// AppendCodeSpan appends the value of code span id to dst.
func (t *Tree) AppendCodeSpan(dst []byte, id NodeID) []byte {
	r := newCodeSpanReader(t, id)
	return appendPieces(dst, &r)
}

// codeSpanReader reads the value of a code span: its CodeText leaves with a
// space for each line ending, without one space at each end when both ends
// are spaces and not every byte is a space (CM 329 to 335).
type codeSpanReader struct {
	t      *Tree
	i, end uint32 // the next leaf, and the closing fence
	skip   bool   // the first byte is not in the value
	left   int    // bytes of the value left to read
	leaf   valueReader
}

func newCodeSpanReader(t *Tree, id NodeID) codeSpanReader {
	r := codeSpanReader{t: t, i: uint32(id) + 2, end: t.nodes[id].link - 1}
	var first, last byte
	blank := true
	for _, m := range t.nodes[r.i:r.end] {
		b := t.codeSpanBytes(m)
		if len(b) == 0 {
			continue
		}
		if r.left == 0 {
			first = b[0]
		}
		last = b[len(b)-1]
		r.left += len(b)
		blank = blank && len(bytes.TrimLeft(b, " ")) == 0
	}
	if first == ' ' && last == ' ' && !blank {
		r.skip, r.left = true, r.left-2
	}
	return r
}

func (r *codeSpanReader) next() []byte {
	for {
		if b := r.leaf.next(); b != nil {
			return b
		}
		if r.left == 0 || r.i == r.end {
			return nil
		}
		b := r.t.codeSpanBytes(r.t.nodes[r.i])
		r.i++
		if r.skip && len(b) > 0 {
			b, r.skip = b[1:], false
		}
		b = b[:min(len(b), r.left)]
		r.left -= len(b)
		r.leaf = valueReader{b: b}
	}
}

// codeSpanBytes returns the bytes that leaf m of a code span gives its value.
func (t *Tree) codeSpanBytes(m Node) []byte {
	switch m.kind {
	case CodeText:
		return t.src[m.start:m.end:m.end]
	case VerbatimLineEnding:
		return spaces[:1:1]
	}
	return nil
}
