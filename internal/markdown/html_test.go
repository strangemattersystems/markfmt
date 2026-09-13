package markdown

import (
	"bytes"
	"cmp"
	"html"
	"slices"
	"strconv"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"
)

func TestRenderHTML(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  string
		want string
	}{
		{"escapes text", `a<&>"'b`, "<p>a&lt;&amp;&gt;&quot;'b</p>\n"},
		{"writes thematic breaks", "***\n", "<hr />\n"},
		{"writes headings", "## a\n", "<h2>a</h2>\n"},
		{"writes indented code", "    <a>\n\n     b", "<pre><code>&lt;a&gt;\n\n b\n</code></pre>\n"},
		{"writes fenced code", "```a b\n<\n```", "<pre><code class=\"language-a\">&lt;\n</code></pre>\n"},
		{"writes decoded info strings", "~~~a\\+b&ouml;\x00 c\nx\n~~~", "<pre><code class=\"language-a+bö\ufffd\">x\n</code></pre>\n"},
		{"writes html blocks", "<div>\n  <a>\n", "<div>\n  <a>\n"},
		{"writes block quotes", "> a\n", "<blockquote>\n<p>a</p>\n</blockquote>\n"},
		{"writes tight lists", "- a\n- b\n", "<ul>\n<li>a</li>\n<li>b</li>\n</ul>\n"},
		{"writes loose ordered lists", "3. a\n\n4. b", "<ol start=\"3\">\n<li><p>a</p>\n</li>\n<li><p>b</p>\n</li>\n</ol>\n"},
		{"writes nothing for link reference definitions", "[a]: /u\n", ""},
		{"writes escapes", "\\*\\<\\a", "<p>*&lt;\\a</p>\n"},
		{"writes u+fffd for nul and invalid utf-8", "a\x00\xffb\xe2\x82", "<p>a\ufffd\ufffdb\ufffd</p>\n"},
		{"writes entity references", "&ouml;&NotEqualTilde;&#0;&#xD800;&#1114112;&#x10FFFF;&amp;", "<p>ö\u2242\u0338\ufffd\ufffd\ufffd\U0010ffff&amp;</p>\n"},
		{"writes code spans", "` a `` `\n``\nb\n`` ` ` `  `", "<p><code>a ``</code>\n<code>b</code> <code> </code> <code>  </code></p>\n"},
		{"writes line breaks", "a\\\nb  \nc \nd  ", "<p>a<br />\nb<br />\nc\nd</p>\n"},
		{"writes paragraphs", "\xEF\xBB\xBFa\r\n b\n \nc", "<p>a\nb</p>\n<p>c</p>\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := renderHTML(Parse([]byte(tt.src))); got != tt.want {
				t.Fatalf("renderHTML of %q = %q, want %q", tt.src, got, tt.want)
			}
		})
	}
}

func TestNormalizeHTML(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		html string
		want string
	}{
		{"collapses whitespace", "<p>a  \t\nb</p>", "<p>a b</p>"},
		{"collapses unicode whitespace", "\u00a0x \u2003 y", " x y"},
		{"removes whitespace around block tags", "\n\t<p>\n\t\ta  b\t\t</p>\n\t", "<p>a b</p>"},
		{"removes whitespace between blocks", "<li>\n<p>x</p>\n</li>", "<li><p>x</p></li>"},
		{"keeps whitespace around inline tags", "<em> a </em> ", "<em> a </em> "},
		{"keeps whitespace in pre", "<p>a</p>\n<pre>  x\n  y</pre>\n", "<p>a</p><pre>  x\n  y</pre>"},
		{"removes line endings after br", "<br />\n\nfoo", "<br>foo"},
		{"writes self-closing tags as start tags", "a\n<hr />\nb", "a<hr>b"},
		{"sorts and lowercases attributes", `<a title="bar" HREF="foo">x</a>`, `<a href="foo" title="bar">x</a>`},
		{"reads attribute forms", `<a B=C b=a x="1"y z=>`, `<a b="C" b="a" x="1" y z="">`},
		{"escapes attribute values", `<A HREF='&quot;x&amp;y&lt;' title="a>b">t</A>`, `<a href="&quot;x&amp;y&lt;" title="a&gt;b">t</a>`},
		{"decodes character references", "&forall;&amp;&gt;&lt;&quot;&#39;&#x27;", "\u2200&amp;&gt;&lt;&quot;''"},
		{"keeps references that are not html 4", "&Dcaron; &foo bar &#x110000;", "&Dcaron; &foo; bar &x110000;"},
		{"keeps text that is not a reference", "&#12a; &amp", "&#12a; &amp"},
		{"drops a < that no > follows", "a<b", "ab"},
		{"keeps a < that is not a tag", "x <> y <3 >", "x <> y <3 >"},
		{"keeps comments, declarations and processing instructions", "<!DOCTYPE html><?php x ?><p>x <!-- a > b --> y</p>", "<!DOCTYPE html><?php x ?><p>x <!-- a > b --> y</p>"},
		{"writes other declarations as comments", "<!ELEMENT x>", "<!--ELEMENT x-->"},
		{"keeps cdata", "<![CDATA[x < y]]>z", "<![CDATA[x < y]]>z"},
		{"keeps script content", "<script>a<b&amp;</script>", "<script>a<b&amp;</script>"},
		{"reads end tags", "</ foo></a b></>", "</foo></a>"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := normalizeHTML(tt.html); got != tt.want {
				t.Fatalf("normalizeHTML(%q) = %q, want %q", tt.html, got, tt.want)
			}
		})
	}
}

var htmlEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")

// renderHTML renders tree as HTML, as cmark does, for conformance tests.
func renderHTML(tree *Tree) string {
	var b strings.Builder
	var open []NodeID // entered interior nodes, innermost last
	c := tree.Walk()
	for e, ok := c.Next(); ok; e, ok = c.Next() {
		n := tree.nodes[e.ID]
		if e.Exit {
			open = open[:len(open)-1]
		}
		//exhaustive:enforce
		switch n.kind {
		case Document, BOM, BlankLine, Indent, LineEnding, TrailingSpace, HardBreakMarker, CodeFence, ThematicRun, ATXMarker, ATXClose, Whitespace,
			CodeIndent, CodeText, VerbatimLineEnding, FenceMarker, InfoString, SetextUnderline, HTMLText, QuoteMarker, ListMarker, ItemIndent,
			LinkReferenceDefinition, LinkLabel, Destination, Title, Bracket, Colon, AngleBracket, TitleQuote,
			FrontMatter, FrontMatterFence, FrontMatterText:
		case CodeBlock:
			if e.Exit {
				break
			}
			b.WriteString("<pre><code")
			if info := string(tree.AppendInfo(nil, e.ID)); info != "" {
				word := info
				if i := strings.IndexAny(info, " \t\n\v\f\r"); i >= 0 {
					word = info[:i]
				}
				b.WriteString(` class="language-` + htmlEscaper.Replace(word) + `"`)
			}
			b.WriteString(">" + htmlEscaper.Replace(string(tree.AppendCode(nil, e.ID))) + "</code></pre>\n")
		case BlockQuote:
			if e.Exit {
				b.WriteString("</blockquote>\n")
			} else {
				b.WriteString("<blockquote>\n")
			}
		case HTMLBlock:
			if !e.Exit {
				b.Write(tree.AppendHTML(nil, e.ID))
			}
		case List:
			switch start, ordered := tree.ListStart(e.ID); {
			case !ordered && e.Exit:
				b.WriteString("</ul>\n")
			case !ordered:
				b.WriteString("<ul>\n")
			case e.Exit:
				b.WriteString("</ol>\n")
			case start != 1:
				b.WriteString(`<ol start="` + strconv.Itoa(start) + "\">\n")
			default:
				b.WriteString("<ol>\n")
			}
		case ListItem:
			b.WriteString(tag("li", e.Exit))
		case Paragraph:
			// A paragraph in an item of a tight list has no tags.
			if len(open) < 2 || tree.Kind(open[len(open)-1]) != ListItem || tree.ListLoose(open[len(open)-2]) {
				b.WriteString(tag("p", e.Exit))
			}
		case Heading:
			b.WriteString(tag("h"+strconv.Itoa(tree.HeadingLevel(e.ID)), e.Exit))
		case ThematicBreak:
			if !e.Exit {
				b.WriteString("<hr />\n")
			}
		case Text, Escape, EntityRef:
			b.WriteString(htmlEscaper.Replace(string(tree.AppendValue(nil, e.ID))))
		case CodeSpan:
			if !e.Exit {
				b.WriteString("<code>" + htmlEscaper.Replace(string(tree.AppendCodeSpan(nil, e.ID))) + "</code>")
			}
		case SoftBreak:
			if !e.Exit {
				b.WriteByte('\n')
			}
		case HardBreak:
			if !e.Exit {
				b.WriteString("<br />\n")
			}
		}
		if !e.Exit && n.kind.class() == classStructure {
			open = append(open, e.ID)
		}
	}
	return b.String()
}

