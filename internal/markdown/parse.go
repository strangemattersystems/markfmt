// Package markdown parses Markdown into a lossless concrete syntax tree.
package markdown

// Parse parses src into a [Tree].
func Parse(src []byte) *Tree {
	p := blockParser{b: newBuilder(src), src: src}
	p.b.open(Document)
	it := newLines(src)
	if it.pos > 0 {
		p.b.leaf(BOM, it.pos)
	}
	for l, ok := it.next(); ok; l, ok = it.next() {
		p.line(l)
	}
	p.closeParagraph()
	p.b.close()
	return p.b.finish()
}
