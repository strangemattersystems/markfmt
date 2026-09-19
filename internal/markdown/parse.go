// Package markdown parses Markdown into a lossless concrete syntax tree.
package markdown

import (
	"bytes"
	"fmt"
)

// Parse parses src into a [Tree]. Pass 1 finds the link reference
// definitions, so that pass 2 can resolve references to them. Every
// definition contains "]:", so an input without it needs no pass 1.
func Parse(src []byte) *Tree {
	return parse(src, nil)
}

// parse is [Parse]. When trace is not nil, pass 2 calls it with each line
// that continues a paragraph, with the number of block quotes, list items and
// footnote definitions that the line matched, and with each line that starts
// a paragraph and no footnote definition. Each call also gives the column
// where the prefixes of the line's containers end.
func parse(src []byte, trace func(l line, matched, end int, continuation bool)) *Tree {
	var defs definitions
	if bytes.Contains(src, []byte("]:")) {
		parseBlocks(src, &defs, true, nil)
	}
	t := parseBlocks(src, &defs, false, trace)
	defs.finish()
	t.spans = t.findDialectSpans()
	return t
}

// parseBlocks runs the block phase over src. Pass 1 skips the inline phase,
// and drops its nodes whenever only the document is open at a line boundary,
// so it holds one top-level block at a time.
func parseBlocks(src []byte, defs *definitions, pass1 bool, trace func(line, int, int, bool)) *Tree {
	// Pass 1 drops its nodes at each top-level boundary, so it stays small.
	// Pass 2 keeps them: the corpora hold about 0.12 nodes per input byte.
	nodes := 0
	if !pass1 {
		nodes = len(src)/6 + 16
	}
	p := blockParser{b: newBuilder(src, nodes), src: src, defs: defs, pass1: pass1, trace: trace}
	p.inline = inlineParser{b: p.b, src: src, defs: defs}
	p.b.open(Document)
	p.containers = append(p.containers, container{kind: Document})
	it := newLines(src)
	if it.pos > 0 {
		p.b.leaf(BOM, it.pos)
	}
	it = p.frontMatter(it)
	for l, ok := it.next(); ok; l, ok = it.next() {
		p.parseLine(l)
		if pass1 && len(p.containers) == 1 && p.leaf.kind == noLeaf {
			p.b.tree.nodes = p.b.tree.nodes[:1]
		}
	}
	p.closeUnmatched(1)
	p.b.close()
	return p.b.finish()
}

// definitions is the link label list and the footnote label list: the
// normalized labels of the link reference definitions and of the footnote
// definitions that pass 1 finds, in document order. Pass 2 finds the same
// definitions, or panics: a mismatch is a parser bug.
type definitions struct {
	labels  []string
	defined map[string]bool
	checked int // definitions that pass 2 found

	footnotes        []string
	footnoteDefined  map[string]bool
	footnotesChecked int
}

// add records the normalized label of a definition that pass 1 finds, or
// checks the label of one that pass 2 finds.
func (d *definitions) add(label []byte, pass1 bool) {
	if pass1 {
		if d.defined == nil {
			d.defined = make(map[string]bool)
		}
		l := string(label)
		d.labels = append(d.labels, l)
		d.defined[l] = true
		return
	}
	if d.checked == len(d.labels) || d.labels[d.checked] != string(label) {
		panic(fmt.Sprintf("markdown: pass 2 found definition %d with label %q, which pass 1 did not find", d.checked, label))
	}
	d.checked++
}

// addFootnote records the normalized label of a footnote definition that pass
// 1 finds, or checks the label of one that pass 2 finds.
func (d *definitions) addFootnote(label []byte, pass1 bool) {
	if pass1 {
		if d.footnoteDefined == nil {
			d.footnoteDefined = make(map[string]bool)
		}
		l := string(label)
		d.footnotes = append(d.footnotes, l)
		d.footnoteDefined[l] = true
		return
	}
	if d.footnotesChecked == len(d.footnotes) || d.footnotes[d.footnotesChecked] != string(label) {
		panic(fmt.Sprintf("markdown: pass 2 found footnote definition %d with label %q, which pass 1 did not find", d.footnotesChecked, label))
	}
	d.footnotesChecked++
}

// finish panics when pass 2 found fewer definitions or footnote definitions
// than pass 1.
func (d *definitions) finish() {
	if d.checked != len(d.labels) {
		panic(fmt.Sprintf("markdown: pass 2 found %d definitions, and pass 1 found %d", d.checked, len(d.labels)))
	}
	if d.footnotesChecked != len(d.footnotes) {
		panic(fmt.Sprintf("markdown: pass 2 found %d footnote definitions, and pass 1 found %d", d.footnotesChecked, len(d.footnotes)))
	}
}
