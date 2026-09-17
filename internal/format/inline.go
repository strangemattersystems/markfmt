package format

import (
	"bytes"
	"cmp"
	"slices"
	"strings"

	"github.com/strangemattersystems/markfmt/internal/markdown"
)

// inlineFrame is the state of an open inline node: the delimiter of emphasis
// or strikethrough, the bytes of a code span, and the part of an inline link
// or image that prints, with the brackets of its text, whether its
// destination is in angle brackets, and its title quotes. A collapsed or
// shortcut reference keeps the bytes of its text, which is its label.
type inlineFrame struct {
	delim    []byte
	code     []byte
	codeDone bool
	tail     tailPart
	brackets int
	label    bool
	angle    bool
	gap      bool // whitespace follows the opening parenthesis of the tail
	under    bool // emphasis that prints with '_'
	quotes   [2]byte
}

// isInline reports whether a node of kind k has a frame in inlines.
func isInline(k markdown.Kind) bool {
	switch k {
	case markdown.CodeSpan, markdown.Link, markdown.Image, markdown.Emphasis, markdown.Strong,
		markdown.Strikethrough:
		return true
	}
	return false
}

// defPart is the part of a link reference definition that prints.
type defPart uint8

// titleQuotes returns the quotes around the title of link reference
// definition id: '"', or a single quote when the title has '"', or
// parentheses when it has both, or the input's quotes when it also has a
// parenthesis. A title decodes the same in each (spec 4.7).
func (p *printer) titleQuotes(id markdown.NodeID) [2]byte {
	t := p.tree
	end, _ := t.Next(id)
	var title []byte
	var source [2]byte
	quotes := 0
	for i := id + 1; i < end; i++ {
		if !t.Kind(i).Leaf() {
			// A link in an image's text has its own title.
			next, _ := t.Next(i)
			i = next - 1
			continue
		}
		switch t.Kind(i) {
		case markdown.Title:
			title = append(title, t.Raw(i)...)
		case markdown.TitleQuote:
			if quotes < 2 {
				source[quotes] = t.Raw(i)[0]
				quotes++
			}
		}
	}
	switch {
	case bytes.IndexByte(title, '"') < 0:
		return [2]byte{'"', '"'}
	case bytes.IndexByte(title, '\'') < 0:
		return [2]byte{'\'', '\''}
	case !bytes.ContainsAny(title, "()"):
		return [2]byte{'(', ')'}
	}
	return source
}

// definitionLeaf prints leaf id of kind k, whose own columns start at column
// start, in the open link reference definition: the label as written, one
// space, the destination as written, and one space and the title in its
// quotes. The whitespace and line endings between them are not written.
func (p *printer) definitionLeaf(id markdown.NodeID, k markdown.Kind, start int) {
	switch p.def {
	case defNone:
	case defLabel:
		switch k {
		case markdown.Indent:
			// Indentation in a label is not meaning, unless it keeps the rest of
			// a line that continues the label from starting a block, which would
			// end the definition. separate pads its first line instead.
			if !p.written || !p.indentHides(id) {
				p.indent = -1
			}
		case markdown.VerbatimLineEnding:
			p.endLine()
		default:
			p.content(id, start)
			if k == markdown.Colon {
				p.def = defDestination
			}
		}
	case defDestination:
		if k == markdown.Whitespace || k == markdown.LineEnding || k == markdown.Indent {
			return
		}
		if !p.spaced {
			p.write(spaces[:1])
			p.spaced = true
		}
		p.indent = -1
		p.content(id, start)
		switch {
		case k == markdown.AngleBracket && !p.defAngle:
			p.defAngle = true
		case k == markdown.AngleBracket, k == markdown.Destination && !p.defAngle:
			p.def = defTitle
		}
	case defTitle:
		if k == markdown.TitleQuote {
			p.write(spaces[:1])
			p.indent, p.replace = -1, p.defQuotes[:1]
			p.content(id, start)
			p.def = defInTitle
		}
	case defInTitle:
		switch k {
		case markdown.VerbatimLineEnding:
			p.endLine()
		case markdown.TitleQuote:
			p.indent, p.replace = -1, p.defQuotes[1:]
			p.content(id, start)
			p.def = defEnd
		default:
			p.indent = -1
			p.content(id, start)
		}
	case defEnd:
	}
}

