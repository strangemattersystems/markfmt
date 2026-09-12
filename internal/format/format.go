// Package format rewrites Markdown source in the markfmt canonical style.
package format

import (
	"bytes"
	"errors"
	"strings"

	"github.com/yuin/goldmark/v2/ast"
	"github.com/yuin/goldmark/v2/extension"
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

	doc := markdown.Parse(body)
	formatted := printBlocks(body, doc)

	var want, got bytes.Buffer
	if err := htmlRenderer.Render(&want, body, doc); err != nil {
		return nil, err
	}
	if err := htmlRenderer.Render(&got, formatted, markdown.Parse(formatted)); err != nil {
		return nil, err
	}
	if !bytes.Equal(want.Bytes(), got.Bytes()) {
		return nil, errMeaningChanged
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
	// Copying needs every block position. Without one, leave src unchanged.
	for n := range doc.Children() {
		if n.Pos() < 0 {
			return src
		}
	}

	var out bytes.Buffer
	for n := range doc.Children() {
		if out.Len() > 0 {
			out.WriteByte('\n')
		}
		switch n := n.(type) {
		case *ast.Heading:
			printHeading(&out, src, n)
		case *ast.Paragraph:
			printParagraph(&out, src, n)
		default:
			out.Write(blockSource(src, n))
		}
		out.WriteByte('\n')
	}
	return out.Bytes()
}

func printHeading(out *bytes.Buffer, src []byte, h *ast.Heading) {
	segs := h.Source()
	if len(segs) > 1 {
		// ponytail: multi-line setext headings are copied; an ATX heading
		// cannot hold the line break.
		out.Write(blockSource(src, h))
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
	for i, seg := range p.Source() {
		line := seg.Bytes(src)
		// ponytail: indentation stays on a line that could start a block
		// without it. Replace with escapes when the inline printer lands.
		if i > 0 && len(line) > 0 && strings.IndexByte("#>-+*=_`~<|:0123456789", line[0]) >= 0 {
			line = src[lineStart(src, seg.Start):seg.Stop]
		}
		text = append(text, line...)
	}
	out.Write(bytes.TrimRight(text, " \t\r\n"))
}

// blockSource returns the source lines of the top-level block n, without
// trailing blank lines.
func blockSource(src []byte, n ast.Node) []byte {
	end := len(src)
	if next := n.NextSibling(); next != nil {
		end = lineStart(src, next.Pos())
	}
	return trimBlankLines(src[lineStart(src, n.Pos()):end])
}

func lineStart(src []byte, pos int) int {
	return bytes.LastIndexByte(src[:pos], '\n') + 1
}

func trimBlankLines(b []byte) []byte {
	for len(b) > 0 {
		i := bytes.LastIndexByte(b, '\n')
		if len(bytes.TrimSpace(b[i+1:])) > 0 {
			break
		}
		b = b[:max(i, 0)]
	}
	return b
}
