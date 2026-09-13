package format

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

func TestSource(t *testing.T) {
	t.Parallel()

	t.Run("cases", func(t *testing.T) {
		t.Parallel()

		cases := readCases(t)
		failing := readFailing(t, cases)
		for _, c := range cases {
			t.Run(strings.ReplaceAll(c.name, "-", " "), func(t *testing.T) {
				t.Parallel()

				got := checkSource(t, c.in)
				switch pass := bytes.Equal(got, c.out); {
				case pass && failing[c.name]:
					t.Error("passes: remove it from testdata/cases/failing.txt")
				case !pass && !failing[c.name]:
					t.Errorf("Source(%q)\n got: %q\nwant: %q", c.in, got, c.out)
				}
			})
		}
	})

	t.Run("has a case for each github printer fixture", func(t *testing.T) {
		t.Parallel()

		cases := readCases(t)
		for _, name := range readSections(t, "../markdown/testdata/github/printer.txt") {
			in, err := os.ReadFile("../markdown/testdata/github/input/printer/" + name + ".md")
			if err != nil {
				t.Fatal(err)
			}
			want := "github-" + strings.ReplaceAll(name, "/", "-")
			if !slices.ContainsFunc(cases, func(c testCase) bool { return c.name == want && bytes.Equal(c.in, in) }) {
				t.Errorf("fixture %s has no case %s with its input", name, want)
			}
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

	t.Run("keeps the short rows of a table at the missing cell cap", func(t *testing.T) {
		t.Parallel()

		// A header of 1,000 cells and 525 rows of one cell reach the cap of
		// missing cells. The rows are long enough that padding them would add
		// fewer bytes than the table has.
		row := "| " + strings.Repeat("x", 7000) + " |"
		in := strings.Repeat("| a ", 1000) + "|\n" + strings.Repeat("| --- ", 1000) + "|\n" + strings.Repeat(row+"\n", 525)
		out := checkSource(t, []byte(in))
		for i, line := range strings.Split(strings.TrimSuffix(string(out), "\n"), "\n")[2:] {
			if line != row {
				t.Fatalf("row %d has %d bytes, want the %d bytes of its input", i+1, len(line), len(row))
			}
		}
	})

	t.Run("rejects an input above the input limit", func(t *testing.T) {
		t.Parallel()

		if _, err := Source(make([]byte, MaxInput+1)); err == nil {
			t.Fatal("Source of MaxInput+1 bytes gives no error")
		}
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

// readSections returns the names of the sections of a GitHub fixture file.
func readSections(t testing.TB, path string) []string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for line := range strings.Lines(string(data)) {
		if name, ok := strings.CutPrefix(line, "## "); ok {
			names = append(names, strings.TrimSpace(name))
		}
	}
	if len(names) == 0 {
		t.Fatalf("%s: no sections", path)
	}
	return names
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
		c := testCase{name: filepath.Base(base)}
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

// readFailing reads the names in testdata/cases/failing.txt, the cases that
// Source does not pass yet. The list only gets shorter: an entry that names
// no case is an error.
func readFailing(t *testing.T, cases []testCase) map[string]bool {
	t.Helper()

	data, err := os.ReadFile("testdata/cases/failing.txt")
	if err != nil {
		t.Fatal(err)
	}
	failing := make(map[string]bool)
	for line := range strings.Lines(string(data)) {
		name, _, _ := strings.Cut(line, "#")
		if name = strings.TrimSpace(name); name == "" {
			continue
		}
		if !slices.ContainsFunc(cases, func(c testCase) bool { return c.name == name }) {
			t.Errorf("failing.txt: %q names no case", name)
		}
		failing[name] = true
	}
	return failing
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
