package markdown

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestParse(t *testing.T) {
	// Not parallel: the pathological subtest times Parse, and parallel tests
	// wait until TestParse's own body returns.

	tests := []struct {
		name string
		src  string
		want string
	}{
		{"gives an empty document for empty input", "", `Document{}`},
		{"gives a bom leaf", "\xEF\xBB\xBFa\n", `Document{BOM "\ufeff", Paragraph{Text "a", LineEnding "\n"}}`},
		{"joins lines into a paragraph", "a\r\n  b\rc", `Document{Paragraph{Text "a", SoftBreak{LineEnding "\r\n"}, Indent "  ", Text "b", SoftBreak{LineEnding "\r"}, Text "c"}}`},
		{"gives trailing spaces at the end of a paragraph", "a  \n", `Document{Paragraph{Text "a", TrailingSpace "  ", LineEnding "\n"}}`},
		{"gives a soft break between paragraph lines", "a \t\nb", `Document{Paragraph{Text "a", TrailingSpace " \t", SoftBreak{LineEnding "\n"}, Text "b"}}`},
		{"gives a hard break for two spaces before a line ending", "a \t  \r\n b", `Document{Paragraph{Text "a", HardBreak{HardBreakMarker " \t  ", LineEnding "\r\n"}, Indent " ", Text "b"}}`},
		{"gives a hard break for a backslash before a line ending", "a \\\nb", `Document{Paragraph{Text "a ", HardBreak{HardBreakMarker "\\", LineEnding "\n"}, Text "b"}}`},
		{"gives escape leaves for ascii punctuation", "\\*a\\b\\\\\nc\\", `Document{Paragraph{Escape "\\*", Text "a\\b", Escape "\\\\", SoftBreak{LineEnding "\n"}, Text "c\\"}}`},
		{"gives entity references", "&amp; &#35;&#X22;&CounterClockwiseContourIntegral; &nope; &#12345678; &#xabcdefa; &#;", `Document{Paragraph{EntityRef "&amp;", Text " ", EntityRef "&#35;", EntityRef "&#X22;", EntityRef "&CounterClockwiseContourIntegral;", Text " &nope; &#12345678; &#xabcdefa; &#;"}}`},
		{"gives a code span", "`` a ` b ``c", "Document{Paragraph{CodeSpan{CodeFence \"``\", CodeText \" a ` b \", CodeFence \"``\"}, Text \"c\"}}"},
		{"gives a code span over lines with their prefix and indent leaves", "> `a  \n>   b\\`", "Document{BlockQuote{QuoteMarker@1 \"> \", Paragraph{CodeSpan{CodeFence \"`\", CodeText \"a  \", VerbatimLineEnding \"\\n\", QuoteMarker@1 \"> \", Indent \"  \", CodeText \"b\\\\\", CodeFence \"`\"}}}}"},
		{"gives a backtick run with no closer of its length as text", "``a`b\n```c`", "Document{Paragraph{Text \"``a\", CodeSpan{CodeFence \"`\", CodeText \"b\", VerbatimLineEnding \"\\n\", CodeText \"```c\", CodeFence \"`\"}}}"},
		{"gives angle autolinks", "<https://a.b/c?d&amp;e> <foo@bar.example.com>` <m:abc> <a@b-.c> <a@b>`", "Document{Paragraph{Autolink{AngleBracket \"<\", AutolinkText \"https://a.b/c?d&amp;e\", AngleBracket \">\"}, Text \" \", Autolink{AngleBracket \"<\", AutolinkText \"foo@bar.example.com\", AngleBracket \">\"}, CodeSpan{CodeFence \"`\", CodeText \" <m:abc> <a@b-.c> <a@b>\", CodeFence \"`\"}}}"},
		{"gives raw html over lines with their prefix and indent leaves", "> a <b\n>  c='d\n> e'>f", "Document{BlockQuote{QuoteMarker@1 \"> \", Paragraph{Text \"a \", RawHTML{HTMLText \"<b\", VerbatimLineEnding \"\\n\", QuoteMarker@1 \"> \", Indent \" \", HTMLText \"c='d\", VerbatimLineEnding \"\\n\", QuoteMarker@1 \"> \", HTMLText \"e'>\"}, Text \"f\"}}}"},
		{"gives raw html comments, processing instructions, declarations and cdata sections", "a <!--> <!---> <!-- b -- c ---> <?x?> <!X y> <![CDATA[>]]> </d >", "Document{Paragraph{Text \"a \", RawHTML{HTMLText \"<!-->\"}, Text \" \", RawHTML{HTMLText \"<!--->\"}, Text \" \", RawHTML{HTMLText \"<!-- b -- c --->\"}, Text \" \", RawHTML{HTMLText \"<?x?>\"}, Text \" \", RawHTML{HTMLText \"<!X y>\"}, Text \" \", RawHTML{HTMLText \"<![CDATA[>]]>\"}, Text \" \", RawHTML{HTMLText \"</d >\"}}}"},
		{"gives text for a tag that does not close", "a <33> </a x> <a b='c> <a\n\nb>", "Document{Paragraph{Text \"a <33> </a x> <a b='c> <a\", LineEnding \"\\n\"}, BlankLine \"\\n\", Paragraph{Text \"b>\"}}"},
		{"gives emphasis and strong emphasis", "***a*** *b** __c__", "Document{Paragraph{Emphasis{Delimiter \"*\", Strong{Delimiter \"**\", Text \"a\", Delimiter \"**\"}, Delimiter \"*\"}, Text \" \", Emphasis{Delimiter \"*\", Text \"b\", Delimiter \"*\"}, Text \"* \", Strong{Delimiter \"__\", Text \"c\", Delimiter \"__\"}}}"},
		{"gives text for delimiter runs that are not flanking", "a * b _c_d*\n*e", "Document{Paragraph{Text \"a * b _c_d*\", SoftBreak{LineEnding \"\\n\"}, Text \"*e\"}}"},
		{"treats unicode symbols and u+fffd as punctuation for flanking", "a \u00a3_b_\u00a3 \xff_c_\xff", "Document{Paragraph{Text \"a \u00a3\", Emphasis{Delimiter \"_\", Text \"b\", Delimiter \"_\"}, Text \"\u00a3 \\xff\", Emphasis{Delimiter \"_\", Text \"c\", Delimiter \"_\"}, Text \"\\xff\"}}"},
		{"gives an inline link", "[a *b*](/u \"t\")", "Document{Paragraph{Link{Bracket \"[\", Text \"a \", Emphasis{Delimiter \"*\", Text \"b\", Delimiter \"*\"}, Bracket \"]\", Paren \"(\", Destination \"/u\", Whitespace \" \", TitleQuote \"\\\"\", Title \"t\", TitleQuote \"\\\"\", Paren \")\"}}}"},
		{"gives an image with a link tail over lines", "![a](\n<b c>\n'd\ne')", "Document{Paragraph{Image{Bracket \"![\", Text \"a\", Bracket \"]\", Paren \"(\", LineEnding \"\\n\", AngleBracket \"<\", Destination \"b c\", AngleBracket \">\", LineEnding \"\\n\", TitleQuote \"'\", Title \"d\", VerbatimLineEnding \"\\n\", Title \"e\", TitleQuote \"'\", Paren \")\"}}}"},
		{"makes the link openers before a link inactive", "[a [b](c) d](e)", "Document{Paragraph{Text \"[a \", Link{Bracket \"[\", Text \"b\", Bracket \"]\", Paren \"(\", Destination \"c\", Paren \")\"}, Text \" d](e)\"}}"},
		{"gives text for a link tail that does not close", "[a](b c d) ![e]", "Document{Paragraph{Text \"[a](b c d) ![e]\"}}"},
		{"gives no break at the end of a block", "a\\\n\n# b\\", `Document{Paragraph{Text "a\\", LineEnding "\n"}, BlankLine "\n", Heading{ATXMarker "#", Whitespace " ", Text "b\\"}}`},
		{"gives breaks between setext heading lines", "a  \nb \n==", `Document{Heading{Text "a", HardBreak{HardBreakMarker "  ", LineEnding "\n"}, Text "b", TrailingSpace " ", LineEnding "\n", SetextUnderline "=="}}`},
		{"gives the indentation of a first line", "   a", `Document{Paragraph{Indent "   ", Text "a"}}`},
		{"ends a paragraph at a blank line", "a\n\nb", `Document{Paragraph{Text "a", LineEnding "\n"}, BlankLine "\n", Paragraph{Text "b"}}`},
		{"gives one leaf per blank line", "\n \t\r\n", `Document{BlankLine "\n", BlankLine " \t\r\n"}`},
		{"gives a blank line at the end of the input", "a\n  ", `Document{Paragraph{Text "a", LineEnding "\n"}, BlankLine "  "}`},
		{"gives a thematic break", " - - -\t\n", `Document{ThematicBreak{Indent " ", ThematicRun "- - -\t", LineEnding "\n"}}`},
		{"interrupts a paragraph with a thematic break", "a\n***\nb", `Document{Paragraph{Text "a", LineEnding "\n"}, ThematicBreak{ThematicRun "***", LineEnding "\n"}, Paragraph{Text "b"}}`},
		{"needs three markers of one kind", "**\n*-*", `Document{Paragraph{Text "**", SoftBreak{LineEnding "\n"}, Emphasis{Delimiter "*", Text "-", Delimiter "*"}}}`},
		{"gives an atx heading", "## a ##  \n", `Document{Heading{ATXMarker "##", Whitespace " ", Text "a", Whitespace " ", ATXClose "##", Whitespace "  ", LineEnding "\n"}}`},
		{"keeps a closing sequence that follows text", " #\ta#", `Document{Heading{Indent " ", ATXMarker "#", Whitespace "\t", Text "a#"}}`},
		{"gives an empty atx heading", "#\n### ###", `Document{Heading{ATXMarker "#", LineEnding "\n"}, Heading{ATXMarker "###", Whitespace " ", ATXClose "###"}}`},
		{"interrupts a paragraph with an atx heading", "a\n# b", `Document{Paragraph{Text "a", LineEnding "\n"}, Heading{ATXMarker "#", Whitespace " ", Text "b"}}`},
		{"needs one to six markers and a space", "####### a\n#a", `Document{Paragraph{Text "####### a", SoftBreak{LineEnding "\n"}, Text "#a"}}`},
		{"gives a setext heading", "a\n b\n===  \n", `Document{Heading{Text "a", SoftBreak{LineEnding "\n"}, Indent " ", Text "b", LineEnding "\n", SetextUnderline "===", Whitespace "  ", LineEnding "\n"}}`},
		{"prefers a setext underline to a thematic break", "a\n  ---", `Document{Heading{Text "a", LineEnding "\n", Indent "  ", SetextUnderline "---"}}`},
		{"needs a setext underline without inner spaces", "a\n= =\n    ==", `Document{Paragraph{Text "a", SoftBreak{LineEnding "\n"}, Text "= =", SoftBreak{LineEnding "\n"}, Indent "    ", Text "=="}}`},
		{"needs a paragraph before a setext underline", "    a\n===", `Document{CodeBlock{CodeIndent "    ", CodeText "a", VerbatimLineEnding "\n"}, Paragraph{Text "==="}}`},
		{"gives indented code", "    a\n\t b\n", `Document{CodeBlock{CodeIndent "    ", CodeText "a", VerbatimLineEnding "\n", CodeIndent "\t", CodeText " b", VerbatimLineEnding "\n"}}`},
		{"keeps blank lines inside indented code", "    a\n  \n      \r\n    b", `Document{CodeBlock{CodeIndent "    ", CodeText "a", VerbatimLineEnding "\n", CodeIndent "  ", VerbatimLineEnding "\n", CodeIndent "    ", CodeText "  ", VerbatimLineEnding "\r\n", CodeIndent "    ", CodeText "b"}}`},
		{"ends indented code before trailing blank lines", "    a\n\n      \nb", `Document{CodeBlock{CodeIndent "    ", CodeText "a", VerbatimLineEnding "\n"}, BlankLine "\n", BlankLine "      \n", Paragraph{Text "b"}}`},
		{"ends indented code before blank lines at the end of the input", "    a\n  ", `Document{CodeBlock{CodeIndent "    ", CodeText "a", VerbatimLineEnding "\n"}, BlankLine "  "}`},
		{"does not interrupt a paragraph with indented code", "a\n    b", `Document{Paragraph{Text "a", SoftBreak{LineEnding "\n"}, Indent "    ", Text "b"}}`},
		{"gives fenced code", "```js x \n<\n```  ", "Document{CodeBlock{FenceMarker \"```\", InfoString \"js x\", Whitespace \" \", LineEnding \"\\n\", CodeText \"<\", VerbatimLineEnding \"\\n\", FenceMarker \"```\", Whitespace \"  \"}}"},
		{"removes the fence indentation from fenced code lines", " ~~~\n  a\n\n ~~~~\nb", "Document{CodeBlock{Indent \" \", FenceMarker \"~~~\", LineEnding \"\\n\", CodeIndent \" \", CodeText \" a\", VerbatimLineEnding \"\\n\", VerbatimLineEnding \"\\n\", Indent \" \", FenceMarker \"~~~~\", LineEnding \"\\n\"}, Paragraph{Text \"b\"}}"},
		{"closes fenced code only with a long enough fence of its character", "````\n```\n~~~~\n    ````", "Document{CodeBlock{FenceMarker \"````\", LineEnding \"\\n\", CodeText \"```\", VerbatimLineEnding \"\\n\", CodeText \"~~~~\", VerbatimLineEnding \"\\n\", CodeText \"    ````\"}}"},
		{"interrupts a paragraph with fenced code", "a\n~~~\nb", "Document{Paragraph{Text \"a\", LineEnding \"\\n\"}, CodeBlock{FenceMarker \"~~~\", LineEnding \"\\n\", CodeText \"b\"}}"},
		{"needs a backtick fence info string without backticks", "``` a`b", "Document{Paragraph{Text \"``` a`b\"}}"},
		{"gives an html block that ends at a blank line", "<div>\n*a*\n\nb", "Document{HTMLBlock[6]{HTMLText \"<div>\", VerbatimLineEnding \"\\n\", HTMLText \"*a*\", VerbatimLineEnding \"\\n\"}, BlankLine \"\\n\", Paragraph{Text \"b\"}}"},
		{"keeps blank lines in an html comment block", " <!-- a\n  \n -->b\nc", "Document{HTMLBlock[2]{HTMLText \" <!-- a\", VerbatimLineEnding \"\\n\", HTMLText \"  \", VerbatimLineEnding \"\\n\", HTMLText \" -->b\", VerbatimLineEnding \"\\n\"}, Paragraph{Text \"c\"}}"},
		{"ends an html block of kind 1 at any end tag of kind 1", "<pre>\n\n</STYLE>x\ny", "Document{HTMLBlock[1]{HTMLText \"<pre>\", VerbatimLineEnding \"\\n\", VerbatimLineEnding \"\\n\", HTMLText \"</STYLE>x\", VerbatimLineEnding \"\\n\"}, Paragraph{Text \"y\"}}"},
		{"ends html blocks of kinds 3 and 5 on their first line", "<?a?>\n<![CDATA[]]>", "Document{HTMLBlock[3]{HTMLText \"<?a?>\", VerbatimLineEnding \"\\n\"}, HTMLBlock[5]{HTMLText \"<![CDATA[]]>\"}}"},
		{"starts an html block of kind 4 with any ascii letter", "<!doctype html>", "Document{HTMLBlock[4]{HTMLText \"<!doctype html>\"}}"},
		{"interrupts a paragraph with a search html block", "a\n<search>", "Document{Paragraph{Text \"a\", LineEnding \"\\n\"}, HTMLBlock[6]{HTMLText \"<search>\"}}"},
		{"gives a complete tag an html block of kind 7", "<source src='x' a>  \n</b >\n\n<a b=c/>", "Document{HTMLBlock[7]{HTMLText \"<source src='x' a>  \", VerbatimLineEnding \"\\n\", HTMLText \"</b >\", VerbatimLineEnding \"\\n\"}, BlankLine \"\\n\", HTMLBlock[7]{HTMLText \"<a b=c/>\"}}"},
		{"does not interrupt a paragraph with an html block of kind 7", "a\n<source>", "Document{Paragraph{Text \"a\", SoftBreak{LineEnding \"\\n\"}, RawHTML{HTMLText \"<source>\"}}}"},
		{"needs only spaces and tabs after the tag of an html block of kind 7", "<a> b\n\n<a b='>\n\n<pre/>", "Document{Paragraph{RawHTML{HTMLText \"<a>\"}, Text \" b\", LineEnding \"\\n\"}, BlankLine \"\\n\", Paragraph{Text \"<a b='>\", LineEnding \"\\n\"}, BlankLine \"\\n\", Paragraph{RawHTML{HTMLText \"<pre/>\"}}}"},
		{"gives a block quote", "> a\n > b", "Document{BlockQuote{QuoteMarker@1 \"> \", Paragraph{Text \"a\", SoftBreak{LineEnding \"\\n\"}, QuoteMarker@1 \" > \", Text \"b\"}}}"},
		{"gives an empty block quote", ">", "Document{BlockQuote{QuoteMarker@1 \">\"}}"},
		{"continues a paragraph in a block quote on a lazy line", "> a\nb", "Document{BlockQuote{QuoteMarker@1 \"> \", Paragraph{Text \"a\", SoftBreak{LineEnding \"\\n\"}, Text \"b\"}}}"},
		{"gives a lazy line the prefix leaves of the containers it matches", " > >a\n>b", "Document{BlockQuote{QuoteMarker@1 \" > \", BlockQuote{QuoteMarker@3 \">\", Paragraph{Text \"a\", SoftBreak{LineEnding \"\\n\"}, QuoteMarker@1 \">\", Text \"b\"}}}}"},
		{"puts a blank line in a block quote after its prefix leaf", "> a\n>\n> b", "Document{BlockQuote{QuoteMarker@1 \"> \", Paragraph{Text \"a\", LineEnding \"\\n\"}, QuoteMarker@1 \">\", BlankLine \"\\n\", QuoteMarker@1 \"> \", Paragraph{Text \"b\"}}}"},
		{"ends a block quote at a blank line", "> a\n\nb", "Document{BlockQuote{QuoteMarker@1 \"> \", Paragraph{Text \"a\", LineEnding \"\\n\"}}, BlankLine \"\\n\", Paragraph{Text \"b\"}}"},
		{"puts the prefix leaves of later code lines inside the code block", ">     a\n>     b", "Document{BlockQuote{QuoteMarker@1 \"> \", CodeBlock{CodeIndent \"    \", CodeText \"a\", VerbatimLineEnding \"\\n\", QuoteMarker@1 \"> \", CodeIndent \"    \", CodeText \"b\"}}}"},
		{"keeps the prefix leaves of pending blank code lines", ">     a\n>\n>     b\n>\nc", "Document{BlockQuote{QuoteMarker@1 \"> \", CodeBlock{CodeIndent \"    \", CodeText \"a\", VerbatimLineEnding \"\\n\", QuoteMarker@1 \">\", VerbatimLineEnding \"\\n\", QuoteMarker@1 \"> \", CodeIndent \"    \", CodeText \"b\", VerbatimLineEnding \"\\n\"}, QuoteMarker@1 \">\", BlankLine \"\\n\"}, Paragraph{Text \"c\"}}"},
		{"does not start indented code on a lazy line", "> a\n    b", "Document{BlockQuote{QuoteMarker@1 \"> \", Paragraph{Text \"a\", SoftBreak{LineEnding \"\\n\"}, Indent \"    \", Text \"b\"}}}"},
		{"does not start an html block of kind 7 on a lazy line", "> a\n<del>", "Document{BlockQuote{QuoteMarker@1 \"> \", Paragraph{Text \"a\", SoftBreak{LineEnding \"\\n\"}, RawHTML{HTMLText \"<del>\"}}}}"},
		{"closes a block quote at a thematic break", "> a\n---", "Document{BlockQuote{QuoteMarker@1 \"> \", Paragraph{Text \"a\", LineEnding \"\\n\"}}, ThematicBreak{ThematicRun \"---\"}}"},
		{"gives a setext heading in a block quote", "> a\n> ---\n> b\n===", "Document{BlockQuote{QuoteMarker@1 \"> \", Heading{Text \"a\", LineEnding \"\\n\", QuoteMarker@1 \"> \", SetextUnderline \"---\", LineEnding \"\\n\"}, QuoteMarker@1 \"> \", Paragraph{Text \"b\", SoftBreak{LineEnding \"\\n\"}, Text \"===\"}}}"},
		{"closes fenced code and html blocks with their block quote", "> ```\n> a\nb\n> <div>\nc", "Document{BlockQuote{QuoteMarker@1 \"> \", CodeBlock{FenceMarker \"```\", LineEnding \"\\n\", QuoteMarker@1 \"> \", CodeText \"a\", VerbatimLineEnding \"\\n\"}}, Paragraph{Text \"b\", LineEnding \"\\n\"}, BlockQuote{QuoteMarker@12 \"> \", HTMLBlock[6]{HTMLText \"<div>\", VerbatimLineEnding \"\\n\"}}, Paragraph{Text \"c\"}}"},
		{"gives a tab split by a block quote to the next leaf", ">\t\tfoo", "Document{BlockQuote{QuoteMarker@1 \">\", CodeBlock{CodeIndent+2 \"\\t\", CodeText+2 \"\\tfoo\"}}}"},
		{"lowers the columns left of a tab that fence indentation splits further", ">  ```\n>\t\tx\n>  ```", "Document{BlockQuote{QuoteMarker@1 \"> \", CodeBlock{Indent \" \", FenceMarker \"```\", LineEnding \"\\n\", QuoteMarker@1 \">\", CodeText+1 \"\\t\\tx\", VerbatimLineEnding \"\\n\", QuoteMarker@1 \"> \", Indent \" \", FenceMarker \"```\"}}}"},
		{"keeps a split tab alone on the last line of fenced code", "> ```\n>\t", "Document{BlockQuote{QuoteMarker@1 \"> \", CodeBlock{FenceMarker \"```\", LineEnding \"\\n\", QuoteMarker@1 \">\", CodeText+2 \"\\t\"}}}"},
		{"keeps a split tab alone on a line of an html block", "> <!--\n>\t\n> -->", "Document{BlockQuote{QuoteMarker@1 \"> \", HTMLBlock[2]{HTMLText \"<!--\", VerbatimLineEnding \"\\n\", QuoteMarker@1 \">\", HTMLText+2 \"\\t\", VerbatimLineEnding \"\\n\", QuoteMarker@1 \"> \", HTMLText \"-->\"}}}"},
		{"gives a split tab before a paragraph to its indent leaf", ">\tfoo", "Document{BlockQuote{QuoteMarker@1 \">\", Paragraph{Indent+2 \"\\t\", Text \"foo\"}}}"},
		{"gives a split tab before a nested block quote marker to that marker", ">\t>\t\tfoo", "Document{BlockQuote{QuoteMarker@1 \">\", BlockQuote{QuoteMarker@3+2 \"\\t>\", CodeBlock{CodeIndent+2 \"\\t\", CodeText+2 \"\\tfoo\"}}}}"},
		{"splits a tab to remove the indentation of fenced code", "  ```\n\tx\n```", "Document{CodeBlock{Indent \"  \", FenceMarker \"```\", LineEnding \"\\n\", CodeText+2 \"\\tx\", VerbatimLineEnding \"\\n\", FenceMarker \"```\"}}"},
		{"gives a list of items", "- a\n- b", "Document{List{ListItem{ListMarker@2 \"- \", Paragraph{Text \"a\", LineEnding \"\\n\"}}, ListItem{ListMarker@7 \"- \", Paragraph{Text \"b\"}}}}"},
		{"gives a later item line its indent leaf", " - a\n   - b", "Document{List{ListItem{ListMarker@2 \" - \", Paragraph{Text \"a\", LineEnding \"\\n\"}, ItemIndent@2 \"   \", List{ListItem{ListMarker@9 \"- \", Paragraph{Text \"b\"}}}}}}"},
		{"continues a list item on a lazy line", "- a\nb", "Document{List{ListItem{ListMarker@2 \"- \", Paragraph{Text \"a\", SoftBreak{LineEnding \"\\n\"}, Text \"b\"}}}}"},
		{"gives indented code after five spaces of padding", "-     code", "Document{List{ListItem{ListMarker@2 \"- \", CodeBlock{CodeIndent \"    \", CodeText \"code\"}}}}"},
		{"splits a tab after a list marker", "-\t\tfoo", "Document{List{ListItem{ListMarker@2 \"-\", CodeBlock{CodeIndent+2 \"\\t\", CodeText+2 \"\\tfoo\"}}}}"},
		{"ends an empty list item at a blank line and keeps the blank line in the list", "-\n\n  a", "Document{List{ListItem{ListMarker@2 \"-\", BlankLine \"\\n\"}, BlankLine \"\\n\"}, Paragraph{Indent \"  \", Text \"a\"}}"},
		{"consumes the spaces of a blank line in a list item", "- a\n \n   \n  b", "Document{List[1]{ListItem{ListMarker@2 \"- \", Paragraph{Text \"a\", LineEnding \"\\n\"}, ItemIndent@2 \" \", BlankLine \"\\n\", ItemIndent@2 \"  \", BlankLine \" \\n\", ItemIndent@2 \"  \", Paragraph{Text \"b\"}}}}"},
		{"starts a new list at another bullet character or delimiter", "- a\n* b\n1. c\n2) d", "Document{List{ListItem{ListMarker@2 \"- \", Paragraph{Text \"a\", LineEnding \"\\n\"}}}, List{ListItem{ListMarker@8 \"* \", Paragraph{Text \"b\", LineEnding \"\\n\"}}}, List{ListItem{ListMarker@14 \"1. \", Paragraph{Text \"c\", LineEnding \"\\n\"}}}, List{ListItem{ListMarker@20 \"2) \", Paragraph{Text \"d\"}}}}"},
		{"interrupts a paragraph only with a list item that starts at 1 and has content", "a\n2. b\n*\n1. c", "Document{Paragraph{Text \"a\", SoftBreak{LineEnding \"\\n\"}, Text \"2. b\", SoftBreak{LineEnding \"\\n\"}, Text \"*\", LineEnding \"\\n\"}, List{ListItem{ListMarker@11 \"1. \", Paragraph{Text \"c\"}}}}"},
		{"needs at most nine digits in a list marker", "1234567890. a", "Document{Paragraph{Text \"1234567890. a\"}}"},
		{"prefers a thematic break to a list item", "- a\n- - -", "Document{List{ListItem{ListMarker@2 \"- \", Paragraph{Text \"a\", LineEnding \"\\n\"}}}, ThematicBreak{ThematicRun \"- - -\"}}"},
		{"closes a list at a block that is not an item", "- a\n\nb", "Document{List{ListItem{ListMarker@2 \"- \", Paragraph{Text \"a\", LineEnding \"\\n\"}, BlankLine \"\\n\"}}, Paragraph{Text \"b\"}}"},
		{"makes a list loose at a blank line between items", "- a\n\n- b", "Document{List[1]{ListItem{ListMarker@2 \"- \", Paragraph{Text \"a\", LineEnding \"\\n\"}, BlankLine \"\\n\"}, ListItem{ListMarker@8 \"- \", Paragraph{Text \"b\"}}}}"},
		{"keeps a list tight at a blank line after its last item", "- a\n- b\n\n", "Document{List{ListItem{ListMarker@2 \"- \", Paragraph{Text \"a\", LineEnding \"\\n\"}}, ListItem{ListMarker@7 \"- \", Paragraph{Text \"b\", LineEnding \"\\n\"}, BlankLine \"\\n\"}}}"},
		{"keeps a list tight at a blank line in fenced code", "- ```\n\n  ```\n- b", "Document{List{ListItem{ListMarker@2 \"- \", CodeBlock{FenceMarker \"```\", LineEnding \"\\n\", VerbatimLineEnding \"\\n\", ItemIndent@2 \"  \", FenceMarker \"```\", LineEnding \"\\n\"}}, ListItem{ListMarker@11 \"- \", Paragraph{Text \"b\"}}}}"},
		{"makes only the list whose item has a blank line between children loose", "- a\n  - b\n\n    c\n- d", "Document{List{ListItem{ListMarker@2 \"- \", Paragraph{Text \"a\", LineEnding \"\\n\"}, ItemIndent@2 \"  \", List[1]{ListItem{ListMarker@9 \"- \", Paragraph{Text \"b\", LineEnding \"\\n\"}, BlankLine \"\\n\", ItemIndent@2 \"  \", ItemIndent@9 \"  \", Paragraph{Text \"c\", LineEnding \"\\n\"}}}}, ListItem{ListMarker@20 \"- \", Paragraph{Text \"d\"}}}}"},
		{"gives a link reference definition", "[foo]: /url \"title\"\n\n[foo]", "Document{LinkReferenceDefinition{Bracket \"[\", LinkLabel \"foo\", Bracket \"]\", Colon \":\", Whitespace \" \", Destination \"/url\", Whitespace \" \", TitleQuote \"\\\"\", Title \"title\", TitleQuote \"\\\"\", LineEnding \"\\n\"}, BlankLine \"\\n\", Paragraph{Text \"[foo]\"}}"},
		{"gives a link reference definition over several lines", "   [foo]: \n      /url  \n           'the title'  \n", "Document{LinkReferenceDefinition{Indent \"   \", Bracket \"[\", LinkLabel \"foo\", Bracket \"]\", Colon \":\", Whitespace \" \", LineEnding \"\\n\", Indent \"      \", Destination \"/url\", Whitespace \"  \", LineEnding \"\\n\", Indent \"           \", TitleQuote \"'\", Title \"the title\", TitleQuote \"'\", Whitespace \"  \", LineEnding \"\\n\"}}"},
		{"gives a label over several lines and an angle destination", "[\nfoo\n]: <my url>\nbar", "Document{LinkReferenceDefinition{Bracket \"[\", VerbatimLineEnding \"\\n\", LinkLabel \"foo\", VerbatimLineEnding \"\\n\", Bracket \"]\", Colon \":\", Whitespace \" \", AngleBracket \"<\", Destination \"my url\", AngleBracket \">\", LineEnding \"\\n\"}, Paragraph{Text \"bar\"}}"},
		{"gives a title over several lines", "[foo]: /url '\ntitle\n'", "Document{LinkReferenceDefinition{Bracket \"[\", LinkLabel \"foo\", Bracket \"]\", Colon \":\", Whitespace \" \", Destination \"/url\", Whitespace \" \", TitleQuote \"'\", VerbatimLineEnding \"\\n\", Title \"title\", VerbatimLineEnding \"\\n\", TitleQuote \"'\"}}"},
		{"gives an empty angle destination", "[foo]: <>", "Document{LinkReferenceDefinition{Bracket \"[\", LinkLabel \"foo\", Bracket \"]\", Colon \":\", Whitespace \" \", AngleBracket \"<\", AngleBracket \">\"}}"},
		{"rewinds a failed title to the end of the destination", "[foo]: /url\n\"title\" ok", "Document{LinkReferenceDefinition{Bracket \"[\", LinkLabel \"foo\", Bracket \"]\", Colon \":\", Whitespace \" \", Destination \"/url\", LineEnding \"\\n\"}, Paragraph{Text \"\\\"title\\\" ok\"}}"},
		{"needs only spaces and tabs after a definition on its line", "[foo]: /url \"title\" ok\n\n[foo]: <bar>(baz)", "Document{Paragraph{Text \"[foo]: /url \\\"title\\\" ok\", LineEnding \"\\n\"}, BlankLine \"\\n\", Paragraph{Text \"[foo]: \", RawHTML{HTMLText \"<bar>\"}, Text \"(baz)\"}}"},
		{"needs a label that is not blank and has no unescaped bracket", "[ ]: /u\n\n[a[b]: /u\n\n[a\\]b]: /u", "Document{Paragraph{Text \"[ ]: /u\", LineEnding \"\\n\"}, BlankLine \"\\n\", Paragraph{Text \"[a[b]: /u\", LineEnding \"\\n\"}, BlankLine \"\\n\", LinkReferenceDefinition{Bracket \"[\", LinkLabel \"a\\\\]b\", Bracket \"]\", Colon \":\", Whitespace \" \", Destination \"/u\"}}"},
		{"needs balanced parentheses in a destination", "[a]: (b)c\n[a]: (b", "Document{LinkReferenceDefinition{Bracket \"[\", LinkLabel \"a\", Bracket \"]\", Colon \":\", Whitespace \" \", Destination \"(b)c\", LineEnding \"\\n\"}, Paragraph{Text \"[a]: (b\"}}"},
		{"needs a destination", "[a]:\n\n[a]:", "Document{Paragraph{Text \"[a]:\", LineEnding \"\\n\"}, BlankLine \"\\n\", Paragraph{Text \"[a]:\"}}"},
		{"gives several definitions before a paragraph", "[a]: /a\n[b]: /b\nc", "Document{LinkReferenceDefinition{Bracket \"[\", LinkLabel \"a\", Bracket \"]\", Colon \":\", Whitespace \" \", Destination \"/a\", LineEnding \"\\n\"}, LinkReferenceDefinition{Bracket \"[\", LinkLabel \"b\", Bracket \"]\", Colon \":\", Whitespace \" \", Destination \"/b\", LineEnding \"\\n\"}, Paragraph{Text \"c\"}}"},
		{"gives a definition in a block quote the prefix leaves of its lines", "> [foo]:\n> /url\n> bar", "Document{BlockQuote{QuoteMarker@1 \"> \", LinkReferenceDefinition{Bracket \"[\", LinkLabel \"foo\", Bracket \"]\", Colon \":\", LineEnding \"\\n\", QuoteMarker@1 \"> \", Destination \"/url\", LineEnding \"\\n\"}, QuoteMarker@1 \"> \", Paragraph{Text \"bar\"}}}"},
		{"parses definitions before a setext heading", "[foo]: /url\nbar\n===", "Document{LinkReferenceDefinition{Bracket \"[\", LinkLabel \"foo\", Bracket \"]\", Colon \":\", Whitespace \" \", Destination \"/url\", LineEnding \"\\n\"}, Heading{Text \"bar\", LineEnding \"\\n\", SetextUnderline \"===\"}}"},
		{"dispatches an underline after definitions alone as if a paragraph were open", "[foo]: /url\n===\n[foo]\n\n[a]: /a\n-\n\n[b]: /b\n---", "Document{LinkReferenceDefinition{Bracket \"[\", LinkLabel \"foo\", Bracket \"]\", Colon \":\", Whitespace \" \", Destination \"/url\", LineEnding \"\\n\"}, Paragraph{Text \"===\", SoftBreak{LineEnding \"\\n\"}, Text \"[foo]\", LineEnding \"\\n\"}, BlankLine \"\\n\", LinkReferenceDefinition{Bracket \"[\", LinkLabel \"a\", Bracket \"]\", Colon \":\", Whitespace \" \", Destination \"/a\", LineEnding \"\\n\"}, Paragraph{Text \"-\", LineEnding \"\\n\"}, BlankLine \"\\n\", LinkReferenceDefinition{Bracket \"[\", LinkLabel \"b\", Bracket \"]\", Colon \":\", Whitespace \" \", Destination \"/b\", LineEnding \"\\n\"}, ThematicBreak{ThematicRun \"---\"}}"},
		{"takes a label of 999 characters", "[" + strings.Repeat("é", 999) + "]: /u", "Document{LinkReferenceDefinition{Bracket \"[\", LinkLabel \"" + strings.Repeat("é", 999) + "\", Bracket \"]\", Colon \":\", Whitespace \" \", Destination \"/u\"}}"},
		{"rejects a label of 1000 characters", "[" + strings.Repeat("a", 1000) + "]: /u", "Document{Paragraph{Text \"[" + strings.Repeat("a", 1000) + "]: /u\"}}"},
		{"gives front matter", "---\na: 1\n\n  \n---  \nb", "Document{FrontMatter{FrontMatterFence \"---\", LineEnding \"\\n\", FrontMatterText \"a: 1\", VerbatimLineEnding \"\\n\", VerbatimLineEnding \"\\n\", FrontMatterText \"  \", VerbatimLineEnding \"\\n\", FrontMatterFence \"---\", Whitespace \"  \", LineEnding \"\\n\"}, Paragraph{Text \"b\"}}"},
		{"gives empty toml front matter after a bom", "\xEF\xBB\xBF+++\t\r\n+++", "Document{BOM \"\\ufeff\", FrontMatter{FrontMatterFence \"+++\", Whitespace \"\\t\", LineEnding \"\\r\\n\", FrontMatterFence \"+++\"}}"},
		{"needs a closing front matter fence of the same delimiter", "---\n+++\na", "Document{ThematicBreak{ThematicRun \"---\", LineEnding \"\\n\"}, Paragraph{Text \"+++\", SoftBreak{LineEnding \"\\n\"}, Text \"a\"}}"},
		{"needs front matter at the start of the input", "\n---\n---", "Document{BlankLine \"\\n\", ThematicBreak{ThematicRun \"---\", LineEnding \"\\n\"}, ThematicBreak{ThematicRun \"---\"}}"},
		{"needs an unindented front matter fence", " ---\n---", "Document{ThematicBreak{Indent \" \", ThematicRun \"---\", LineEnding \"\\n\"}, ThematicBreak{ThematicRun \"---\"}}"},
		{"needs a front matter fence of exactly three characters", "----\n----", "Document{ThematicBreak{ThematicRun \"----\", LineEnding \"\\n\"}, ThematicBreak{ThematicRun \"----\"}}"},
		{"needs a thematic break indented less than four columns", "a\n  \t___", `Document{Paragraph{Text "a", SoftBreak{LineEnding "\n"}, Indent "  \t", Text "___"}}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tree := Parse([]byte(tt.src))
			if err := tree.Verify(); err != nil {
				t.Fatalf("Parse(%q).Verify() = %v", tt.src, err)
			}
			if got := dump(tree); got != tt.want {
				t.Fatalf("Parse(%q)\n got %s\nwant %s", tt.src, got, tt.want)
			}
		})
	}

	t.Run("pathological", func(t *testing.T) {
		for _, in := range pathologicalInputs {
			t.Run(in.name, func(t *testing.T) {
				// The 10n run takes at least 50 ms, so the n run is long enough to
				// time.
				n := 1000
				large, _ := timeParse(t, in.build(10*n))
				for large < 50*time.Millisecond && 10*n < inputLimit {
					n *= 2
					large, _ = timeParse(t, in.build(10*n))
				}
				small, _ := timeParse(t, in.build(n))
				if ratio := float64(large) / float64(small); ratio > 30 {
					t.Errorf("%d bytes take %v and %d bytes take %v: ratio %.0f, want at most 30", n, small, 10*n, large, ratio)
				}
			})
		}
	})

	t.Run("long", func(t *testing.T) {
		if spec := os.Getenv("MARKFMT_LONG_CHILD"); spec != "" {
			longChild(t, spec)
			return
		}
		if raceEnabled {
			t.Skip("the race detector makes times and memory unlike those of a normal build")
		}
		if os.Getenv("MARKFMT_LONG") != "1" {
			t.Skip("set MARKFMT_LONG=1 to run")
		}
		empty := runLongChild(t, "empty", 0)
		prose := runLongChild(t, "prose", inputLimit)
		quotes := runLongChild(t, "quote lines", inputLimit)
		perByte := float64(prose.time) / float64(prose.bytes)
		perNode := float64(quotes.time) / float64(quotes.nodes)
		for _, in := range pathologicalInputs {
			t.Run(in.name, func(t *testing.T) {
				small := runLongChild(t, in.name, inputLimit/10)
				large := runLongChild(t, in.name, inputLimit)
				t.Logf("%d bytes: %v; %d bytes and %d nodes: %v and %d bytes of memory",
					small.bytes, small.time, large.bytes, large.nodes, large.time, large.maxrss-empty.maxrss)
				if ratio := float64(large.time) / float64(small.time); ratio > 30 {
					t.Errorf("%d bytes take %v and %d bytes take %v: ratio %.0f, want at most 30", small.bytes, small.time, large.bytes, large.time, ratio)
				}
				// A linear path with a large constant fails this bound, whatever
				// its node count.
				if bound := time.Duration(20 * (float64(large.bytes)*perByte + float64(large.nodes)*perNode)); large.time > bound {
					t.Errorf("%d bytes and %d nodes take %v, want at most %v", large.bytes, large.nodes, large.time, bound)
				}
				if large.maxrss > 0 && empty.maxrss > 0 && large.maxrss-empty.maxrss > memoryBound {
					t.Errorf("%d bytes take %d bytes of memory, want at most %d", large.bytes, large.maxrss-empty.maxrss, int64(memoryBound))
				}
			})
		}
	})

	for _, c := range corpora {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			testConformance(t, c)
		})
	}
}

// inputLimit is the input limit of markfmt.Format (design 7.2).
const inputLimit = 8 << 20

// memoryBound is the peak memory of the long test at stage 2: half the 4 GiB
// budget of markfmt.Format, which parses two trees (design 7.2, 11.1).
const memoryBound = 2 << 30

// longInputs are the calibration inputs of the long test, by name: prose for
// the time per byte, and block quote lines for the time per node.
var longInputs = map[string]func(n int) []byte{
	"empty":       func(int) []byte { return nil },
	"prose":       func(n int) []byte { return bytes.Repeat([]byte("Lorem ipsum dolor sit amet.\n"), n/28) },
	"quote lines": func(n int) []byte { return bytes.Repeat([]byte(">a\n"), n/3) },
}

type longResult struct {
	time         time.Duration
	nodes, bytes int
	maxrss       int64 // peak resident set size in bytes, or 0
}

// runLongChild runs the input called name at size bytes in a child process of
// the test binary (design 11.1).
func runLongChild(t *testing.T, name string, size int) longResult {
	t.Helper()

	cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestParse$/^long$") //nolint:gosec // The command is the test binary itself.
	cmd.Env = append(os.Environ(), "MARKFMT_LONG_CHILD="+name+","+strconv.Itoa(size))
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("child for %s at %d bytes: %v\n%s", name, size, err, out)
	}
	var r longResult
	for line := range strings.Lines(string(out)) {
		if _, err := fmt.Sscanf(line, "markfmt-long %d %d %d", &r.time, &r.nodes, &r.bytes); err == nil {
			r.maxrss, _ = maxrssBytes(cmd.ProcessState)
			return r
		}
	}
	t.Fatalf("child for %s at %d bytes gave no result:\n%s", name, size, out)
	return r
}

// longChild builds the input that spec names, "name,size", and prints the
// best time of Parse, Verify and Equal on it, its node count and its size.
func longChild(t *testing.T, spec string) {
	name, size, _ := strings.Cut(spec, ",")
	n, err := strconv.Atoi(size)
	if err != nil {
		t.Fatal(err)
	}
	build := longInputs[name]
	for _, in := range pathologicalInputs {
		if in.name == name {
			build = in.build
		}
	}
	if build == nil {
		t.Fatalf("no input %q", name)
	}
	src := build(n)
	d, nodes := timeParse(t, src)
	fmt.Printf("markfmt-long %d %d %d\n", d, nodes, len(src))
}

// pathologicalInputs are the inputs of design 6.8. Each builds an input
// of about n bytes.
var pathologicalInputs = []struct {
	name  string
	build func(n int) []byte
}{
	{"list markers on one line", func(n int) []byte {
		return []byte(strings.Repeat("- ", n/2) + "a")
	}},
	{"nested list items then blank lines", func(n int) []byte {
		return []byte(strings.Repeat("- ", n/4) + "a" + strings.Repeat("\n", n/2))
	}},
	{"nested block quotes on one line", func(n int) []byte {
		return []byte(strings.Repeat(">", n) + "a")
	}},
	{"definitions with unclosed titles", func(n int) []byte {
		return []byte(strings.Repeat("[a]: b 'c\n", n/10))
	}},
	{"unclosed html comments", func(n int) []byte {
		return []byte("a " + strings.Repeat("<!--", n/4))
	}},
	{"unclosed processing instructions", func(n int) []byte {
		return []byte("a " + strings.Repeat("<?", n/2))
	}},
	{"unclosed cdata sections", func(n int) []byte {
		return []byte("a " + strings.Repeat("<![CDATA[", n/9))
	}},
	{"unclosed declarations", func(n int) []byte {
		return []byte("a " + strings.Repeat("<!X", n/3))
	}},
	{"tags with an attribute on each line", func(n int) []byte {
		return []byte("a " + strings.Repeat("<a x=\"1\"\n", n/9))
	}},
	{"emphasis openers without closers", func(n int) []byte {
		return []byte(strings.Repeat("_a ", n/3))
	}},
	{"emphasis closers without openers", func(n int) []byte {
		return []byte(strings.Repeat("a_ ", n/3))
	}},
	{"mismatched emphasis characters", func(n int) []byte {
		return []byte(strings.Repeat("*a_ ", n/4))
	}},
	{"emphasis runs whose lengths sum to multiples of 3", func(n int) []byte {
		return []byte("a**b" + strings.Repeat("c* ", n/3))
	}},
	{"strong emphasis in emphasis", func(n int) []byte {
		return []byte(strings.Repeat("***a*** ", n/8))
	}},
	{"nested strong emphasis", func(n int) []byte {
		return []byte(strings.Repeat("*a **a ", n/14) + "b" + strings.Repeat(" a** a*", n/14))
	}},
	{"unmatched link openers", func(n int) []byte {
		return []byte(strings.Repeat("[", n))
	}},
	{"unmatched link closers", func(n int) []byte {
		return []byte(strings.Repeat("]", n))
	}},
	{"images with empty links in them", func(n int) []byte {
		return []byte(strings.Repeat("![[]()", n/6))
	}},
	{"angle destinations that do not close", func(n int) []byte {
		return []byte(strings.Repeat("[a](<b", n/6))
	}},
	{"destinations that do not close", func(n int) []byte {
		return []byte(strings.Repeat("[a](b", n/5))
	}},
	{"parenthesized titles that do not close", func(n int) []byte {
		return []byte(strings.Repeat("[ (](", n/5))
	}},
	{"backtick runs of every length", func(n int) []byte {
		var b []byte
		for i := 1; len(b) < n; i++ {
			b = append(append(b, 'e'), strings.Repeat("`", i)...)
		}
		return b
	}},
}

