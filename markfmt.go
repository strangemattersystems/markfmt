// Package markfmt formats Markdown in one canonical style.
package markfmt

import (
	"fmt"
	"io"
	"runtime/debug"

	"github.com/strangemattersystems/markfmt/internal/format"
)

// Format reads Markdown from r and writes it to w in the canonical style.
//
// Format writes nothing and returns an error if the input is larger than
// 8 MiB, if the output would be larger than 16 MiB, or if markfmt cannot show
// that the output has the meaning of the input. A panic while formatting is
// returned as an error with its stack.
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
			out, err = nil, fmt.Errorf("internal error: %v\n%s", v, debug.Stack())
		}
	}()
	src, err := io.ReadAll(io.LimitReader(r, format.MaxInput+1))
	if err != nil {
		return nil, err
	}
	return format.Source(src)
}
