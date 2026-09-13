package markdown

// frontMatterFence returns the end of the front matter fence that is line l,
// and its delimiter: "---" or "+++" with only spaces and tabs after it. It
// returns 0 when l is not a fence.
func frontMatterFence(src []byte, l line) (uint32, byte) {
	if l.end-l.start < 3 {
		return 0, 0
	}
	c := src[l.start]
	if c != '-' && c != '+' || src[l.start+1] != c || src[l.start+2] != c ||
		trimSpaceRight(src, l.start+3, l.end) != l.start+3 {
		return 0, 0
	}
	return l.start + 3, c
}

// FrontMatterTOML reports whether front matter id is TOML: its fences are
// "+++", not "---".
func (t *Tree) FrontMatterTOML(id NodeID) bool {
	return t.src[t.nodes[id+1].start] == '+'
}

// frontMatter appends the front matter at the start of the lines of it, if
// there is one, and returns the lines after it (design 8.1).
func (p *blockParser) frontMatter(it lines) lines {
	body := it
	open, ok := body.next()
	if !ok {
		return it
	}
	end, delim := frontMatterFence(p.src, open)
	if end == 0 {
		return it
	}
	after := body
	for l, ok := after.next(); ok; l, ok = after.next() {
		closeEnd, c := frontMatterFence(p.src, l)
		if closeEnd == 0 || c != delim {
			continue
		}
		p.b.open(FrontMatter)
		p.b.leaf(FrontMatterFence, end)
		p.b.leafIf(Whitespace, open.end)
		p.b.leafIf(LineEnding, open.eol)
		for b, _ := body.next(); b.start != l.start; b, _ = body.next() {
			p.b.leafIf(FrontMatterText, b.end)
			p.b.leafIf(VerbatimLineEnding, b.eol)
		}
		p.b.leaf(FrontMatterFence, closeEnd)
		p.b.leafIf(Whitespace, l.end)
		p.b.leafIf(LineEnding, l.eol)
		p.b.close()
		return after
	}
	return it
}
