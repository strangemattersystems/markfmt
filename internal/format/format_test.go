package format

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestSource(t *testing.T) {
	t.Parallel()

	t.Run("cases", func(t *testing.T) {
		t.Parallel()

		for _, c := range readCases(t) {
			t.Run(c.name, func(t *testing.T) {
				t.Parallel()

				if got := checkSource(t, c.in); !bytes.Equal(got, c.out) {
					t.Fatalf("Source(%q)\n got: %q\nwant: %q", c.in, got, c.out)
				}
			})
		}
	})

	t.Run("spec", func(t *testing.T) {
		t.Parallel()

		for _, ex := range readSpec(t) {
			t.Run(ex.name, func(t *testing.T) {
				t.Parallel()

				checkSource(t, ex.markdown)
			})
		}
	})
}

func TestCheckRendering(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		src, formatted string
		wantErr        bool
	}{
		{"accepts output that renders the same", "#   A\n", "# A\n", false},
		{"rejects output that renders differently", "a\n", "b\n", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			src := []byte(tt.src)
			err := checkRendering(src, markdown.Parse(src), []byte(tt.formatted))
			if (err != nil) != tt.wantErr {
				t.Fatalf("checkRendering(%q, %q) error = %v, want error %t", tt.src, tt.formatted, err, tt.wantErr)
			}
		})
	}
}

func FuzzSource(f *testing.F) {
	for _, c := range readCases(f) {
		f.Add(c.in)
	}
	for _, ex := range readSpec(f) {
		f.Add(ex.markdown)
	}

	f.Fuzz(func(t *testing.T, in []byte) {
		checkSource(t, in)
	})
}

// checkSource formats in, checks that the output is stable, and returns it.
func checkSource(t testing.TB, in []byte) []byte {
	t.Helper()

	once, err := Source(in)
	if err != nil {
		t.Fatalf("Source(%q) error: %v", in, err)
	}
	twice, err := Source(once)
	if err != nil {
		t.Fatalf("Source(%q) error: %v", once, err)
	}
	if !bytes.Equal(once, twice) {
		t.Fatalf("Source is not idempotent\ninput: %q\n once: %q\ntwice: %q", in, once, twice)
	}
	return once
}

type testCase struct {
	name    string
	in, out []byte
}

// readCases reads each testdata/cases/NAME.in.md file and the matching
// NAME.out.md file.
func readCases(t testing.TB) []testCase {
	t.Helper()

	inputs, err := filepath.Glob("testdata/cases/*.in.md")
	if err != nil {
		t.Fatal(err)
	}
	if len(inputs) == 0 {
		t.Fatal("no test cases in testdata/cases")
	}

	cases := make([]testCase, 0, len(inputs))
	for _, input := range inputs {
		base := strings.TrimSuffix(input, ".in.md")
		c := testCase{name: strings.ReplaceAll(filepath.Base(base), "-", " ")}
		if c.in, err = os.ReadFile(input); err != nil {
			t.Fatal(err)
		}
		if c.out, err = os.ReadFile(base + ".out.md"); err != nil {
			t.Fatal(err)
		}
		cases = append(cases, c)
	}
	return cases
}

type specExample struct {
	name     string
	markdown []byte
}

// readSpec reads the Markdown of each example in testdata/spec: the
// CommonMark examples in spec.json, and the goldmark cases in the .txt files.
func readSpec(t testing.TB) []specExample {
	t.Helper()

	data, err := os.ReadFile("testdata/spec/spec.json")
	if err != nil {
		t.Fatal(err)
	}
	var commonmark []struct {
		Markdown string `json:"markdown"`
		Example  int    `json:"example"`
	}
	if err := json.Unmarshal(data, &commonmark); err != nil {
		t.Fatal(err)
	}
	examples := make([]specExample, 0, len(commonmark))
	for _, ex := range commonmark {
		examples = append(examples, specExample{
			name:     "commonmark example " + strconv.Itoa(ex.Example),
			markdown: []byte(ex.Markdown),
		})
	}

	files, err := filepath.Glob("testdata/spec/*.txt")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		cases := readGoldmarkCases(strings.TrimSuffix(filepath.Base(path), ".txt"), string(data))
		if len(cases) == 0 {
			t.Fatalf("%s: no cases", path)
		}
		examples = append(examples, cases...)
	}
	return examples
}

// readGoldmarkCases reads the Markdown of each case in a goldmark test file.
// A case is a "N: description" header, an optional OPTIONS line, then the
// Markdown and the HTML, each opened by a separator line.
//
// The OPTIONS are not applied. An input with its escape sequences or
// surrounding space left in is still valid Markdown.
func readGoldmarkCases(file, data string) []specExample {
	const separator = "//- - - - - - - - -//"
	const end = "//= = = = = = = = = = = = = = = = = = = = = = = =//"

	var cases []specExample
	// goldmark reads these files with bufio.ScanLines, which drops a \r
	// before each \n. Some cases have CRLF line endings.
	lines := strings.Split(strings.ReplaceAll(data, "\r\n", "\n"), "\n")
	for i := 0; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "" {
			continue
		}
		no, _, _ := strings.Cut(lines[i], ":")
		for i < len(lines) && lines[i] != separator {
			i++
		}
		start := i + 1
		for i = start; i < len(lines) && lines[i] != separator; i++ {
		}
		cases = append(cases, specExample{
			name:     file + " example " + strings.TrimSpace(no),
			markdown: []byte(strings.Join(lines[start:min(i, len(lines))], "\n")),
		})
		for i < len(lines) && lines[i] != end {
			i++
		}
	}
	return cases
}
