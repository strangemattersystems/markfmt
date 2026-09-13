package markdown

import "strconv"

// Kind is the kind of a [Node]. Appendix A of docs/design/parser.md lists
// every kind.
type Kind uint8

const (
	Document Kind = iota
	FrontMatter
	BlockQuote
	List
	ListItem
	Paragraph
	ThematicBreak
	Heading
	CodeBlock
	HTMLBlock
	LinkReferenceDefinition
	SoftBreak
	HardBreak
	CodeSpan
	Autolink
	RawHTML
	Emphasis
	Strong
	Link
	Image
	Text
	CodeText
	VerbatimLineEnding
	InfoString
	HTMLText
	LinkLabel
	Destination
	Title
	FrontMatterText
	Escape
	EntityRef
	AutolinkText
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
	ListMarker
	ItemIndent
	Bracket
	Colon
	AngleBracket
	TitleQuote
	FrontMatterFence
	TrailingSpace
	HardBreakMarker
	CodeFence
	Delimiter
	Paren
	Strikethrough
	Table
	TableRow
	TableCell
	TablePipe
	TableDelimiter
	CellPipeEscape
	TaskBox
	FootnoteDefinition
	FootnoteIndent
	FootnoteLabel
	Caret
	FootnoteReference
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
	case Document, FrontMatter, BlockQuote, List, ListItem, Paragraph, ThematicBreak, Heading, CodeBlock, HTMLBlock, LinkReferenceDefinition,
		SoftBreak, HardBreak, CodeSpan, Autolink, RawHTML, Emphasis, Strong, Link, Image, Strikethrough,
		Table, TableRow, TableCell, FootnoteDefinition, FootnoteReference:
		return classStructure
	case Text, CodeText, VerbatimLineEnding, InfoString, HTMLText, LinkLabel, Destination, Title, FrontMatterText, Escape, EntityRef, AutolinkText, CellPipeEscape, FootnoteLabel:
		return classContent
	case BOM, LineEnding, BlankLine, Indent, ThematicRun, ATXMarker, ATXClose, Whitespace, CodeIndent, FenceMarker, SetextUnderline, QuoteMarker, ListMarker, ItemIndent, Bracket, Colon, AngleBracket, TitleQuote, FrontMatterFence,
		TrailingSpace, HardBreakMarker, CodeFence, Delimiter, Paren, TablePipe, TableDelimiter, TaskBox, FootnoteIndent, Caret:
		return classSyntax
	}
	return classInvalid
}

// owner returns the kind of the container that owns a prefix leaf of kind k,
// and whether k is a prefix kind.
func (k Kind) owner() (Kind, bool) {
	switch k {
	case QuoteMarker:
		return BlockQuote, true
	case ListMarker, ItemIndent:
		return ListItem, true
	case FootnoteIndent:
		return FootnoteDefinition, true
	}
	return 0, false
}

// validFlags reports whether f is a flags value in range for kind k.
func (k Kind) validFlags(f uint8) bool {
	switch k {
	case HTMLBlock:
		return 1 <= f && f <= 7
	case List:
		return f <= 1
	case ListItem:
		return f == 0 || f == taskFlag || f == taskFlag|checkedFlag
	case Link, Image:
		return f <= 3
	case TableCell:
		return f <= 7
	case FootnoteReference:
		return f <= 1
	case CellPipeEscape:
		// The kind of the content leaves around it (design 10.3).
		switch Kind(f) {
		case Text, CodeText, HTMLText, AutolinkText, Destination, Title, LinkLabel, FootnoteLabel:
			return true
		}
		return false
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
	case List:
		return "List"
	case ListItem:
		return "ListItem"
	case ListMarker:
		return "ListMarker"
	case ItemIndent:
		return "ItemIndent"
	case LinkReferenceDefinition:
		return "LinkReferenceDefinition"
	case LinkLabel:
		return "LinkLabel"
	case Destination:
		return "Destination"
	case Title:
		return "Title"
	case Bracket:
		return "Bracket"
	case Colon:
		return "Colon"
	case AngleBracket:
		return "AngleBracket"
	case TitleQuote:
		return "TitleQuote"
	case FrontMatter:
		return "FrontMatter"
	case FrontMatterFence:
		return "FrontMatterFence"
	case FrontMatterText:
		return "FrontMatterText"
	case Escape:
		return "Escape"
	case EntityRef:
		return "EntityRef"
	case SoftBreak:
		return "SoftBreak"
	case HardBreak:
		return "HardBreak"
	case TrailingSpace:
		return "TrailingSpace"
	case HardBreakMarker:
		return "HardBreakMarker"
	case CodeSpan:
		return "CodeSpan"
	case CodeFence:
		return "CodeFence"
	case Autolink:
		return "Autolink"
	case AutolinkText:
		return "AutolinkText"
	case RawHTML:
		return "RawHTML"
	case Emphasis:
		return "Emphasis"
	case Strong:
		return "Strong"
	case Delimiter:
		return "Delimiter"
	case Link:
		return "Link"
	case Image:
		return "Image"
	case Paren:
		return "Paren"
	case Strikethrough:
		return "Strikethrough"
	case Table:
		return "Table"
	case TableRow:
		return "TableRow"
	case TableCell:
		return "TableCell"
	case TablePipe:
		return "TablePipe"
	case TableDelimiter:
		return "TableDelimiter"
	case CellPipeEscape:
		return "CellPipeEscape"
	case TaskBox:
		return "TaskBox"
	case FootnoteDefinition:
		return "FootnoteDefinition"
	case FootnoteIndent:
		return "FootnoteIndent"
	case FootnoteLabel:
		return "FootnoteLabel"
	case Caret:
		return "Caret"
	case FootnoteReference:
		return "FootnoteReference"
	}
	return "Kind(" + strconv.Itoa(int(k)) + ")"
}
