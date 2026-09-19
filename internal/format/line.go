package format

import (
	"bytes"
	"slices"

	"github.com/strangemattersystems/markfmt/internal/markdown"
)

// column returns the output column that the next byte of the open line
// starts at. Only prefixes and indentation are written before the content of
// a line, and they hold no byte that is wider than one column.
func (p *printer) column() int {
	return len(p.out) - p.lineOffset
}

// rest returns the prefix that container f writes on the lines after its
// first: its marker for a block quote, 4 columns for a footnote definition
// (appendix B, trap 18), and the columns of its marker for a list item.
func (f *frame) rest() []byte {
	switch f.kind {
	case markdown.BlockQuote:
		return quotePrefix
	case markdown.FootnoteDefinition:
		return spaces[:4]
	}
	return spaces[:len(f.marker)]
}

// write appends b to the output. At the output limit it stops, so a printer
// bug cannot build more than max bytes.
func (p *printer) write(b []byte) {
	if p.full || len(p.out)+len(b) > p.max {
		p.full = true
		return
	}
	if i := bytes.LastIndexByte(b, '\n'); i >= 0 {
		p.lineOffset = len(p.out) + i + 1
	}
	p.out = append(p.out, b...)
}

// writeLF writes b with each CRLF and CR as LF.
func (p *printer) writeLF(b []byte) {
	for {
		i := bytes.IndexByte(b, '\r')
		if i < 0 {
			p.write(b)
			return
		}
		p.write(b[:i])
		p.write(lineFeed)
		b = b[i+1:]
		if len(b) > 0 && b[0] == '\n' {
			b = b[1:]
		}
	}
}

func (p *printer) writeSpaces(n int) {
	for ; n > len(spaces); n -= len(spaces) {
		p.write(spaces)
	}
	if n > 0 {
		p.write(spaces[:n])
	}
}

// trimSpaces removes the spaces at the end of the output after offset start.
func (p *printer) trimSpaces(start int) {
	if !p.full {
		p.out = p.out[:start+len(bytes.TrimRight(p.out[start:], " "))]
	}
}

// writePrefix writes the prefixes of the open containers at the start of a
// line: the marker of a container on its first line, and the rest of each
// container that the input line matched, so that a lazy line stays lazy. A
// blank line gets the prefixes without trailing spaces.
func (p *printer) writePrefix(blank bool) {
	start, n, opened := len(p.out), 0, 0
prefixes:
	for i := range p.stack {
		f := &p.stack[i]
		if !f.container {
			continue
		}
		marker, rest := f.marker, f.rest()
		switch {
		case !f.started && blank:
			break prefixes
		case !f.started:
			if opened >= 99 && f.kind != markdown.BlockQuote {
				// GitHub starts no list item or footnote definition after 99
				// blocks on a line (appendix B, trap 21).
				p.nextPrefixLine(start, i)
				start, opened = len(p.out), 0
			}
			col := p.column()
			p.write(marker)
			f.started = true
			opened++
			// An item whose marker line is blank continues one column after its
			// marker (spec 5.2); every other container after its prefix.
			f.outContent = p.column()
			if f.keepBlank {
				f.outContent = col + len(bytes.TrimRight(marker, " ")) + 1
			}
			// Padding would take the columns that the content starts with, the
			// item keeps its blank marker line, or the indentation of a kept
			// marker after it would pad its marker.
			if f.blankFirst && f.kind != markdown.BlockQuote && (p.lead || f.keepBlank || p.indentedMarkerAfter(i)) {
				p.nextPrefixLine(start, i+1)
				start, opened = len(p.out), 0
			}
		case blank || n < p.matched:
			p.write(rest)
		default:
			break prefixes
		}
		n++
	}
	if blank {
		p.trimSpaces(start)
	}
}

// indentedMarkerAfter reports whether the next container after frame i has not
// started and has a kept marker that starts with its indentation.
func (p *printer) indentedMarkerAfter(i int) bool {
	for j := i + 1; j < len(p.stack); j++ {
		if c := &p.stack[j]; c.container {
			return !c.started && len(c.marker) > 0 && c.marker[0] == ' '
		}
	}
	return false
}

// nextPrefixLine ends the line of prefixes that starts at offset start, and
// writes the rest of each container in the first end frames of the stack.
func (p *printer) nextPrefixLine(start, end int) {
	if p.full {
		return
	}
	p.trimSpaces(start)
	p.write(lineFeed)
	for j := range p.stack[:end] {
		if p.stack[j].container {
			p.write(p.stack[j].rest())
		}
	}
}

// continuation sets the prefix and the indentation of a paragraph line whose
// first written leaf is id. The line gets the prefixes of every open
// container, unless it is lazy in the input, those prefixes are longer than
// the line (appendix B, trap 2), and it starts no block as a lazy line. It
// gets 4 columns of indentation when its content would otherwise start a
// block (trap 1).
func (p *printer) continuation(id markdown.NodeID) {
	// A hard break of spaces can print as a backslash, so the decisions read
	// the line without a last backslash and with one, and a second format
	// decides the same.
	line := bytes.TrimSuffix(bytes.TrimRight(p.tree.RestOfLine(id), " \t"), []byte{'\\'})
	p.lineBreak = append(append(p.lineBreak[:0], line...), '\\')
	withBreak := p.lineBreak
	interrupts := func(lazy bool) bool {
		return markdown.InterruptsParagraph(line, lazy) || markdown.InterruptsParagraph(withBreak, lazy)
	}
	p.indent = -1
	if p.matched < p.prefixes && !interrupts(true) {
		// The line stays lazy when its canonical prefix is longer than its
		// printed content, which endLine knows (appendix B, trap 2). With
		// the prefix, the line gets the padding of trap 1.
		p.lazyLine, p.lazyPad = true, interrupts(false)
		return
	}
	p.matched = p.prefixes
	if interrupts(false) {
		p.pad = 4
	}
}

func (p *printer) endLine() {
	p.indent = -1
	if p.lineStart {
		p.writePrefix(true)
	}
	p.prefixLazyLine()
	p.write(lineFeed)
	p.lineStart = true
}

// prefixLazyLine writes the prefixes that the open lazy line does not have
// before its content, unless its full canonical prefix is longer than that
// content (appendix B, trap 2). The content is a measure of the printer's own
// output, so a second format decides the same (design 12).
func (p *printer) prefixLazyLine() {
	if !p.lazyLine {
		return
	}
	p.lazyLine = false
	if p.full || len(p.out)-p.lazyStart < p.prefixLen {
		return
	}
	prefix, n := make([]byte, 0, p.prefixLen), 0
	for i := range p.stack {
		if !p.stack[i].container {
			continue
		}
		if n >= p.lazyKept {
			prefix = append(prefix, p.stack[i].rest()...)
		}
		n++
	}
	if p.lazyPad {
		prefix = append(prefix, spaces[:4]...)
	}
	if len(p.out)+len(prefix) > p.max {
		p.full = true
		return
	}
	p.out = slices.Insert(p.out, p.lazyStart, prefix...)
	p.matched = p.prefixes
}