// tag returns the start tag of an element, or its end tag and a line ending.
func tag(name string, end bool) string {
	if end {
		return "</" + name + ">\n"
	}
	return "<" + name + ">"
}

// normalizeHTML returns s in the normal form of cmark's test/normalize.py, so
// that renderings that differ only in insignificant ways are equal.
// Whitespace is collapsed outside <pre> and removed around block tags,
// self-closing tags become start tags, attributes are sorted, and character
// references are decoded, except to <, >, & and ".
//
// normalize.py builds on Python's HTMLParser. This scanner follows its
// output, not its code.
func normalizeHTML(s string) string {
	n := htmlNormalizer{last: htmlStartTag}
	// normalize.py splits its input into chunks at markup first, and a "<"
	// that no ">" follows is in no chunk.
	lastGT := strings.LastIndexByte(s, '>')
	for i := 0; i < len(s); {
		switch {
		case s[i] == '<' && i > lastGT:
			i++
		case s[i] == '<':
			i += n.markup(s[i:])
		case s[i] == '&':
			i += n.reference(s[i:])
		default:
			j := strings.IndexAny(s[i:], "<&")
			if j < 0 {
				j = len(s) - i
			}
			n.data(s[i : i+j])
			i += j
		}
	}
	return string(n.out)
}

type htmlToken uint8

const (
	htmlOther htmlToken = iota
	htmlStartTag
	htmlEndTag
)

type htmlNormalizer struct {
	out     []byte
	last    htmlToken
	lastTag string
	inPre   bool
}

type htmlAttr struct {
	name, value string
	hasValue    bool
}

// markup reads the markup at the start of s, which starts with "<" and has a
// ">", and returns its length.
func (n *htmlNormalizer) markup(s string) int {
	gt := strings.IndexByte(s, '>')
	switch {
	case strings.HasPrefix(s, "<![CDATA["):
		if end := strings.Index(s, "]]>") + 3; end >= 3 {
			// normalize.py writes a CDATA section on one line without parsing it.
			n.out = append(n.out, s[:end]...)
			if strings.Contains(s[:end], "\n") {
				n.last = htmlOther
			}
			return end
		}
	case strings.HasPrefix(s, "<!--"):
		if end := strings.Index(s[4:], "-->"); end >= 0 {
			n.raw(s[:4+end+3])
			return 4 + end + 3
		}
	case strings.HasPrefix(s, "<?"):
		n.raw(s[:gt+1])
		return gt + 1
	case strings.HasPrefix(s, "</>"):
		return 3
	case strings.HasPrefix(s, "</"):
		name := strings.TrimLeft(s[2:gt], " \t\n\r\f")
		if end := strings.IndexAny(name, " \t\n\r\f/\x00"); end >= 0 {
			name = name[:end]
		}
		if name == "" {
			n.raw("<!--" + s[2:gt] + "-->")
		} else {
			n.endTag(strings.ToLower(name))
		}
		return gt + 1
	case len(s) > 1 && isASCIILetter(s[1]):
		if end := n.startTag(s); end > 0 {
			return end
		}
	case strings.HasPrefix(s, "<!"):
		if len(s) >= 9 && strings.EqualFold(s[2:9], "doctype") {
			n.raw(s[:gt+1])
		} else {
			n.raw("<!--" + s[2:gt] + "-->")
		}
		return gt + 1
	}
	n.data("<")
	return 1
}

// startTag reads the start tag at the start of s and returns its length, or
// 0 when s does not start with a whole start tag. The content of a script or
// style element is part of its start tag.
func (n *htmlNormalizer) startTag(s string) int {
	j := 1
	for j < len(s) && strings.IndexByte("\t\n\r\f />\x00", s[j]) < 0 {
		j++
	}
	name := strings.ToLower(s[1:j])
	var attrs []htmlAttr
	for {
		for j < len(s) && (isHTMLSpace(s[j]) || s[j] == '/' && !strings.HasPrefix(s[j:], "/>")) {
			j++
		}
		switch {
		case j == len(s):
			return 0
		case s[j] == '>' || strings.HasPrefix(s[j:], "/>"):
			selfClosing := s[j] == '/'
			n.writeStartTag(name, attrs, selfClosing)
			end := j + 1
			if selfClosing {
				return end + 1
			}
			if name == "script" || name == "style" {
				content := indexFold(s[end:], "</"+name)
				if content < 0 {
					content = len(s) - end
				}
				n.data(s[end : end+content])
				end += content
			}
			return end
		}
		// An attribute follows whitespace, "/" or a quote.
		if strings.IndexByte(" \t\n\r\f\v/\"'", s[j-1]) < 0 {
			return 0
		}
		k := j + 1
		for k < len(s) && !isHTMLSpace(s[k]) && strings.IndexByte("/=>", s[k]) < 0 {
			k++
		}
		a := htmlAttr{name: strings.ToLower(s[j:k])}
		j = k
		v := k
		for v < len(s) && isHTMLSpace(s[v]) {
			v++
		}
		if v < len(s) && s[v] == '=' {
			for v < len(s) && (s[v] == '=' || isHTMLSpace(s[v])) {
				v++
			}
			if v < len(s) && (s[v] == '"' || s[v] == '\'') {
				if end := strings.IndexByte(s[v+1:], s[v]); end >= 0 {
					a.value, a.hasValue, j = s[v+1:v+1+end], true, v+1+end+1
				}
			} else {
				end := v
				for end < len(s) && s[end] != '>' && !isHTMLSpace(s[end]) {
					end++
				}
				a.value, a.hasValue, j = s[v:end], true, end
			}
		}
		attrs = append(attrs, a)
	}
}

