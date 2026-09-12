// Package markdown parses Markdown into a lossless concrete syntax tree.
package markdown

// Parse parses src into a [Tree].
func Parse(src []byte) *Tree {
	b := newBuilder(src)
	b.open(Document)
	if len(src) > 0 {
		b.leaf(Text, b.size)
	}
	b.close()
	return b.finish()
}
