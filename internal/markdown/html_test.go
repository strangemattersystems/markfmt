package markdown

import (
	"bytes"
	"cmp"
	"html"
	"regexp"
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
		{"writes an info word that starts with language- as its class", "```language-r\nx\n```", "<pre><code class=\"language-r\">x\n</code></pre>\n"},
		{"writes decoded info strings", "~~~a\\+b&ouml;\x00 c\nx\n~~~", "<pre><code class=\"language-a+bö\ufffd\">x\n</code></pre>\n"},
		{"writes a line ending before a block after a start tag, as cmark does", "- <div>\n\n  a\n> # b", "<ul>\n<li>\n<div>\n<p>a</p>\n</li>\n</ul>\n<blockquote>\n<h1>b</h1>\n</blockquote>\n"},
		{"writes html blocks", "<div>\n  <a>\n", "<div>\n  <a>\n"},
		{"writes block quotes", "> a\n", "<blockquote>\n<p>a</p>\n</blockquote>\n"},
		{"writes tight lists", "- a\n- b\n", "<ul>\n<li>a</li>\n<li>b</li>\n</ul>\n"},
		{"writes loose ordered lists", "3. a\n\n4. b", "<ol start=\"3\">\n<li>\n<p>a</p>\n</li>\n<li>\n<p>b</p>\n</li>\n</ol>\n"},
		{"writes nothing for link reference definitions", "[a]: /u\n", ""},
		{"writes nothing for footnote definitions without references", "[^a]: b\n\nc", "<p>c</p>\n"},
		{"writes escapes", "\\*\\<\\a", "<p>*&lt;\\a</p>\n"},
		{"writes u+fffd for nul and invalid utf-8", "a\x00\xffb\xe2\x82", "<p>a\ufffd\ufffdb\ufffd</p>\n"},
		{"writes entity references", "&ouml;&NotEqualTilde;&#0;&#xD800;&#1114112;&#x10FFFF;&amp;", "<p>ö\u2242\u0338\ufffd\ufffd\ufffd\U0010ffff&amp;</p>\n"},
		{"writes code spans", "` a `` `\n``\nb\n`` ` ` `  `", "<p><code>a ``</code>\n<code>b</code> <code> </code> <code>  </code></p>\n"},
		{"writes autolinks", "<https://a.b/\\[&amp;\u00e9'> <A@b.c>", "<p><a href=\"https://a.b/%5C%5B&amp;%C3%A9&#x27;\">https://a.b/\\[&amp;\u00e9'</a> <a href=\"mailto:A@b.c\">A@b.c</a></p>\n"},
		{"writes extended email autolinks", "a\\_b@c.de mailto:x@y.zz xmpp:p@q.rr/s", "<p><a href=\"mailto:a_b@c.de\">a_b@c.de</a> <a href=\"mailto:x@y.zz\">mailto:x@y.zz</a> <a href=\"xmpp:p@q.rr/s\">xmpp:p@q.rr/s</a></p>\n"},
		{"writes extended autolinks with their text as written", "www.a.com/b&c http://d.e/\\_f", "<p><a href=\"http://www.a.com/b&amp;c\">www.a.com/b&amp;c</a> <a href=\"http://d.e/%5C_f\">http://d.e/\\_f</a></p>\n"},
		{"writes raw html", "a <b\n c='d'>e<!---->", "<p>a <b\nc='d'>e<!----></p>\n"},
		{"writes emphasis", "*a* __b__", "<p><em>a</em> <strong>b</strong></p>\n"},
		{"writes strikethrough", "~a~ ~~b~~", "<p><del>a</del> <del>b</del></p>\n"},
		{"writes cell pipe escapes in text, a code span, a destination and an autolink", "| \\| | `\\\\|` | [a](\\\\|) | <http://a\\|b> |\n|-|-|-|-|", "<table>\n<thead>\n<tr>\n<th>|</th>\n<th><code>\\|</code></th>\n<th><a href=\"%7C\">a</a></th>\n<th><a href=\"http://a%7Cb\">http://a|b</a></th>\n</tr>\n</thead>\n</table>\n"},
		{"writes task list items", "- [ ] a\n- [x] b\n\n1. [X] c\n\n   d", "<ul>\n<li><input type=\"checkbox\" disabled=\"\" /> a</li>\n<li><input type=\"checkbox\" checked=\"\" disabled=\"\" /> b</li>\n</ul>\n<ol>\n<li>\n<p><input type=\"checkbox\" checked=\"\" disabled=\"\" /> c</p>\n<p>d</p>\n</li>\n</ol>\n"},
		{"writes footnote references and the footnote section as cmark-gfm does", "a[^x] b[^y] c[^x] [^z]\n\n[^y]: Y\n\n    Z\n[^x]: X\n", "<p>a<sup class=\"footnote-ref\"><a href=\"#fn-x\" id=\"fnref-x\" data-footnote-ref>1</a></sup> b<sup class=\"footnote-ref\"><a href=\"#fn-y\" id=\"fnref-y\" data-footnote-ref>2</a></sup> c<sup class=\"footnote-ref\"><a href=\"#fn-x\" id=\"fnref-x-2\" data-footnote-ref>1</a></sup> [^z]</p>\n<section class=\"footnotes\" data-footnotes>\n<ol>\n<li id=\"fn-x\">\n<p>X <a href=\"#fnref-x\" class=\"footnote-backref\" data-footnote-backref data-footnote-backref-idx=\"1\" aria-label=\"Back to reference 1\">↩</a> <a href=\"#fnref-x-2\" class=\"footnote-backref\" data-footnote-backref data-footnote-backref-idx=\"1-2\" aria-label=\"Back to reference 1-2\">↩<sup class=\"footnote-ref\">2</sup></a></p>\n</li>\n<li id=\"fn-y\">\n<p>Y</p>\n<p>Z <a href=\"#fnref-y\" class=\"footnote-backref\" data-footnote-backref data-footnote-backref-idx=\"2\" aria-label=\"Back to reference 2\">↩</a></p>\n</li>\n</ol>\n</section>\n"},
		{"writes tables with missing cells and without cells beyond the header count", "| a | b |\n| :-: | - |\n| c |\n| d | e | f |", "<table>\n<thead>\n<tr>\n<th align=\"center\">a</th>\n<th>b</th>\n</tr>\n</thead>\n<tbody>\n<tr>\n<td align=\"center\">c</td>\n<td></td>\n</tr>\n<tr>\n<td align=\"center\">d</td>\n<td>e</td>\n</tr>\n</tbody>\n</table>\n"},
		{"writes links and images", "[a *b*](/u&amp; \"t\\\"\") ![c *d* `e`\n<f>](g 'h')", "<p><a href=\"/u&amp;\" title=\"t&quot;\">a <em>b</em></a> <img src=\"g\" alt=\"c d e &lt;f&gt;\" title=\"h\" /></p>\n"},
		{"writes reference links", "[a][B] [b][] [b] ![b]\n\n[B]: /u \"t\"\n[b]: /v", "<p><a href=\"/u\" title=\"t\">a</a> <a href=\"/u\" title=\"t\">b</a> <a href=\"/u\" title=\"t\">b</a> <img src=\"/u\" alt=\"b\" title=\"t\" /></p>\n"},
		{"writes line breaks", "a\\\nb  \nc \nd  ", "<p>a<br />\nb<br />\nc\nd</p>\n"},
		{"writes paragraphs", "\xEF\xBB\xBFa\r\n b\n \nc", "<p>a\nb</p>\n<p>c</p>\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := renderHTML(Parse([]byte(tt.src)), false); got != tt.want {
				t.Fatalf("renderHTML of %q = %q, want %q", tt.src, got, tt.want)
			}
		})
	}

	t.Run("applies the tag filter to every tag in an html block and to the start of raw html", func(t *testing.T) {
		t.Parallel()

		for _, tt := range []struct{ src, want string }{
			{"<div><title>a</title>\n<TEXTAREA x><xmp/><titles><style", "<div>&lt;title>a&lt;/title>\n&lt;TEXTAREA x>&lt;xmp/><titles>&lt;style\n"},
			{"a <script x='<title>'> </style>", "<p>a &lt;script x='<title>'> &lt;/style></p>\n"},
		} {
			if got := renderHTML(Parse([]byte(tt.src)), true); got != tt.want {
				t.Errorf("renderHTML of %q with the tag filter = %q, want %q", tt.src, got, tt.want)
			}
		}
	})
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

