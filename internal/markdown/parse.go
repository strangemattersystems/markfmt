// Package markdown parses Markdown into a lossless concrete syntax tree.
package markdown

// Parse parses src into a [Tree].
func Parse(src []byte) *Tree {
	p := blockParser{b: newBuilder(src), src: src}
	p.inline = inlineParser{b: p.b, src: src}
	p.b.open(Document)
	p.containers = append(p.containers, container{kind: Document})
	it := newLines(src)
	if it.pos > 0 {
		p.b.leaf(BOM, it.pos)
	}
	it = p.frontMatter(it)
	for l, ok := it.next(); ok; l, ok = it.next() {
		p.parseLine(l)
	}
	p.closeUnmatched(1)
	p.b.close()
	return p.b.finish()
}