// reference reads the character reference or "&" at the start of s and
// returns its length.
func (n *htmlNormalizer) reference(s string) int {
	if strings.HasPrefix(s, "&#") {
		digits, base := 2, 10
		if len(s) > 2 && (s[2] == 'x' || s[2] == 'X') {
			digits, base = 3, 16
		}
		end := digits
		for end < len(s) && isDigit(s[end], base) {
			end++
		}
		// A numeric reference ends before a byte that is not a hexadecimal
		// digit.
		if end == digits || end == len(s) || isDigit(s[end], 16) {
			n.data("&#")
			return 2
		}
		if v, err := strconv.ParseUint(s[digits:end], base, 32); err == nil && v <= unicode.MaxRune {
			n.char(string(rune(v)))
		} else {
			n.char("&" + s[2:end] + ";")
		}
		if s[end] == ';' {
			end++
		}
		return end
	}

	end := 1
	if len(s) > 1 && isASCIILetter(s[1]) {
		end = 2
		for end < len(s) && (isDigit(s[end], 10) || isASCIILetter(s[end]) || s[end] == '-' || s[end] == '.') {
			end++
		}
		if end == len(s) {
			// A named reference ends before a byte that is not a letter or a
			// digit, so the name gives back its last "-" or ".".
			end = 2 + strings.LastIndexAny(s[2:], "-.")
		}
	}
	if end == 1 {
		n.data("&")
		return 1
	}
	if name := s[1:end]; html4Entities[name] {
		n.char(html.UnescapeString("&" + name + ";"))
	} else {
		n.char("&" + name + ";")
	}
	if s[end] == ';' {
		end++
	}
	return end
}

func (n *htmlNormalizer) data(s string) {
	afterTag := n.last == htmlStartTag || n.last == htmlEndTag
	if afterTag && n.lastTag == "br" {
		s = strings.TrimLeft(s, "\n")
	}
	if !n.inPre {
		var b strings.Builder
		space := false
		for i, r := range s {
			if isPythonSpace(r) {
				if !space {
					b.WriteByte(' ')
				}
				space = true
				continue
			}
			space = false
			_, size := utf8.DecodeRuneInString(s[i:])
			b.WriteString(s[i : i+size])
		}
		s = b.String()
		if afterTag && htmlBlockTags[n.lastTag] {
			s = strings.TrimLeftFunc(s, isPythonSpace)
			if n.last == htmlEndTag {
				s = strings.TrimRightFunc(s, isPythonSpace)
			}
		}
	}
	n.out = append(n.out, s...)
	n.last = htmlOther
}

func (n *htmlNormalizer) writeStartTag(name string, attrs []htmlAttr, selfClosing bool) {
	if name == "pre" {
		n.inPre = true
	}
	if htmlBlockTags[name] {
		n.out = bytes.TrimRightFunc(n.out, isPythonSpace)
	}
	n.out = append(n.out, "<"+name...)
	slices.SortFunc(attrs, func(a, b htmlAttr) int {
		return cmp.Or(strings.Compare(a.name, b.name), strings.Compare(a.value, b.value))
	})
	for _, a := range attrs {
		n.out = append(n.out, " "+a.name...)
		if a.hasValue {
			n.out = append(n.out, `="`+htmlAttrEscaper.Replace(html.UnescapeString(a.value))+`"`...)
		}
	}
	n.out = append(n.out, '>')
	n.lastTag = name
	n.last = htmlStartTag
	if selfClosing {
		n.last = htmlEndTag
	}
}