// delimiter returns the delimiter of node id of kind k, which is emphasis,
// strong emphasis or strikethrough: '_', "**" or "~~" (roadmap Decisions),
// or nil to keep the input's. A node keeps its input delimiters wherever the
// canonical delimiters could pair differently (spec 6.2), or could change a
// construct that starts next to them. Each check below gives its reason.
func (p *printer) delimiter(id markdown.NodeID, k markdown.Kind) []byte {
	t := p.tree
	raw, open := t.Raw(id), t.Raw(id+1)
	var closer []byte
	end, _ := t.Next(id)
	_, contentStart := t.NodeSpan(id + 1)
	_, contentEnd := t.NodeSpan(id)
	for i := end - 1; i > id; i-- {
		if t.Kind(i) == markdown.Delimiter {
			closer = t.Raw(i)
			contentEnd, _ = t.NodeSpan(i)
			break
		}
	}
	content := raw[len(open) : len(raw)-len(closer)]
	if t.Kind(id+2) == markdown.Autolink || len(content) >= 4 && bytes.EqualFold(content[:4], []byte("www.")) {
		// An extended www autolink depends on the byte before it (design 6.2).
		return nil
	}
	// An extended autolink can start in the word before the node, which its
	// bytes continue, or in the last word of its content (design 6.2).
	nodeStart, _ := t.NodeSpan(id)
	if p.autolinkWord(0, nodeStart) || p.autolinkWord(contentStart, contentEnd) {
		// An underscore in the last two segments of a domain keeps an
		// extended autolink from forming (design 6.2), so '*' would make one,
		// of the word before the node or of the word that its content ends
		// with.
		return nil
	}
	before, after := t.Around(id)
	if before == '$' || after == '$' || len(content) > 0 && (content[0] == '$' || content[len(content)-1] == '$') {
		// No character next to '$' changes (appendix B, trap 16).
		return nil
	}
	switch k {
	case markdown.Emphasis:
		// After a '<' in text, '_' could start an attribute name of a tag.
		if bytes.ContainsAny(content, "*_") || !flanksLikeSpace(before) || !flanksLikeSpace(after) || p.inText('_') || p.inText('<') {
			return nil
		}
		if p.inUnder > 0 && (!spaceLike(before) || !spaceLike(after)) {
			// Inside emphasis that prints with '_', a '_' run that
			// punctuation flanks can open and close, so it could pair with
			// the runs around it (spec 6.2).
			return nil
		}
		return []byte{'_'}
	case markdown.Strong:
		// Next to '*' or '_', which can be the unused part of a delimiter run
		// (design 6.4), "**" could pair differently.
		if bytes.ContainsAny(content, "*_") || before == '*' || before == '_' || after == '*' || after == '_' || p.inText('*') {
			return nil
		}
		return []byte("**")
	default:
		// No '~' goes next to a "~~" delimiter (appendix B, trap 13), and
		// "~~" inside a strikethrough could close it.
		if bytes.IndexByte(content, '~') >= 0 || before == '~' || after == '~' || p.inText('~') || p.inStrike > 0 {
			return nil
		}
		return []byte("~~")
	}
}

// lastWord returns the bytes of b after its last whitespace.
func lastWord(b []byte) []byte {
	return b[bytes.LastIndexAny(b, " \t\n\r")+1:]
}

// autolinkWordBytes reports whether word holds "://" or "www.", the start of
// an extended autolink (design 6.2). The printer asks this of its output,
// where no input offsets exist.
func autolinkWordBytes(word []byte) bool {
	if bytes.Contains(word, []byte("://")) {
		return true
	}
	for i := 0; i+4 <= len(word); i++ {
		if (word[i] == 'w' || word[i] == 'W') && bytes.EqualFold(word[i:i+4], []byte("www.")) {
			return true
		}
	}
	return false
}

// span is a range of input bytes.
type span struct{ start, end uint32 }

// autolinkWord reports whether the word that ends at input offset end holds
// "://" or "www.", the start of an extended autolink (design 6.2). The word
// starts after the last whitespace before end, and never before low.
//
// The offsets come from one pass over the text block, because reading the
// word for each node of a block costs O(n^2).
func (p *printer) autolinkWord(low, end uint32) bool {
	p.wordFacts()
	if i, _ := slices.BinarySearch(p.wordSpaces, end); i > 0 && p.wordSpaces[i-1] >= low {
		low = p.wordSpaces[i-1] + 1
	}
	i, _ := slices.BinarySearchFunc(p.wordStarts, low, func(s span, low uint32) int {
		return cmp.Compare(s.start, low)
	})
	// The starts are in order, so a later one ends later than this one.
	return i < len(p.wordStarts) && p.wordStarts[i].end <= end
}

