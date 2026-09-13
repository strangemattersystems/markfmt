// Package format rewrites Markdown source in the markfmt canonical style.
package format

import (
	"fmt"

	"github.com/strangemattersystems/markfmt/internal/markdown"
)

// MaxInput and MaxOutput are the sizes in bytes above which [Source] returns
// an error (design 7.2).
const (
	MaxInput  = 8 << 20
	MaxOutput = 16 << 20
)

// Source returns src in the canonical style, with LF line endings.
//
// Source returns an error, not output, when src is larger than [MaxInput],
// when the output is larger than [MaxOutput], and when the tree of the output
// is not [markdown.Equal] to the tree of src.
func Source(src []byte) ([]byte, error) {
	if len(src) > MaxInput {
		return nil, fmt.Errorf("input of %d bytes is larger than %d bytes", len(src), MaxInput)
	}
	in := markdown.Parse(src)
	p := printer{tree: in, max: MaxOutput}
	p.document()
	if p.full {
		return nil, fmt.Errorf("output is larger than %d bytes", MaxOutput)
	}
	if err := markdown.Equal(in, markdown.Parse(p.out)); err != nil {
		return nil, fmt.Errorf("the output would change the meaning of the input: %w", err)
	}
	return p.out, nil
}
