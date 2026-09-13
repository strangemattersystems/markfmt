package markdown

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEqual(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("testdata/pairs/pairs.txt")
	if err != nil {
		t.Fatal(err)
	}
	listed := make(map[string]bool)
	for line := range strings.Lines(string(data)) {
		if strings.HasPrefix(line, "#") || strings.TrimSpace(line) == "" {
			continue
		}
		name, rest, _ := strings.Cut(strings.TrimSpace(line), " ")
		verdict, reason, _ := strings.Cut(rest, " ")
		listed[name] = true
		t.Run(strings.ReplaceAll(name, "-", " ")+" is "+verdict, func(t *testing.T) {
			t.Parallel()

			a, b := readPair(t, name)
			err := Equal(a, b)
			htmlA, htmlB := normalizeHTML(renderHTML(a)), normalizeHTML(renderHTML(b))
			switch verdict {
			case "equal":
				if err != nil {
					t.Errorf("Equal = %v, want nil: %s", err, reason)
				}
				if htmlA != htmlB {
					t.Errorf("the pair renders different test HTML, so it cannot be equal:\n%q\n%q", htmlA, htmlB)
				}
			case "different":
				if err == nil {
					t.Errorf("Equal = nil, want a difference: %s", reason)
				}
				if htmlA == htmlB {
					t.Errorf("the pair renders equal test HTML %q, so it needs a GitHub fixture to be different", htmlA)
				}
			default:
				t.Fatalf("verdict %q is not equal or different", verdict)
			}
		})
	}

	t.Run("rejects a line break against no line break", func(t *testing.T) {
		t.Parallel()

		// No pair holds this case: the test HTML normalizes the line ending
		// to a space.
		if err := Equal(Parse([]byte("a\nb")), Parse([]byte("ab"))); err == nil {
			t.Fatal("Equal = nil, want a difference")
		}
	})

	t.Run("rejects a soft break against a space", func(t *testing.T) {
		t.Parallel()

		// No pair holds this case: the test HTML normalizes the line ending
		// to a space (design 8.4).
		if err := Equal(Parse([]byte("a\nb")), Parse([]byte("a b"))); err == nil {
			t.Fatal("Equal = nil, want a difference")
		}
	})

	t.Run("lists every pair", func(t *testing.T) {
		t.Parallel()

		files, err := filepath.Glob("testdata/pairs/*.a.md")
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range files {
			if name := strings.TrimSuffix(filepath.Base(f), ".a.md"); !listed[name] {
				t.Errorf("pair %q is not in pairs.txt", name)
			}
		}
	})
}

// readPair parses the files of pair name and verifies both trees.
func readPair(t *testing.T, name string) (*Tree, *Tree) {
	t.Helper()

	var trees [2]*Tree
	for i, suffix := range []string{".a.md", ".b.md"} {
		src, err := os.ReadFile(filepath.Join("testdata/pairs", name+suffix))
		if err != nil {
			t.Fatal(err)
		}
		trees[i] = Parse(src)
		if err := trees[i].Verify(); err != nil {
			t.Fatalf("%s%s: %v", name, suffix, err)
		}
	}
	return trees[0], trees[1]
}

func FuzzEqual(f *testing.F) {
	for _, src := range []string{
		"- a\n- b\n\n1. c\n2. d\n",
		"> ```\n> a\n> ```\n>\n> b\r\n",
		"    code\n\n\nb\n\n~~~ info\nx\n~~~\n",
		"# h\n\n<div>\n\n[a]: /u 't'\n---\n",
		">     code\n>\n> - x\n>\n>   y\n",
		"a  \nb \\\n> c \nd  \n",
		"\\# a \\* b\\\\\n\\- c\n",
		"&amp; &ast;a&#42; &#x2d; b\n",
		"`a` `` b\n` `` ``` c`",
		"<https://a.b> <a@b.c>\n",
	} {
		for op := range byte(mutations) {
			f.Add([]byte(src), op)
		}
	}

	f.Fuzz(func(t *testing.T, src []byte, op byte) {
		a := Parse(src)
		mutated := mutateSyntax(a, op)
		b := Parse(mutated)
		if err := b.Verify(); err != nil {
			t.Fatalf("Parse(%q).Verify() = %v", mutated, err)
		}
		if Equal(a, b) != nil {
			return
		}
		if htmlA, htmlB := normalizeHTML(renderHTML(a)), normalizeHTML(renderHTML(b)); htmlA != htmlB {
			t.Fatalf("Equal accepts %q and its mutation %q, whose test HTML differs:\n%q\n%q", src, mutated, htmlA, htmlB)
		}
	})
}

const mutations = 10

// mutateSyntax returns the source of tree with one kind of block syntax
// changed everywhere, chosen by op: bullet characters, line endings, ordered
// delimiters, fence characters, the number of blank lines, the space after a
// block quote marker, trailing spaces, the backslash of escapes, entity
// references as their characters, or the length of code span fences (design
// 10.5). A mutation may change meaning.
func mutateSyntax(tree *Tree, op byte) []byte {
	var out []byte
	for i, n := range tree.nodes {
		if n.kind.class() == classStructure {
			continue
		}
		b := tree.src[n.start:n.end]
		switch op % mutations {
		case 0:
			if n.kind == ListMarker {
				b = swapBytes(b, "-*+", "*+-")
			}
		case 1:
			if n.kind == LineEnding || n.kind == VerbatimLineEnding || n.kind == BlankLine {
				b = swapLineEnding(b)
			}
		case 2:
			if n.kind == ListMarker {
				b = swapBytes(b, ".)", ").")
			}
		case 3:
			if n.kind == FenceMarker {
				b = swapBytes(b, "`~", "~`")
			}
		case 4:
			if n.kind == BlankLine {
				b = bytes.Repeat(b, 2)
			}
		case 5:
			if n.kind == QuoteMarker {
				if trimmed, ok := bytes.CutSuffix(b, []byte("> ")); ok {
					b = append(bytes.Clone(trimmed), '>')
				} else {
					b = append(bytes.Clone(b), ' ')
				}
			}
		case 6:
			if n.kind == TrailingSpace {
				continue
			}
		case 7:
			if n.kind == Escape {
				b = b[1:]
			}
		case 8:
			if n.kind == EntityRef {
				b = tree.AppendValue(nil, NodeID(i))
			}
		case 9:
			if n.kind == CodeFence {
				b = bytes.Repeat(b, 2)
			}
		}
		out = append(out, b...)
	}
	return out
}

// swapBytes returns a copy of b with each byte in from replaced by the byte at
// the same index in to.
func swapBytes(b []byte, from, to string) []byte {
	out := bytes.Clone(b)
	for i, c := range out {
		if j := strings.IndexByte(from, c); j >= 0 {
			out[i] = to[j]
		}
	}
	return out
}

// swapLineEnding returns b with its line ending swapped: CRLF becomes LF, and
// LF or CR becomes CRLF.
func swapLineEnding(b []byte) []byte {
	switch {
	case bytes.HasSuffix(b, []byte("\r\n")):
		return append(bytes.Clone(b[:len(b)-2]), '\n')
	case bytes.HasSuffix(b, []byte("\n")), bytes.HasSuffix(b, []byte("\r")):
		return append(bytes.Clone(b[:len(b)-1]), '\r', '\n')
	}
	return b
}
