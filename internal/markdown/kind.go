package markdown

import "strconv"

// Kind is the kind of a [Node]. Appendix A of docs/design/parser.md lists
// every kind.
type Kind uint8

const (
	Document Kind = iota
	BlockQuote
	Paragraph
	ThematicBreak
	Heading
	CodeBlock
	HTMLBlock
	Text
	CodeText
	VerbatimLineEnding
	InfoString
	HTMLText
	BOM
	LineEnding
	BlankLine
	Indent
	ThematicRun
	ATXMarker
	ATXClose
	Whitespace
	CodeIndent
	FenceMarker
	SetextUnderline
	QuoteMarker
)

type class uint8

const (
	classInvalid class = iota
	classStructure
	classContent
	classSyntax
)

// class returns the comparison class of k. Structure kinds are interior;
// content and syntax kinds are leaves.
func (k Kind) class() class {
	//exhaustive:enforce
	switch k {
	case Document, BlockQuote, Paragraph, ThematicBreak, Heading, CodeBlock, HTMLBlock:
		return classStructure
	case Text, CodeText, VerbatimLineEnding, InfoString, HTMLText:
		return classContent
	case BOM, LineEnding, BlankLine, Indent, ThematicRun, ATXMarker, ATXClose, Whitespace, CodeIndent, FenceMarker, SetextUnderline, QuoteMarker:
		return classSyntax
	}
	return classInvalid
}

// owner returns the kind of the container that owns a prefix leaf of kind k,
// and whether k is a prefix kind.
func (k Kind) owner() (Kind, bool) {
	if k == QuoteMarker {
		return BlockQuote, true
	}
	return 0, false
}

// validFlags reports whether f is a flags value in range for kind k.
func (k Kind) validFlags(f uint8) bool {
	if k == HTMLBlock {
		return 1 <= f && f <= 7
	}
	return f == 0
}

func (k Kind) String() string {
	//exhaustive:enforce
	switch k {
	case Document:
		return "Document"
	case Paragraph:
		return "Paragraph"
	case Text:
		return "Text"
	case BOM:
		return "BOM"
	case LineEnding:
		return "LineEnding"
	case BlankLine:
		return "BlankLine"
	case Indent:
		return "Indent"
	case ThematicBreak:
		return "ThematicBreak"
	case ThematicRun:
		return "ThematicRun"
	case Heading:
		return "Heading"
	case ATXMarker:
		return "ATXMarker"
	case ATXClose:
		return "ATXClose"
	case Whitespace:
		return "Whitespace"
	case CodeBlock:
		return "CodeBlock"
	case CodeText:
		return "CodeText"
	case VerbatimLineEnding:
		return "VerbatimLineEnding"
	case CodeIndent:
		return "CodeIndent"
	case InfoString:
		return "InfoString"
	case FenceMarker:
		return "FenceMarker"
	case SetextUnderline:
		return "SetextUnderline"
	case HTMLBlock:
		return "HTMLBlock"
	case HTMLText:
		return "HTMLText"
	case BlockQuote:
		return "BlockQuote"
	case QuoteMarker:
		return "QuoteMarker"
	}
	return "Kind(" + strconv.Itoa(int(k)) + ")"
}
