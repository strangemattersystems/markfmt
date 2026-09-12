// Package format rewrites Markdown source in the markfmt canonical style.
package format

import (
	"bytes"
	"errors"
	"regexp"
	"slices"
	"strings"

	"github.com/yuin/goldmark/v2/ast"
	"github.com/yuin/goldmark/v2/extension"
	east "github.com/yuin/goldmark/v2/extension/ast"
	"github.com/yuin/goldmark/v2/parser"
	"github.com/yuin/goldmark/v2/renderer/html"
)

var (
	markdown     = parser.New(parser.WithExtensions(extension.GFMParser))
	htmlRenderer = html.New(html.WithUnsafe(), html.WithExtensions(extension.GFMHTMLRenderer))
)

var errMeaningChanged = errors.New("formatted output renders differently from the input")

// Source returns src in the canonical style.
//
// Source returns an error, not output, if the output would render to
// different HTML than src.
func Source(src []byte) ([]byte, error) {
	front, body := splitFrontMatter(src)
	// The end of the input is a line ending. goldmark renders a final HTML
	// block differently without an explicit one.
	if len(body) > 0 && body[len(body)-1] != '\n' {
		body = append(slices.Clip(body), '\n')
	}

	doc := markdown.Parse(body)
	formatted := printBlocks(body, doc)
	if err := checkRendering(body, doc, formatted); err != nil {
		return nil, err
	}

	if front == nil {
		return formatted, nil
	}
	out := make([]byte, 0, len(front)+len(formatted)+2)
	out = append(out, bytes.TrimRight(front, "\r\n")...)
	out = append(out, '\n')
	if len(formatted) > 0 {
		out = append(out, '\n')
		out = append(out, formatted...)
	}
	return out, nil
}

// checkRendering returns an error if formatted renders to different HTML
// than src. doc is the parsed src.
func checkRendering(src []byte, doc ast.Node, formatted []byte) error {
	var want, got bytes.Buffer
	if err := htmlRenderer.Render(&want, src, doc); err != nil {
		return err
	}
	if err := htmlRenderer.Render(&got, formatted, markdown.Parse(formatted)); err != nil {
		return err
	}
	if !bytes.Equal(trailingSpace.ReplaceAll(want.Bytes(), []byte("</")), trailingSpace.ReplaceAll(got.Bytes(), []byte("</"))) {
		return errMeaningChanged
	}
	return nil
}

// trailingSpace matches spaces and tabs before a closing tag, which do not
// render. goldmark keeps them in a paragraph that a table interrupts.
var trailingSpace = regexp.MustCompile(`[ \t]+</`)

// splitFrontMatter splits a leading YAML (---) or TOML (+++) front matter
// block from src. Front matter is not Markdown: goldmark reads it as a
// thematic break and a setext heading, which the printer would rewrite.
func splitFrontMatter(src []byte) (front, body []byte) {
	first, rest, _ := bytes.Cut(src, []byte("\n"))
	delim := bytes.TrimRight(first, " \t\r")
	if string(delim) != "---" && string(delim) != "+++" {
		return nil, src
	}
	end := len(first) + 1
	for len(rest) > 0 {
		line, next, _ := bytes.Cut(rest, []byte("\n"))
		end = min(end+len(line)+1, len(src))
		if bytes.Equal(bytes.TrimRight(line, " \t\r"), delim) {
			return src[:end], src[end:]
		}
		rest = next
	}
	return nil, src
}

// printBlocks prints the top-level blocks of doc, one blank line apart.
// Block kinds without a printer are copied from src unchanged.
func printBlocks(src []byte, doc ast.Node) []byte {
	// Copying needs each block to start on a later line than the block before
	// it. goldmark positions do not always hold to that; then leave src
	// unchanged.
	prev := -1
	for n := range doc.Children() {
		pos := blockPos(n)
		if pos < 0 || lineStart(src, pos) <= prev {
			return src
		}
		prev = lineStart(src, pos)
	}

	var out, block bytes.Buffer
	separate := false
	for n := range doc.Children() {
		block.Reset()
		open := false
		switch n := n.(type) {
		case *ast.Heading:
			printHeading(&block, src, n)
		case *ast.Paragraph:
			printParagraph(&block, src, n)
		default:
			var b []byte
			b, open = blockSource(src, n)
			block.Write(b)
		}
		// After a blank line, an indented line that continues the previous
		// block would start an indented code block.
		if separate && (n.Kind() == ast.KindCodeBlock || !codeIndented(block.Bytes())) {
			out.WriteByte('\n')
		}
		out.Write(block.Bytes())
		out.WriteByte('\n')
		// A blank line after an open block would become its content.
		separate = !open
	}
	return out.Bytes()
}

