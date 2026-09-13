package markdown

// defCut is the end of one leaf of a link reference definition, or, when
// line is 0 or more, the start of that pending line, whose prefix leaves come
// there.
type defCut struct {
	kind Kind
	end  uint32
	line int
}

// defParser reads link reference definitions from the pending lines of a
// paragraph. It reads each line from its first byte that is not a space or a
// tab, as a paragraph does, and records the leaves as cuts.
type defParser struct {
	p    *blockParser
	k    int    // index of the pending line being read
	i    uint32 // offset in that line
	cuts []defCut
}

// commitDefinitions appends the link reference definitions at the start of
// the pending paragraph lines, and removes their lines (design 5.4). It
// reports whether a pending line remains.
func (p *blockParser) commitDefinitions() bool {
	d := defParser{p: p}
	k := 0
	for k < len(p.pending) && d.definition(k) {
		for n, c := range d.cuts {
			if c.line < 0 {
				p.b.leafIf(c.kind, c.end)
				continue
			}
			pl := p.pending[c.line]
			p.appendPendingPrefix(pl)
			if n == 0 {
				p.b.open(LinkReferenceDefinition)
			}
			p.b.split = splitVirt(int(pl.col), int(pl.used))
		}
		p.b.close()
		k = d.k + 1
	}
	p.pending = p.pending[k:]
	return len(p.pending) > 0
}

// definition reads a link reference definition from the start of pending
// line k: a label, ':', a destination and an optional title, then the end of
// a line.
func (d *defParser) definition(k int) bool {
	d.cuts = d.cuts[:0]
	d.startLine(k)
	if !d.label() || d.i == d.end() || d.p.src[d.i] != ':' {
		return false
	}
	d.i++
	d.cut(Colon)
	d.spaceLine()
	if !d.destination() {
		return false
	}
	k, i, n := d.k, d.i, len(d.cuts)
	if !d.spaceLine() || !d.title() || !d.lineEnd() {
		// A failed title rewinds to the end of the destination (CM 209, 210).
		d.k, d.i, d.cuts = k, i, d.cuts[:n]
		if !d.lineEnd() {
			return false
		}
	}
	d.i = d.p.pending[d.k].rest.eol
	d.cut(LineEnding)
	return true
}

// label reads a link label: '[', up to 999 characters with no unescaped
// bracket and at least one that is not a space, tab or line ending, and ']'
// (design 6.7).
func (d *defParser) label() bool {
	src := d.p.src
	if d.i == d.end() || src[d.i] != '[' {
		return false
	}
	d.i++
	d.cut(Bracket)
	chars, blank := 0, true
	for chars <= 999 {
		if d.i == d.end() {
			d.cut(LinkLabel)
			if !d.nextLine(VerbatimLineEnding) {
				return false
			}
			chars++
			continue
		}
		switch c := src[d.i]; {
		case c == ']':
			d.cut(LinkLabel)
			d.i++
			d.cut(Bracket)
			return !blank
		case c == '[':
			return false
		case c == '\\' && d.i+1 < d.end() && isASCIIPunct(src[d.i+1]):
			d.i++
			chars++
			blank = false
		case !isSpaceOrTab(c):
			blank = false
		}
		if src[d.i]&0xC0 != 0x80 {
			chars++
		}
		d.i++
	}
	return false
}

// destination reads a link destination: '<', characters with no unescaped
// '<' or '>', and '>' on one line; or characters that are not spaces or
// ASCII control characters, with parentheses balanced and nested at most 32
// deep.
func (d *defParser) destination() bool {
	src, end := d.p.src, d.end()
	if d.i == end {
		return false
	}
	if src[d.i] == '<' {
		d.i++
		d.cut(AngleBracket)
		for ; d.i < end; d.i++ {
			switch src[d.i] {
			case '>':
				d.cut(Destination)
				d.i++
				d.cut(AngleBracket)
				return true
			case '<':
				return false
			case '\\':
				if d.i+1 < end && isASCIIPunct(src[d.i+1]) {
					d.i++
				}
			}
		}
		return false
	}
	start, depth := d.i, 0
	for ; d.i < end; d.i++ {
		c := src[d.i]
		if c == '\\' && d.i+1 < end && isASCIIPunct(src[d.i+1]) {
			d.i++
			continue
		}
		if c <= ' ' || c == 0x7f || c == ')' && depth == 0 {
			break
		}
		switch c {
		case '(':
			if depth++; depth > 32 {
				return false
			}
		case ')':
			depth--
		}
	}
	if d.i == start || depth > 0 {
		return false
	}
	d.cut(Destination)
	return true
}

// title reads a link title over any number of lines: characters between two
// double quotes, between two apostrophes, or between '(' and ')' with no
// unescaped '('.
func (d *defParser) title() bool {
	src := d.p.src
	if d.i == d.end() {
		return false
	}
	open, closing := src[d.i], src[d.i]
	switch open {
	case '"', '\'':
	case '(':
		closing = ')'
	default:
		return false
	}
	d.i++
	d.cut(TitleQuote)
	for {
		if d.i == d.end() {
			d.cut(Title)
			if !d.nextLine(VerbatimLineEnding) {
				return false
			}
			continue
		}
		switch c := src[d.i]; {
		case c == closing:
			d.cut(Title)
			d.i++
			d.cut(TitleQuote)
			return true
		case c == '(' && open == '(':
			return false
		case c == '\\' && d.i+1 < d.end() && isASCIIPunct(src[d.i+1]):
			d.i++
		}
		d.i++
	}
}

// spaceLine skips spaces and tabs, up to one line ending, and the spaces and
// tabs after it. It reports whether it skipped any.
func (d *defParser) spaceLine() bool {
	k, i := d.k, d.i
	d.skipSpace()
	if d.i == d.end() {
		d.nextLine(LineEnding)
	}
	return d.k != k || d.i != i
}

// lineEnd skips spaces and tabs, and reports whether the line ends there.
func (d *defParser) lineEnd() bool {
	d.skipSpace()
	return d.i == d.end()
}

func (d *defParser) skipSpace() {
	d.i = d.p.skipSpace(d.i, d.end())
	d.cut(Whitespace)
}

// nextLine records the line ending of the line as a leaf of kind k and moves
// to the next pending line. It reports false on the last line.
func (d *defParser) nextLine(k Kind) bool {
	if d.k+1 == len(d.p.pending) {
		return false
	}
	d.i = d.p.pending[d.k].rest.eol
	d.cut(k)
	d.startLine(d.k + 1)
	return true
}

// startLine moves to pending line k, after its leading spaces and tabs,
// which are one Indent leaf.
func (d *defParser) startLine(k int) {
	d.k = k
	rest := d.p.pending[k].rest
	d.cuts = append(d.cuts, defCut{line: k})
	d.i = d.p.skipSpace(rest.start, rest.end)
	d.cut(Indent)
}

// cut records the end of a leaf of kind k at the offset.
func (d *defParser) cut(k Kind) {
	d.cuts = append(d.cuts, defCut{kind: k, end: d.i, line: -1})
}

func (d *defParser) end() uint32 {
	return d.p.pending[d.k].rest.end
}