// timeParse returns the best of 3 times of Parse, Verify, and Equal of the
// tree with itself on src, and the number of nodes.
func timeParse(t testing.TB, src []byte) (time.Duration, int) {
	t.Helper()

	best, nodes := time.Duration(1<<63-1), 0
	for range 3 {
		start := time.Now()
		tree := Parse(src)
		if err := tree.Verify(); err != nil {
			t.Fatal(err)
		}
		if err := Equal(tree, tree); err != nil {
			t.Fatal(err)
		}
		best, nodes = min(best, time.Since(start)), len(tree.nodes)
	}
	return best, nodes
}

// testConformance renders the examples of c's sections and compares them with
// the expected HTML, against the examples listed in failing.txt and
// grammar-differs.txt next to c's file. The list checks cover the whole
// corpus, whatever subtests -run selects.
func testConformance(t *testing.T, c corpus) {
	examples := slices.DeleteFunc(readExamples(t, c.path), func(ex example) bool {
		return !strings.HasSuffix(ex.section, c.sections)
	})
	failing := readFailing(t, filepath.Join(filepath.Dir(c.path), "failing.txt"), examples)
	differs := readGrammarDiffers(t, filepath.Join(filepath.Dir(c.path), "grammar-differs.txt"), examples, failing)

	got := make([]string, len(examples))
	want := make([]string, len(examples))
	pass := make([]bool, len(examples))
	var unlisted, passing, same []int
	for i, ex := range examples {
		tree := Parse([]byte(ex.markdown))
		if err := tree.Verify(); err != nil {
			t.Errorf("%s example %d: %v", c.name, ex.id, err)
		}
		got[i] = normalizeHTML(renderHTML(tree))
		want[i] = normalizeHTML(ex.html)
		// cmark-gfm counts an example whose expected HTML is <IGNORE> as passing:
		// it tests only that parsing does not crash.
		pass[i] = got[i] == want[i] || strings.TrimSpace(ex.html) == "<IGNORE>"
		switch {
		case differs[ex.id]:
			if pass[i] {
				same = append(same, ex.id)
			}
		case !pass[i] && !failing[ex.id]:
			unlisted = append(unlisted, ex.id)
		case pass[i] && failing[ex.id]:
			passing = append(passing, ex.id)
		}
	}
	if len(unlisted) > 0 {
		t.Errorf("%d examples fail and are not in failing.txt: %v", len(unlisted), unlisted)
	}
	if len(passing) > 0 {
		t.Errorf("%d examples in failing.txt pass: %v", len(passing), passing)
	}
	if len(same) > 0 {
		t.Errorf("%d examples in grammar-differs.txt no longer differ: %v", len(same), same)
	}

	for i, ex := range examples {
		t.Run(c.name+" example "+strconv.Itoa(ex.id), func(t *testing.T) {
			t.Parallel()

			switch {
			case differs[ex.id]:
				if pass[i] {
					t.Error("no longer differs: remove it from grammar-differs.txt")
				}
			case !pass[i] && !failing[ex.id]:
				t.Errorf("fails and is not in failing.txt (section %s)\nmarkdown: %q\n     got: %q\n    want: %q",
					ex.section, ex.markdown, got[i], want[i])
			case pass[i] && failing[ex.id]:
				t.Error("passes: remove it from failing.txt")
			}
		})
	}
}