func printHeading(out *bytes.Buffer, src []byte, h *ast.Heading) {
	segs := h.Source()
	if len(segs) > 1 {
		// ponytail: multi-line setext headings are copied; an ATX heading
		// cannot hold the line break.
		b, _ := blockSource(src, h)
		out.Write(b)
		return
	}

	out.WriteString(strings.Repeat("#", h.Level))
	if len(segs) == 0 {
		return
	}
	text := bytes.TrimRight(segs[0].Bytes(src), " \t")
	if len(text) == 0 {
		return
	}
	out.WriteByte(' ')
	// ATX reads a trailing run of '#' after a space as a closing sequence.
	content := bytes.TrimRight(text, "#")
	if len(content) < len(text) && (len(content) == 0 || content[len(content)-1] == ' ' || content[len(content)-1] == '\t') {
		out.Write(content)
		out.WriteByte('\\')
		out.Write(text[len(content):])
		return
	}
	out.Write(text)
}

func printParagraph(out *bytes.Buffer, src []byte, p *ast.Paragraph) {
	var text []byte
	for _, seg := range p.Source() {
		line := seg.Bytes(src)
		stop := seg.Stop
		// goldmark leaves a final \r out of the segment, but it is content:
		// without it, "*\r" becomes a list item.
		if !bytes.HasSuffix(line, []byte("\n")) && stop < len(src) && src[stop] == '\r' {
			stop++
			line = append(slices.Clip(line), '\r')
		}
		// ponytail: indentation stays on a line that could start a block
		// without it. Replace with escapes when the inline printer lands.
		if len(line) > 0 && strings.IndexByte("#>-+*=_`~<|:0123456789", line[0]) >= 0 {
			line = src[lineStart(src, seg.Start):stop]
		}
		text = append(text, line...)
	}
	out.Write(bytes.TrimRight(text, " \t\n"))
}

// blockSource returns the source lines of the top-level block n without the
// final line ending or trailing blank lines. If n is open, so that a blank
// line after it becomes content, blockSource keeps every line up to the next
// block and reports open.
func blockSource(src []byte, n ast.Node) (b []byte, open bool) {
	start, end := lineStart(src, blockPos(n)), len(src)
	if next := n.NextSibling(); next != nil {
		end = lineStart(src, blockPos(next))
	}
	b = trimBlankLines(src[start:end])
	// ponytail: renders each copied block twice to find an unclosed code
	// fence or HTML block. Replace with closure checks when printers own them.
	if !bytes.Equal(renderHTML(append(slices.Clip(b), '\n')), renderHTML(append(slices.Clip(b), "\n\n"...))) {
		return bytes.TrimSuffix(src[start:end], []byte("\n")), true
	}
	return b, false
}

// renderHTML returns the HTML rendering of src. A render error leaves the
// rendering incomplete, which callers see as a different rendering.
func renderHTML(src []byte) []byte {
	var b bytes.Buffer
	_ = htmlRenderer.Render(&b, src, markdown.Parse(src))
	return b.Bytes()
}

// blockPos returns the source offset where the top-level block n starts.
func blockPos(n ast.Node) int {
	// goldmark gives a table that interrupts a paragraph the position of the
	// paragraph. The header row has the correct position.
	if n.Kind() == east.KindTable && n.FirstChild() != nil {
		return n.FirstChild().Pos()
	}
	return n.Pos()
}

// codeIndented reports whether b starts with at least four columns of
// whitespace, enough to start an indented code block.
func codeIndented(b []byte) bool {
	width := 0
	for _, c := range b {
		switch c {
		case ' ':
			width++
		case '\t':
			width += 4 - width%4
		default:
			return width >= 4
		}
	}
	return width >= 4
}

func lineStart(src []byte, pos int) int {
	return bytes.LastIndexByte(src[:pos], '\n') + 1
}

func trimBlankLines(b []byte) []byte {
	for len(b) > 0 {
		i := bytes.LastIndexByte(b, '\n')
		// A blank line holds only spaces and tabs. Other Unicode space, such
		// as \f, is content.
		if len(bytes.Trim(b[i+1:], " \t\r")) > 0 {
			break
		}
		b = b[:max(i, 0)]
	}
	return b
}