// wordFacts fills wordSpaces and wordStarts for the text block being
// printed.
func (p *printer) wordFacts() {
	if p.wordScope == p.textBlock {
		return
	}
	raw := p.tree.Raw(p.textBlock)
	low, _ := p.tree.NodeSpan(p.textBlock)
	p.wordScope = p.textBlock
	p.wordSpaces, p.wordStarts = p.wordSpaces[:0], p.wordStarts[:0]
	for i, c := range raw {
		at := low + uint32(i)
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			p.wordSpaces = append(p.wordSpaces, at)
		case c == ':' && bytes.HasPrefix(raw[i:], []byte("://")):
			p.wordStarts = append(p.wordStarts, span{at, at + 3})
		case (c == 'w' || c == 'W') && len(raw)-i >= 4 && bytes.EqualFold(raw[i:i+4], []byte("www.")):
			p.wordStarts = append(p.wordStarts, span{at, at + 4})
		}
	}
}

// inText reports whether the last paragraph, heading or table cell that
// started has a text leaf with c, which is '*', '_', '~' or '<'. A canonical
// delimiter could pair with a run of c that is text, where the input's
// delimiter does not, and '_' next to a '<' could start an attribute name of
// a tag.
func (p *printer) inText(c byte) bool {
	t := p.tree
	if scope := p.textBlock; scope != p.delimScope {
		p.delimScope, p.delimText = scope, [4]bool{}
		end, _ := t.Next(scope)
		for j := scope + 1; j < end; j++ {
			if t.Kind(j) == markdown.Text {
				for k, d := range []byte("*_~<") {
					p.delimText[k] = p.delimText[k] || bytes.IndexByte(t.Raw(j), d) >= 0
				}
			}
		}
	}
	return p.delimText[strings.IndexByte("*_~<", c)]
}

// spaceLike reports whether c, the byte next to an emphasis delimiter, is
// the start or the end of the input, a space, a tab or a line ending.
func spaceLike(c byte) bool {
	switch c {
	case 0, ' ', '\t', '\n', '\r':
		return true
	}
	return false
}

// flanksLikeSpace reports whether c, the byte next to an emphasis
// delimiter, lets '_' flank as '*' does: 0 for the start or the end of the
// input, a space, a tab, a line ending, or ASCII punctuation other than '*',
// '_' and '\\'.
func flanksLikeSpace(c byte) bool {
	switch c {
	case 0, ' ', '\t', '\n', '\r':
		return true
	case '*', '_', '\\':
		return false
	}
	return strings.IndexByte("!\"#$%&'()+,-./:;<=>?@[]^`{|}~", c) >= 0
}

// codeSpan returns the bytes of code span id in the canonical style, or nil
// to keep its input bytes when it spans lines or holds a cell pipe escape.
// The fence is the shortest run of backticks that its value does not hold,
// with a space inside each end when the input has one there, or the value
// starts or ends with a backtick, or starts and ends with a space and is not
// only spaces (spec 6.1, appendix B, trap 9). Padding the input has stays: a
// space can keep the text around the code span from forming a link
// destination or definition, which hold no space (spec 6.3, 4.7).
func (p *printer) codeSpan(id markdown.NodeID) []byte {
	t := p.tree
	end, _ := t.Next(id)
	var value []byte
	for i := id + 1; i < end; i++ {
		switch t.Kind(i) {
		case markdown.CodeText:
			value = append(value, t.Raw(i)...)
		case markdown.CodeFence:
		default:
			return nil
		}
	}
	padded := len(value) >= 2 && value[0] == ' ' && value[len(value)-1] == ' ' && len(bytes.Trim(value, " ")) > 0
	if padded {
		value = value[1 : len(value)-1]
	}
	// runs[n] reports whether the value holds a run of n backticks. append
	// writes false, so the buffer of the last code span is not read again.
	p.runs = p.runs[:0]
	for run, i := 0, 0; i <= len(value); i++ {
		if i < len(value) && value[i] == '`' {
			run++
			continue
		}
		for len(p.runs) <= run {
			p.runs = append(p.runs, false)
		}
		p.runs[run], run = true, 0
	}
	runs := func(n int) bool { return n < len(p.runs) && p.runs[n] }
	n := 1
	for runs(n) {
		n++
	}
	fence := bytes.Repeat([]byte{'`'}, n)
	out := append([]byte(nil), fence...)
	pad := padded || len(value) > 0 && (value[0] == '`' || value[len(value)-1] == '`' ||
		value[0] == ' ' && value[len(value)-1] == ' ' && len(bytes.Trim(value, " ")) > 0)
	if pad {
		out = append(out, ' ')
	}
	out = append(out, value...)
	if pad {
		out = append(out, ' ')
	}
	return append(out, fence...)
}

// tailPart is the part of an inline link or image that prints.
type tailPart uint8