// readFailing reads the IDs in a failing.txt file: one example ID per line,
// in increasing order, with "#" comments. It reports an entry that is not an
// ID, that is a duplicate or out of order, or that names no example.
func readFailing(t *testing.T, path string, examples []example) map[int]bool {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	ids := make(map[int]bool, len(examples))
	for _, ex := range examples {
		ids[ex.id] = true
	}
	failing := make(map[int]bool)
	last, n := 0, 0
	for line := range strings.Lines(string(data)) {
		n++
		entry, _, _ := strings.Cut(line, "#")
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		id, err := strconv.Atoi(entry)
		switch {
		case err != nil:
			t.Errorf("%s:%d: %q is not an example ID", path, n, entry)
			continue
		case id <= last:
			t.Errorf("%s:%d: %d is a duplicate or out of order", path, n, id)
		case !ids[id]:
			t.Errorf("%s:%d: %d names no example", path, n, id)
		}
		failing[id] = true
		last = max(last, id)
	}
	return failing
}

// readGrammarDiffers reads the entries of a grammar-differs.txt file, if it
// exists: one per line, in increasing order, with "#" comments. An entry is
// an example ID, the rule, a colon, and the section of the case in
// testdata/markfmt/grammar.txt that has the same input. It reports an entry
// that is not one, that names no example, that is also in failing.txt, or
// whose case is missing or has another input.
func readGrammarDiffers(t *testing.T, path string, examples []example, failing map[int]bool) map[int]bool {
	t.Helper()

	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	inputs := make(map[int]string, len(examples))
	for _, ex := range examples {
		inputs[ex.id] = ex.markdown
	}
	cases := make(map[string]string)
	for _, ex := range readExamples(t, "testdata/markfmt/grammar.txt") {
		cases[ex.section] = ex.markdown
	}
	differs := make(map[int]bool)
	last, n := 0, 0
	for line := range strings.Lines(string(data)) {
		n++
		entry, _, _ := strings.Cut(line, "#")
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		idText, rule, _ := strings.Cut(entry, " ")
		_, section, found := strings.Cut(rule, ":")
		section = strings.TrimSpace(section)
		id, err := strconv.Atoi(idText)
		input, isExample := inputs[id]
		caseInput, hasCase := cases[section]
		switch {
		case err != nil || !found:
			t.Errorf("%s:%d: %q is not an entry", path, n, entry)
			continue
		case id <= last:
			t.Errorf("%s:%d: %d is a duplicate or out of order", path, n, id)
		case !isExample:
			t.Errorf("%s:%d: %d names no example", path, n, id)
		case failing[id]:
			t.Errorf("%s:%d: %d is also in failing.txt", path, n, id)
		case !hasCase:
			t.Errorf("%s:%d: %q names no case in testdata/markfmt/grammar.txt", path, n, section)
		case caseInput != input:
			t.Errorf("%s:%d: case %q has another input than example %d", path, n, section, id)
		}
		differs[id] = true
		last = max(last, id)
	}
	return differs
}