func (n *htmlNormalizer) endTag(name string) {
	if name == "pre" {
		n.inPre = false
	} else if htmlBlockTags[name] {
		n.out = bytes.TrimRightFunc(n.out, isPythonSpace)
	}
	n.out = append(n.out, "</"+name+">"...)
	n.lastTag = name
	n.last = htmlEndTag
}

func (n *htmlNormalizer) raw(s string) {
	n.out = append(n.out, s...)
	n.last = htmlOther
}

func (n *htmlNormalizer) char(c string) {
	switch c {
	case "<":
		c = "&lt;"
	case ">":
		c = "&gt;"
	case "&":
		c = "&amp;"
	case `"`:
		c = "&quot;"
	}
	n.raw(c)
}

var htmlAttrEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&#x27;")

// htmlBlockTags are the tags around which normalize.py removes whitespace.
var htmlBlockTags = map[string]bool{
	"article": true, "header": true, "aside": true, "hgroup": true, "blockquote": true,
	"hr": true, "iframe": true, "body": true, "li": true, "map": true, "button": true,
	"object": true, "canvas": true, "ol": true, "caption": true, "output": true,
	"col": true, "p": true, "colgroup": true, "pre": true, "dd": true, "progress": true,
	"div": true, "section": true, "dl": true, "table": true, "td": true, "dt": true,
	"tbody": true, "embed": true, "textarea": true, "fieldset": true, "tfoot": true,
	"figcaption": true, "th": true, "figure": true, "thead": true, "footer": true,
	"tr": true, "form": true, "ul": true, "h1": true, "h2": true, "h3": true, "h4": true,
	"h5": true, "h6": true, "video": true, "script": true, "style": true,
}

// html4Entities are the names of the HTML 4 character entities. normalize.py
// decodes a named reference only when its name is one of them.
var html4Entities = func() map[string]bool {
	m := make(map[string]bool)
	for name := range strings.FieldsSeq(html4EntityNames) {
		m[name] = true
	}
	return m
}()

const html4EntityNames = `
AElig Aacute Acirc Agrave Alpha Aring Atilde Auml Beta Ccedil Chi Dagger
Delta ETH Eacute Ecirc Egrave Epsilon Eta Euml Gamma Iacute Icirc Igrave
Iota Iuml Kappa Lambda Mu Ntilde Nu OElig Oacute Ocirc Ograve Omega
Omicron Oslash Otilde Ouml Phi Pi Prime Psi Rho Scaron Sigma THORN Tau
Theta Uacute Ucirc Ugrave Upsilon Uuml Xi Yacute Yuml Zeta aacute acirc
acute aelig agrave alefsym alpha amp and ang aring asymp atilde auml bdquo
beta brvbar bull cap ccedil cedil cent chi circ clubs cong copy crarr cup
curren dArr dagger darr deg delta diams divide eacute ecirc egrave empty
emsp ensp epsilon equiv eta eth euml euro exist fnof forall frac12 frac14
frac34 frasl gamma ge gt hArr harr hearts hellip iacute icirc iexcl igrave
image infin int iota iquest isin iuml kappa lArr lambda lang laquo larr
lceil ldquo le lfloor lowast loz lrm lsaquo lsquo lt macr mdash micro
middot minus mu nabla nbsp ndash ne ni not notin nsub ntilde nu oacute
ocirc oelig ograve oline omega omicron oplus or ordf ordm oslash otilde
otimes ouml para part permil perp phi pi piv plusmn pound prime prod prop
psi quot rArr radic rang raquo rarr rceil rdquo real reg rfloor rho rlm
rsaquo rsquo sbquo scaron sdot sect shy sigma sigmaf sim spades sub sube
sum sup sup1 sup2 sup3 supe szlig tau there4 theta thetasym thinsp thorn
tilde times trade uArr uacute uarr ucirc ugrave uml upsih upsilon uuml
weierp xi yacute yen yuml zeta zwj zwnj
`

func isDigit(c byte, base int) bool {
	return '0' <= c && c <= '9' || base == 16 && 'a' <= c|0x20 && c|0x20 <= 'f'
}

func isHTMLSpace(c byte) bool {
	return strings.IndexByte(" \t\n\r\f\v", c) >= 0
}

// isPythonSpace reports whether Python's str.isspace is true for r.
func isPythonSpace(r rune) bool {
	return unicode.IsSpace(r) || 0x1c <= r && r <= 0x1f
}

// indexFold returns the index of the first instance of substr in s, with
// ASCII case folding, or -1.
func indexFold(s, substr string) int {
	for i := 0; i+len(substr) <= len(s); i++ {
		if strings.EqualFold(s[i:i+len(substr)], substr) {
			return i
		}
	}
	return -1
}