// tailLeaf prints leaf id of kind k, whose own columns start at column
// start, that is a child of the inline link or image in: its text, and a
// tail of the destination as written and one space and the title in its
// quotes. The whitespace, indentation and line endings between them are not
// written.
func (p *printer) tailLeaf(in *inlineFrame, id markdown.NodeID, k markdown.Kind, start int) {
	skip := k == markdown.Whitespace || k == markdown.LineEnding || k == markdown.Indent
	switch in.tail {
	case tailNone:
	case tailText:
		p.content(id, start)
		if k == markdown.Bracket {
			in.brackets++
			if in.brackets == 2 {
				in.tail = tailOpen
			}
		}
	case tailOpen:
		p.content(id, start)
		in.tail = tailDestination
	case tailDestination:
		if !skip && in.gap {
			// A destination holds no space, so without one here a
			// destination that starts before the link could run through it
			// (spec 6.3).
			in.gap = false
			p.write(spaces[:1])
		}
		switch {
		case k == markdown.LineEnding && p.inText('<'):
			// A destination in angle brackets holds no line ending, so where
			// the text has a '<' a space here could let one form across the
			// line. The line ending stays.
			in.gap = false
			p.indent = -1
			p.endLine()
		case skip:
			in.gap = true
		case k == markdown.Paren:
			p.indent = -1
			p.content(id, start)
			in.tail = tailEnd
		default:
			p.indent = -1
			p.content(id, start)
			if k == markdown.AngleBracket && !in.angle {
				in.angle = true
			} else if k == markdown.Destination && !in.angle || k == markdown.AngleBracket {
				in.tail = tailTitle
			}
		}
	case tailTitle:
		switch {
		case skip:
			in.gap = true
		case k == markdown.TitleQuote:
			in.gap = false
			p.write(spaces[:1])
			p.indent, p.replace = -1, in.quotes[:1]
			p.content(id, start)
			in.tail = tailInTitle
		default:
			// After a backslash that is not an escape, the parenthesis would
			// be an escape. Whitespace before it stays for the reason of
			// tailDestination.
			if k == markdown.Paren && (p.backslash || in.gap) {
				p.write(spaces[:1])
			}
			p.indent = -1
			p.content(id, start)
			in.tail = tailEnd
		}
	case tailInTitle:
		switch k {
		case markdown.VerbatimLineEnding:
			p.endLine()
		case markdown.TitleQuote:
			p.indent, p.replace = -1, in.quotes[1:]
			p.content(id, start)
			in.tail = tailEnd
		default:
			p.indent = -1
			p.content(id, start)
		}
	case tailEnd:
		if !skip {
			p.indent = -1
			p.content(id, start)
		}
	}
}

// codeFence returns the fence of code block id: backticks, or tildes when its
// info string has a backtick, one more than the longest run of that
// character in the code and at least 3 (roadmap Decisions). It also reports
// whether the input's code block has a fence line.
func (p *printer) codeFence(id markdown.NodeID) ([]byte, bool) {
	t := p.tree
	end, _ := t.Next(id)
	char, fenced := byte('`'), false
	for i := id + 1; i < end; i++ {
		switch t.Kind(i) {
		case markdown.FenceMarker:
			fenced = true
		case markdown.InfoString:
			if bytes.IndexByte(t.Raw(i), '`') >= 0 {
				char = '~'
			}
		}
	}
	longest := 2
	for i := id + 1; i < end; i++ {
		if t.Kind(i) != markdown.CodeText {
			continue
		}
		run := 0
		for _, c := range t.Raw(i) {
			run++
			if c != char {
				run = 0
			}
			longest = max(longest, run)
		}
	}
	return bytes.Repeat([]byte{char}, longest+1), fenced
}

// codeLeaf prints leaf id of kind k, whose own columns start at column
// start, in the open code block: the info string of the opening fence line,
// and the lines of code as their value.
func (p *printer) codeLeaf(id markdown.NodeID, k markdown.Kind, start int) {
	switch {
	case p.opening && k == markdown.InfoString:
		p.indent = -1
		if p.tree.Raw(id)[0] == p.fence[0] {
			// Without a space, the fence would take the info string's first
			// characters.
			p.write(spaces[:1])
		}
		p.content(id, start)
	case p.opening && k == markdown.LineEnding:
		p.opening = false
		p.endLine()
	case p.opening:
	case k == markdown.CodeText:
		// The code's value leaves out the input's indentation.
		p.indent = -1
		p.content(id, start)
	case k == markdown.VerbatimLineEnding:
		p.lineOpen = false
		p.endLine()
	case k == markdown.FenceMarker:
		// The input's closing fence line.
		p.lineOpen = false
	}
}
