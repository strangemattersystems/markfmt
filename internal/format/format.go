// Package format rewrites Markdown source in the markfmt canonical style.
package format

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/strangemattersystems/markfmt/internal/markdown"
)

// MaxInput and MaxOutput are the sizes in bytes above which [Source] returns
// an error (design 7.2).
const (
	MaxInput  = 8 << 20
	MaxOutput = 16 << 20
)

// Source returns src in the canonical style, with LF line endings. A
// top-level block whose canonical form would change what the input means
// keeps the bytes of the input, and where no block can be found for a
// difference, the whole input stays as it is: Source formats what it can,
// and changes nothing that it cannot keep.
//
// Source returns an error only when src is larger than [MaxInput] or the
// output would be larger than [MaxOutput].
func Source(src []byte) ([]byte, error) {
	if len(src) > MaxInput {
		return nil, fmt.Errorf("input of %d bytes is larger than %d bytes", len(src), MaxInput)
	}
	tree := markdown.Parse(src)
	var raw map[markdown.NodeID]bool
	// Each retry prints and checks the whole input again, so a few are
	// allowed: a finding of the check is rare, and nearly always one block.
	for range maxRetries {
		out, err := sourceTree(tree, raw, MaxOutput)
		var m *markdown.MismatchError
		switch {
		case err == nil:
			return out, nil
		case !errors.As(err, &m):
			return nil, err
		}
		block, ok := topLevelBlock(tree, m.At)
		if !ok {
			break
		}
		if raw[block] {
			// The block keeps its bytes, so the difference comes from the
			// block before it, which the canonical form of that one joins.
			if block, ok = blockBefore(tree, block); !ok || raw[block] {
				break
			}
		}
		if raw == nil {
			raw = make(map[markdown.NodeID]bool)
		}
		raw[block] = true
	}
	return bytes.Clone(src), nil
}

// maxRetries bounds the prints of [Source] for one input.
const maxRetries = 4

// Strict returns src in the canonical style, or an error where [Source] would
// keep bytes of the input: the tests of the printer use it, so that a block
// that the printer cannot write is a failure and not a quiet copy.
func Strict(src []byte) ([]byte, error) {
	if len(src) > MaxInput {
		return nil, fmt.Errorf("input of %d bytes is larger than %d bytes", len(src), MaxInput)
	}
	return sourceTree(markdown.Parse(src), nil, MaxOutput)
}

// sourceTree prints tree, with the top-level blocks in raw as the input holds
// them, and checks the output. limit is the output limit, which a test
// lowers.
func sourceTree(tree *markdown.Tree, raw map[markdown.NodeID]bool, limit int) ([]byte, error) {
	p := printer{tree: tree, max: limit, raw: raw}
	p.document()
	if p.full {
		return nil, fmt.Errorf("output is larger than %d bytes", limit)
	}
	if bytes.Equal(p.out, tree.Raw(0)) {
		// The output parses to the tree of the input, so the check has
		// nothing to find, and a formatted file is the common input.
		return p.out, nil
	}
	if err := check(tree, p.out); err != nil {
		return nil, err
	}
	return p.out, nil
}

// check reports whether out, the printed form of tree, would change what the
// input means (design 10), or its kept syntax (design 12). The error holds a
// [markdown.MismatchError].
func check(tree *markdown.Tree, out []byte) error {
	printed := markdown.Parse(out)
	if err := markdown.Equal(tree, printed); err != nil {
		return fmt.Errorf("the output would change the meaning of the input: %w", err)
	}
	// Equal reads the value of a construct, where GitHub can read the bytes
	// that the kept syntax holds (design 12).
	if err := markdown.KeptMismatch(tree, printed); err != nil {
		return fmt.Errorf("the output would change the kept syntax of the input: %w", err)
	}
	return nil
}

// topLevelBlock returns the block of the document that holds node id.
func topLevelBlock(tree *markdown.Tree, id markdown.NodeID) (markdown.NodeID, bool) {
	for child := markdown.NodeID(1); int(child) < tree.Len(); {
		end, _ := tree.Next(child)
		if id >= child && id < end {
			return child, !tree.Kind(child).Leaf()
		}
		child = end
	}
	return 0, false
}

// blockBefore returns the block of the document before block id.
func blockBefore(tree *markdown.Tree, id markdown.NodeID) (markdown.NodeID, bool) {
	before, found := markdown.NodeID(0), false
	for child := markdown.NodeID(1); child < id; child, _ = tree.Next(child) {
		if !tree.Kind(child).Leaf() {
			before, found = child, true
		}
	}
	return before, found
}
