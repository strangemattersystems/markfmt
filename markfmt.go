// Package markfmt formats Markdown in one canonical style.
package markfmt

import (
	"fmt"
	"io"

	"github.com/strangemattersystems/markfmt/internal/format"
)

// DefaultMaxInput is the input limit of a [Formatter] whose
// [Formatter.MaxInput] is 0. Parsing uses memory in proportion to the input,
// up to about 500 times its size for deeply nested input, so the default
// keeps the worst case under 4 GiB.
const DefaultMaxInput = 8 << 20

// maxInput is the largest input that the parser can index: its node indices
// are 32 bits, and an input has at most about 2 nodes per byte.
const maxInput = 1 << 30

// A Formatter formats Markdown. The zero value is ready to use.
type Formatter struct {
	// MaxInput is the largest input in bytes. 0 means [DefaultMaxInput]. A
	// negative value, or a value above 1 GiB, means 1 GiB, the largest input
	// that markfmt can parse.
	MaxInput int64
}

// Format reads Markdown from r and writes it to w in the canonical style.
//
// A block of the document whose canonical form markfmt cannot show to have
// the meaning of the input keeps the bytes of the input, with LF line
// endings, so Format changes nothing that it cannot keep.
//
// Format writes nothing and returns an [*InputTooLargeError] if the input is
// larger than the input limit of f.
func (f Formatter) Format(w io.Writer, r io.Reader) error {
	limit := f.MaxInput
	switch {
	case limit == 0:
		limit = DefaultMaxInput
	case limit < 0 || limit > maxInput:
		limit = maxInput
	}
	src, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return err
	}
	if int64(len(src)) > limit {
		return &InputTooLargeError{Limit: limit}
	}
	out, err := format.Source(src)
	if err != nil {
		return err
	}
	_, err = w.Write(out)
	return err
}

// Format formats Markdown with the zero [Formatter].
func Format(w io.Writer, r io.Reader) error {
	return Formatter{}.Format(w, r)
}

// InputTooLargeError is the error for an input larger than the input limit
// of a [Formatter].
type InputTooLargeError struct {
	Limit int64
}

func (e *InputTooLargeError) Error() string {
	return fmt.Sprintf("input is larger than %d bytes", e.Limit)
}
