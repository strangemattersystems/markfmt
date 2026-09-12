// Package markdown parses Markdown into a lossless concrete syntax tree.
package markdown

// Parse parses src into a [Tree].
func Parse(src []byte) *Tree {
	b := newBuilder(src)
	b.open(Document)
	it := newLines(src)
	if it.pos > 0 {
		b.leaf(BOM, it.pos)
	}
	for l, ok := it.next(); ok; l, ok = it.next() {
		if l.end > l.start {
			b.leaf(Text, l.end)
		}
		if l.eol > l.end {
			b.leaf(LineEnding, l.eol)
		}
	}
	b.close()
	return b.finish()
}