func TestNormalizeGitHub(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		html string
		want string
	}{
		{"writes a space for each br, which mode gfm writes for soft breaks", "<p>a<br>\nb<br />\nc</p>", "<p>a b c</p>"},
		{"removes the table wrapper and role", `<markdown-accessiblity-table><table role="table"><tr><td>a</td></tr></table></markdown-accessiblity-table>`, "<table><tr><td>a</td></tr></table>"},
		{"removes notranslate classes", `<pre class="notranslate"><code class="notranslate">a</code></pre>`, "<pre><code>a</code></pre>"},
		{"removes rel attributes", `<a href="https://a.b" rel="nofollow">a</a>`, `<a href="https://a.b">a</a>`},
		{"removes the link and style around an image", `<a target="_blank" rel="noopener noreferrer" href="/i"><img src="/i" alt="a" style="max-width: 100%;"></a>`, `<img alt="a" src="/i">`},
		{"removes task list classes, ids and labels", `<ul class="contains-task-list"><li class="task-list-item"><input type="checkbox" id="" disabled="" class="task-list-item-checkbox" aria-label="Completed task" checked=""> a</li></ul>`, `<ul><li><input checked="" disabled="" type="checkbox"> a</li></ul>`},
		{"removes the footnote heading and section class", `<section data-footnotes="" class="footnotes"><h2 id="footnote-label" class="sr-only">Footnotes</h2><ol></ol></section>`, "<section data-footnotes><ol></ol></section>"},
		{"removes user-content- prefixes and hashes from footnote references", `<sup><a href="#user-content-fn-1-0f1e088e7177de600f1295d2090035f7" id="user-content-fnref-1-2-0f1e088e7177de600f1295d2090035f7" data-footnote-ref="" aria-describedby="footnote-label">1</a></sup>`, `<sup><a data-footnote-ref href="#fn-1" id="fnref-1-2">1</a></sup>`},
		{"removes the footnote classes and back reference index that cmark-gfm writes", `<sup class="footnote-ref"><a href="#fn-1" id="fnref-1" data-footnote-ref>1</a></sup> <a href="#fnref-1" class="footnote-backref" data-footnote-backref data-footnote-backref-idx="1" aria-label="Back to reference 1">↩</a>`, `<sup><a data-footnote-ref href="#fn-1" id="fnref-1">1</a></sup> <a aria-label="Back to reference 1" data-footnote-backref href="#fnref-1">↩</a>`},
		{"removes the class of a github footnote back reference", `<a href="#user-content-fnref-1-0f1e088e7177de600f1295d2090035f7" data-footnote-backref="" aria-label="Back to reference 1" class="data-footnote-backref">↩</a>`, `<a aria-label="Back to reference 1" data-footnote-backref href="#fnref-1">↩</a>`},
		{"writes the lang attribute of a code block as the class of its code", `<pre lang="a b" class="notranslate"><code class="notranslate">x</code></pre>`, `<pre><code class="language-a b">x</code></pre>`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := normalizeGitHub(normalizeHTML(tt.html)); got != tt.want {
				t.Fatalf("normalizeGitHub(%q) = %q, want %q", tt.html, got, tt.want)
			}
		})
	}
}

var htmlEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")

// renderHTML renders tree as HTML, as cmark does, for conformance tests. With
// tagFilter, raw HTML passes through the GFM tag filter.
func renderHTML(tree *Tree, tagFilter bool) string {
	var b strings.Builder
	var open []NodeID // entered interior nodes, innermost last
	var plain NodeID  // the image whose alt text is being written, or 0
	var skip NodeID   // the table cell beyond the header count that is not written, or 0
	var aligns []Alignment
	var columns, rows, cells int // of the table and of the row being written
	var body, header bool        // the table has a body, and the row is its header row
	// cr starts a line unless one is started, as cmark does before a block.
	cr := func() {
		if out := b.String(); out != "" && out[len(out)-1] != '\n' {
			b.WriteByte('\n')
		}
	}
	defs := make(map[string]NodeID) // the first definition of each label
	for i, n := range tree.nodes {
		if n.kind != LinkReferenceDefinition {
			continue
		}
		if label := string(tree.AppendLabel(nil, NodeID(i))); defs[label] == 0 {
			defs[label] = NodeID(i)
		}
	}
	// target returns the node that holds the destination and title of link
	// or image id.
	target := func(id NodeID) NodeID {
		if tree.LinkForm(id) == InlineLink {
			return id
		}
		return defs[string(tree.AppendLinkLabel(nil, id))]
	}
	// cmark-gfm numbers the footnote definitions in the order of their first
	// resolved reference, counts the references to each, and writes the
	// definitions with a reference in that order at the end.
	footnoteDefs := make(map[string]NodeID) // the first definition of each label
	footnoteIx := make(map[NodeID]int)      // the number of a definition with a reference
	footnoteRefs := make(map[NodeID]int)    // the references to a definition
	footnoteRefIx := make(map[NodeID]int)   // the number of a reference among the references to its definition
	var footnotes []NodeID                  // the definitions with a reference, in number order
	footnoteOf := func(id NodeID) NodeID {
		return footnoteDefs[string(tree.AppendFootnoteReferenceLabel(nil, id, true))]
	}
	for i, n := range tree.nodes {
		if n.kind == FootnoteDefinition {
			f := labelFolder{}
			f.write(tree.FootnoteDefinitionLabel(NodeID(i)))
			if label := string(f.dst); footnoteDefs[label] == 0 {
				footnoteDefs[label] = NodeID(i)
			}
		}
	}
	for i, n := range tree.nodes {
		if n.kind != FootnoteReference || !tree.FootnoteReferenceResolved(NodeID(i)) {
			continue
		}
		def := footnoteOf(NodeID(i))
		if footnoteIx[def] == 0 {
			footnotes = append(footnotes, def)
			footnoteIx[def] = len(footnotes)
		}
		footnoteRefs[def]++
		footnoteRefIx[NodeID(i)] = footnoteRefs[def]
	}
	var section NodeID // the definition whose list item is being written, or 0
	var backref bool   // the back references of section are written
	backrefs := func(def NodeID) string {
		label := escapeHref(string(tree.FootnoteDefinitionLabel(def)))
		m := strconv.Itoa(footnoteIx[def])
		var s strings.Builder
		s.WriteString(`<a href="#fnref-` + label + `" class="footnote-backref" data-footnote-backref data-footnote-backref-idx="` + m + `" aria-label="Back to reference ` + m + `">↩</a>`)
		for k := 2; k <= footnoteRefs[def]; k++ {
			n := strconv.Itoa(k)
			s.WriteString(` <a href="#fnref-` + label + "-" + n + `" class="footnote-backref" data-footnote-backref data-footnote-backref-idx="` + m + "-" + n + `" aria-label="Back to reference ` + m + "-" + n + `">↩<sup class="footnote-ref">` + n + `</sup></a>`)
		}
		return s.String()
	}
	// lastBlock returns the last child block of definition def that is not a
	// definition, which cmark-gfm moves out, or 0.
	lastBlock := func(def NodeID) NodeID {
		var last NodeID
		for i := def + 1; i < NodeID(tree.nodes[def].link); i++ {
			if m := tree.nodes[i]; m.kind.class() == classStructure {
				if m.kind != FootnoteDefinition {
					last = i
				}
				i = NodeID(m.link) - 1
			}
		}
		return last
	}
	walk := func(c Cursor) {
		for e, ok := c.Next(); ok; e, ok = c.Next() {
			n := tree.nodes[e.ID]
			if e.Exit {
				open = open[:len(open)-1]
			}
			if plain != 0 {
				// Alt text is plain text, as cmark writes it.
				switch {
				case e.Exit && e.ID == plain:
					b.WriteString(`"` + titleAttr(tree, target(e.ID)) + " />")
					plain = 0
				case e.Exit:
				case n.kind == Text, n.kind == Escape, n.kind == EntityRef, n.kind == AutolinkText, n.kind == CellPipeEscape && written(n):
					b.WriteString(htmlEscaper.Replace(string(tree.AppendValue(nil, e.ID))))
				case n.kind == CodeSpan:
					b.WriteString(htmlEscaper.Replace(string(tree.AppendCodeSpan(nil, e.ID))))
				case n.kind == RawHTML:
					b.WriteString(htmlEscaper.Replace(string(tree.AppendRawHTML(nil, e.ID))))
				case n.kind == SoftBreak, n.kind == HardBreak:
					b.WriteByte(' ')
				case n.kind == FootnoteReference && !e.Exit && !tree.FootnoteReferenceResolved(e.ID):
					b.WriteString(htmlEscaper.Replace("[^" + string(tree.AppendFootnoteReferenceLabel(nil, e.ID, false)) + "]"))
				}
				if !e.Exit && n.kind.class() == classStructure {
					open = append(open, e.ID)
				}
				continue
			}
			if skip != 0 {
				if e.Exit && e.ID == skip {
					skip = 0
				}
				if !e.Exit && n.kind.class() == classStructure {
					open = append(open, e.ID)
				}
				continue
			}
			//exhaustive:enforce
			switch n.kind {
			case Document, BOM, BlankLine, Indent, LineEnding, TrailingSpace, HardBreakMarker, CodeFence, Delimiter, Paren, ThematicRun, ATXMarker, ATXClose, Whitespace,
				CodeIndent, CodeText, VerbatimLineEnding, FenceMarker, InfoString, SetextUnderline, HTMLText, QuoteMarker, ListMarker, ItemIndent,
				LinkReferenceDefinition, LinkLabel, Destination, Title, Bracket, Colon, AngleBracket, TitleQuote,
				FrontMatter, FrontMatterFence, FrontMatterText, TablePipe, TableDelimiter, FootnoteIndent, FootnoteLabel, Caret:
			case CodeBlock:
				if e.Exit {
					break
				}
				cr()
				b.WriteString("<pre><code")
				if info := string(tree.AppendInfo(nil, e.ID)); info != "" {
					word := info
					if i := strings.IndexAny(info, " \t\n\v\f\r"); i >= 0 {
						word = info[:i]
					}
					// cmark and commonmark.js write no second language- prefix.
					b.WriteString(` class="` + htmlEscaper.Replace("language-"+strings.TrimPrefix(word, "language-")) + `"`)
				}
				b.WriteString(">" + htmlEscaper.Replace(string(tree.AppendCode(nil, e.ID))) + "</code></pre>\n")
			case BlockQuote:
				if e.Exit {
					b.WriteString("</blockquote>\n")
				} else {
					cr()
					b.WriteString("<blockquote>\n")
				}
			case HTMLBlock:
				if !e.Exit {
					cr()
					b.WriteString(filterTags(tree.AppendHTML(nil, e.ID), tagFilter, true))
				}
			case List:
				if !e.Exit {
					cr()
				}
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
				if !e.Exit {
					cr()
				}
				b.WriteString(tag("li", e.Exit))
			case Paragraph:
				// A paragraph in an item of a tight list has no tags.
				if len(open) < 2 || tree.Kind(open[len(open)-1]) != ListItem || tree.ListLoose(open[len(open)-2]) {
					if !e.Exit {
						cr()
					}
					if e.Exit && section != 0 && open[len(open)-1] == section && lastBlock(section) == e.ID {
						b.WriteString(" " + backrefs(section))
						backref = true
					}
					b.WriteString(tag("p", e.Exit))
				}
			case Heading:
				if !e.Exit {
					cr()
				}
				b.WriteString(tag("h"+strconv.Itoa(tree.HeadingLevel(e.ID)), e.Exit))
			case ThematicBreak:
				if !e.Exit {
					cr()
					b.WriteString("<hr />\n")
				}
			case Autolink:
				if e.Exit {
					b.WriteString("</a>")
					break
				}
				href := string(tree.AppendAutolinkText(nil, e.ID))
				switch {
				case tree.AutolinkEmail(e.ID):
					href = "mailto:" + href
				case !tree.AutolinkAngle(e.ID) && strings.HasPrefix(href, "www."):
					href = "http://" + href
				}
				b.WriteString(`<a href="` + escapeHref(href) + `">`)
			case Text, Escape, EntityRef, AutolinkText:
				b.WriteString(htmlEscaper.Replace(string(tree.AppendValue(nil, e.ID))))
			case FootnoteDefinition:
				// cmark-gfm writes a definition only in the footnote section, and only
				// when a reference resolves to it.
				skip = e.ID
			case FootnoteReference:
				skip = e.ID
				switch def := footnoteOf(e.ID); {
				case !tree.FootnoteReferenceResolved(e.ID):
					b.WriteString(htmlEscaper.Replace("[^" + string(tree.AppendFootnoteReferenceLabel(nil, e.ID, false)) + "]"))
				default:
					label := escapeHref(string(tree.FootnoteDefinitionLabel(def)))
					id := label
					if k := footnoteRefIx[e.ID]; k > 1 {
						id += "-" + strconv.Itoa(k)
					}
					b.WriteString(`<sup class="footnote-ref"><a href="#fn-` + label + `" id="fnref-` + id + `" data-footnote-ref>` + strconv.Itoa(footnoteIx[def]) + "</a></sup>")
				}
			case CellPipeEscape:
				if written(n) {
					b.WriteString("|")
				}
			case TaskBox:
				// GitHub writes the box in the paragraph, also in a loose list.
				if _, checked := tree.ListItemTask(open[len(open)-2]); checked {
					b.WriteString(`<input type="checkbox" checked="" disabled="" /> `)
				} else {
					b.WriteString(`<input type="checkbox" disabled="" /> `)
				}
			case Link:
				if e.Exit {
					b.WriteString("</a>")
					break
				}
				b.WriteString(`<a href="` + escapeHref(string(tree.AppendDestination(nil, target(e.ID)))) + `"` + titleAttr(tree, target(e.ID)) + ">")
			case Image:
				b.WriteString(`<img src="` + escapeHref(string(tree.AppendDestination(nil, target(e.ID)))) + `" alt="`)
				plain = e.ID
			case Emphasis:
				b.WriteString(inlineTag("em", e.Exit))
			case Strong:
				b.WriteString(inlineTag("strong", e.Exit))
			case Strikethrough:
				b.WriteString(inlineTag("del", e.Exit))
			case Table:
				if e.Exit {
					if body {
						cr()
						b.WriteString("</tbody>")
					}
					cr()
					b.WriteString("</table>\n")
					break
				}
				cr()
				b.WriteString("<table>")
				columns, rows, aligns, body = tree.TableColumns(e.ID), 0, aligns[:0], false
			case TableRow:
				// cmark-gfm writes the cells missing from a row as empty cells.
				if e.Exit {
					for ; cells < columns; cells++ {
						cr()
						b.WriteString("<td" + alignAttr(aligns[cells]) + "></td>")
					}
					cr()
					b.WriteString("</tr>")
					if header {
						cr()
						b.WriteString("</thead>")
					}
					break
				}
				cr()
				header, cells = rows == 0, 0
				rows++
				switch {
				case header:
					b.WriteString("<thead>\n")
				case !body:
					b.WriteString("<tbody>\n")
					body = true
				}
				b.WriteString("<tr>")
			case TableCell:
				name := "td"
				if header {
					name = "th"
				}
				switch {
				case e.Exit:
					b.WriteString("</" + name + ">")
				case cells == columns:
					skip = e.ID
				default:
					if header {
						aligns = append(aligns, tree.CellAlignment(e.ID))
					}
					cr()
					b.WriteString("<" + name + alignAttr(tree.CellAlignment(e.ID)) + ">")
					cells++
				}
			case RawHTML:
				if !e.Exit {
					b.WriteString(filterTags(tree.AppendRawHTML(nil, e.ID), tagFilter, false))
				}
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
	}
	walk(tree.Walk())
	for _, def := range footnotes {
		if section == 0 {
			b.WriteString("<section class=\"footnotes\" data-footnotes>\n<ol>\n")
		}
		section, backref = def, false
		b.WriteString(`<li id="fn-` + escapeHref(string(tree.FootnoteDefinitionLabel(def))) + "\">\n")
		open = append(open[:0], def)
		walk(Cursor{nodes: tree.nodes[:tree.nodes[def].link], next: uint32(def) + 1})
		if !backref {
			b.WriteString(backrefs(def) + "\n")
		}
		b.WriteString("</li>\n")
	}
	if section != 0 {
		b.WriteString("</ol>\n</section>\n")
	}
	return b.String()
}

// filterTags returns raw HTML, with the GFM tag filter when filter is true.
// cmark-gfm's filter writes "&lt;" for a "<" that starts a tag of a
// disallowed element. It checks every "<" of an HTML block, and only the
// first byte of inline raw HTML.
func filterTags(raw []byte, filter, block bool) string {
	if !filter {
		return string(raw)
	}
	var b strings.Builder
	for i, c := range raw {
		if c == '<' && (block || i == 0) && isDisallowedTag(raw[i:]) {
			b.WriteString("&lt;")
			continue
		}
		b.WriteByte(c)
	}
	return b.String()
}

// isDisallowedTag reports whether s starts with a start or end tag of an
// element that the GFM tag filter disallows: "<", an optional "/", the name in
// any ASCII case, then whitespace, ">" or "/>".
func isDisallowedTag(s []byte) bool {
	i := 1
	if len(s) > 1 && s[1] == '/' {
		i = 2
	}
	for _, name := range [...]string{"title", "textarea", "style", "xmp", "iframe", "noembed", "noframes", "script", "plaintext"} {
		j := i + len(name)
		if j >= len(s) || !equalFoldASCII(s[i:j], name) {
			continue
		}
		if isHTMLSpace(s[j]) || s[j] == '>' || s[j] == '/' && j+1 < len(s) && s[j+1] == '>' {
			return true
		}
	}
	return false
}

// equalFoldASCII reports whether b is name, a lowercase ASCII word, in any
// ASCII case.
func equalFoldASCII(b []byte, name string) bool {
	for k, c := range b {
		if c|0x20 != name[k] {
			return false
		}
	}
	return true
}

// normalizeGitHub returns html, in the normal form of [normalizeHTML], without
// the decorations of the GitHub Markdown API and of cmark-gfm's footnotes, so
// that a GitHub fixture compares with the test renderer.
func normalizeGitHub(html string) string {
	for _, d := range gitHubDecorations {
		html = d.re.ReplaceAllString(html, d.with)
	}
	return normalizeHTML(html)
}

// gitHubDecorations are the rewrites of [normalizeGitHub], in order, on HTML in
// the normal form of [normalizeHTML], where attributes are sorted.
var gitHubDecorations = []struct {
	re   *regexp.Regexp
	with string
}{
	// mode=gfm writes <br> for every soft break, so no fixture tells a soft
	// break from a hard break.
	{regexp.MustCompile(`<br>`), " "},
	{regexp.MustCompile(`</?markdown-accessiblity-table>`), ""},
	{regexp.MustCompile(` (rel|role|style|aria-describedby|data-footnote-backref-idx)="[^"]*"`), ""},
	{regexp.MustCompile(` aria-label="(Incomplete|Completed) task"`), ""},
	{regexp.MustCompile(` class="(notranslate|contains-task-list|task-list-item|task-list-item-checkbox|footnotes|footnote-ref|footnote-backref|data-footnote-backref)"`), ""},
	{regexp.MustCompile(` id=""`), ""},
	{regexp.MustCompile(`<a href="[^"]*" target="_blank">(<img [^>]*>)</a>`), "$1"},
	{regexp.MustCompile(`<h2 class="sr-only" id="footnote-label">Footnotes</h2>`), ""},
	{regexp.MustCompile(` (data-footnote-ref|data-footnotes|data-footnote-backref)=""`), " $1"},
	{regexp.MustCompile(`"(#?)user-content-`), `"$1`},
	{regexp.MustCompile(`(fn(?:ref)?-[^"]*)-[0-9a-f]{32}"`), `$1"`},
	{regexp.MustCompile(`<pre lang="([^"]*)"><code>`), `<pre><code class="language-$1">`},
}

// escapeHref escapes a link destination as cmark's houdini_escape_href does:
// it keeps ASCII letters, digits and -_.+!*(),%#@?=;:/$~, writes & and ' as
// character references, and percent-encodes every other byte.
func escapeHref(s string) string {
	const hex = "0123456789ABCDEF"
	var b strings.Builder
	for i := range len(s) {
		switch c := s[i]; {
		case c == '&':
			b.WriteString("&amp;")
		case c == '\'':
			b.WriteString("&#x27;")
		case isASCIIAlphanumeric(c) || strings.IndexByte("-_.+!*(),%#@?=;:/$~", c) >= 0:
			b.WriteByte(c)
		default:
			b.Write([]byte{'%', hex[c>>4], hex[c&15]})
		}
	}
	return b.String()
}

// titleAttr returns the title attribute of link or image id, or nothing when
// its title is empty.
func titleAttr(tree *Tree, id NodeID) string {
	title := tree.AppendTitle(nil, id)
	if len(title) == 0 {
		return ""
	}
	return ` title="` + htmlEscaper.Replace(string(title)) + `"`
}

// written reports whether the test renderer writes cell pipe escape n as
// text: in text and in an autolink, and not in a destination, a title, a
// label, a code span or raw HTML, whose values it writes from their nodes.
func written(n Node) bool {
	return Kind(n.flags) == Text || Kind(n.flags) == AutolinkText
}

func alignAttr(a Alignment) string {
	switch a {
	case AlignLeft:
		return ` align="left"`
	case AlignCenter:
		return ` align="center"`
	case AlignRight:
		return ` align="right"`
	}
	return ""
}

// inlineTag returns the start or end tag of an inline element.
func inlineTag(name string, end bool) string {
	if end {
		return "</" + name + ">"
	}
	return "<" + name + ">"
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
