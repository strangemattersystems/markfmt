// Package markfmt formats Markdown in one canonical style.
package markfmt

import (
	"fmt"
	"io"
	"runtime/debug"

	"github.com/strangemattersystems/markfmt/internal/format"
)

// MaxInput and MaxOutput are the sizes in bytes above which [Format] returns
// an error that wraps [ErrTooLarge].
const (
	MaxInput  = format.MaxInput
	MaxOutput = format.MaxOutput
)

// ErrTooLarge is the error for an input above [MaxInput] or an output above
// [MaxOutput].
var ErrTooLarge = format.ErrTooLarge

// InternalError is a panic while formatting, which is a bug in markfmt.
type InternalError struct {
	Value any    // the value passed to panic
	Stack []byte // the stack of the goroutine at the panic
}

func (e *InternalError) Error() string {
	return fmt.Sprintf("internal error: %v", e.Value)
}

// Format reads Markdown from r and writes it to w in the canonical style.
//
// A block of the document whose canonical form markfmt cannot show to have
// the meaning of the input keeps the bytes of the input, with LF line
// endings, so Format changes nothing that it cannot keep.
//
// Format writes nothing and returns an error if the input or the output is
// too large. A panic while formatting is returned as an [*InternalError].
func Format(w io.Writer, r io.Reader) error {
	out, err := source(r)
	if err != nil {
		return err
	}
	_, err = w.Write(out)
	return err
}

// source reads r and formats it. It recovers a panic, so that a parser or
// printer bug fails one input and not the whole run of the CLI.
func source(r io.Reader) (out []byte, err error) {
	defer func() {
		if v := recover(); v != nil {
			out, err = nil, &InternalError{Value: v, Stack: debug.Stack()}
		}
	}()
	src, err := io.ReadAll(io.LimitReader(r, MaxInput+1))
	if err != nil {
		return nil, err
	}
	return format.Source(src)
}