func TestNeedsInlines(t *testing.T) {
	t.Parallel()

	t.Run("leaves 250 of 296 block-section commonmark examples to stage 2", func(t *testing.T) {
		t.Parallel()

		var block, blockOnly int
		for _, ex := range readExamples(t, "testdata/commonmark/spec.txt") {
			if slices.Contains(blockSections, ex.section) {
				block++
				if !needsInlines(ex) {
					blockOnly++
				}
			}
		}
		if block != 296 || blockOnly != 250 {
			t.Fatalf("%d of %d block-section examples do not need inlines, want 250 of 296", blockOnly, block)
		}
	})
}

// blockSections are the CommonMark spec sections about block structure.
var blockSections = []string{
	"Tabs", "Precedence", "Thematic breaks", "ATX headings", "Setext headings",
	"Indented code blocks", "Fenced code blocks", "HTML blocks",
	"Link reference definitions", "Paragraphs", "Blank lines", "Block quotes",
	"List items", "Lists",
}

// needsInlines reports whether a CommonMark example needs the inline phase to
// pass (design 11.2): its expected HTML, outside every <pre> element, has an
// inline element or a character reference other than &quot;, &amp;, &lt; and
// &gt;, or its Markdown has "\" or "&". Stage 2 passes every block-section
// example that does not. Delete it in the commit that passes the stage 3 gate.
func needsInlines(ex example) bool {
	// CM 148 has <em> inside the raw <pre> of an HTML block, which the <pre>
	// rule skips. CM 201 has raw inline HTML, which the element list misses.
	if strings.ContainsAny(ex.markdown, `\&`) || ex.id == 148 || ex.id == 201 {
		return true
	}
	for html := ex.html; ; {
		outside, rest, found := strings.Cut(html, "<pre")
		if hasInlineHTML(outside) {
			return true
		}
		if !found {
			return false
		}
		if _, html, found = strings.Cut(rest, "</pre>"); !found {
			return false
		}
	}
}

