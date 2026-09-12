// Package markfmt formats Markdown in one canonical style.
package markfmt

import (
	"io"

	"github.com/strangemattersystems/markfmt/internal/format"
)

// Format reads Markdown from r and writes it to w in the canonical style.
//
// Format writes nothing and returns an error if the formatted document would
// render differently from the input.
func Format(w io.Writer, r io.Reader) error {
	// ponytail: reads the whole document into memory. Replace with a
	// two-pass parser if very large inputs matter.
	src, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	out, err := format.Source(src)
	if err != nil {
		return err
	}
	_, err = w.Write(out)
	return err
}
