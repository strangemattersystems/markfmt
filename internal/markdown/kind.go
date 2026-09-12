package markdown

// Kind is the kind of a [Node]. Appendix A of docs/design/parser.md lists
// every kind.
type Kind uint8

const (
	Document Kind = iota
	Text
)

type class uint8

const (
	classInvalid class = iota
	classStructure
	classContent
)

// class returns the comparison class of k. Structure kinds are interior;
// content kinds are leaves.
func (k Kind) class() class {
	//exhaustive:enforce
	switch k {
	case Document:
		return classStructure
	case Text:
		return classContent
	}
	return classInvalid
}