// hasInlineHTML reports whether html has an element that the inline phase
// writes, or a character reference other than &quot;, &amp;, &lt; and &gt;.
func hasInlineHTML(html string) bool {
	for _, tag := range []string{"<em", "<strong", "<a", "<img", "<code", "<br"} {
		for rest, found := html, true; found; {
			_, rest, found = strings.Cut(rest, tag)
			if found && rest != "" && strings.IndexByte(" \t\n\r\f\v/>", rest[0]) >= 0 {
				return true
			}
		}
	}
	for rest, found := html, true; found; {
		_, rest, found = strings.Cut(rest, "&")
		name, _, semicolon := strings.Cut(rest, ";")
		if !found || !semicolon || slices.Contains([]string{"quot", "amp", "lt", "gt"}, name) {
			continue
		}
		digits := strings.TrimPrefix(name, "#")
		if digits != "" && strings.TrimFunc(digits, func(r rune) bool {
			return 'a' <= r|0x20 && r|0x20 <= 'z' || '0' <= r && r <= '9'
		}) == "" {
			return true
		}
	}
	return false
}

func FuzzParse(f *testing.F) {
	for _, src := range []string{"", "a", "a\nb\r\nc\rd\r\r\n"} {
		f.Add([]byte(src))
	}

	f.Fuzz(func(t *testing.T, src []byte) {
		tree := Parse(src)
		if err := tree.Verify(); err != nil {
			t.Fatalf("Parse(%q).Verify() = %v", src, err)
		}
		var leaves []byte
		for _, n := range tree.nodes {
			if n.kind.class() != classStructure {
				leaves = append(leaves, src[n.start:n.end]...)
			}
		}
		if !bytes.Equal(leaves, src) {
			t.Fatalf("leaves of Parse(%q) = %q", src, leaves)
		}
	})
}

