package markdown

import "strconv"

// Kind is the kind of a [Node]. Appendix A of docs/design/parser.md lists
// every kind.
type Kind uint8

const (
	Document Kind = iota
	Paragraph
	ThematicBreak
	Heading
	Text
	BOM
	LineEnding
	BlankLine
	Indent
	ThematicRun
	ATXMarker
	ATXClose
	Whitespace
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
	case Document, Paragraph, ThematicBreak, Heading:
		return classStructure
	case Text:
		return classContent
	case BOM, LineEnding, BlankLine, Indent, ThematicRun, ATXMarker, ATXClose, Whitespace:
		return classSyntax
	}
	return classInvalid
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
	}
	return "Kind(" + strconv.Itoa(int(k)) + ")"
}