// firstOf returns the first node of kind k in tree.
func firstOf(tree *Tree, k Kind) NodeID {
	for i, n := range tree.nodes {
		if n.kind == k {
			return NodeID(i)
		}
	}
	panic("no node of kind " + k.String())
}

// dump returns tree as nested kinds with their flags, and each leaf with the
// owner of a prefix leaf, its virt and its bytes, as in
// Document{BlockQuote{QuoteMarker@1 ">", HTMLBlock[6]{HTMLText+2 "\t<p>"}}}.
func dump(tree *Tree) string {
	var b strings.Builder
	sep := ""
	c := tree.Walk()
	for e, ok := c.Next(); ok; e, ok = c.Next() {
		k := tree.Kind(e.ID)
		switch {
		case e.Exit:
			b.WriteString("}")
			sep = ", "
		case k.class() == classStructure:
			b.WriteString(sep + k.String())
			if f := tree.nodes[e.ID].flags; f != 0 {
				b.WriteString("[" + strconv.Itoa(int(f)) + "]")
			}
			b.WriteString("{")
			sep = ""
		default:
			b.WriteString(sep + k.String())
			if link := tree.nodes[e.ID].link; link != 0 {
				b.WriteString("@" + strconv.Itoa(int(link)))
			}
			if virt := tree.nodes[e.ID].virt; virt != 0 {
				b.WriteString("+" + strconv.Itoa(int(virt)))
			}
			b.WriteString(" " + strconv.Quote(string(tree.Raw(e.ID))))
			sep = ", "
		}
	}
	return b.String()
}
