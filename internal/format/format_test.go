package format

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/strangemattersystems/markfmt/internal/markdown"
)

func TestCheck(t *testing.T) {
	t.Parallel()

	t.Run("accepts an output with the meaning of the input", func(t *testing.T) {
		t.Parallel()

		if err := check(markdown.Parse([]byte("*a*\n")), []byte("_a_\n")); err != nil {
			t.Fatalf("check gives %v, want no error", err)
		}
	})

	t.Run("rejects an output that changes the meaning", func(t *testing.T) {
		t.Parallel()

		if err := check(markdown.Parse([]byte("*a*\n")), []byte("a\n")); err == nil {
			t.Fatal("check gives no error for an output without its emphasis")
		}
	})
}

func TestSourceTree(t *testing.T) {
	t.Parallel()

	t.Run("rejects an output above the limit", func(t *testing.T) {
		t.Parallel()

		if _, err := sourceTree(markdown.Parse([]byte("# a\n")), 2); err == nil {
			t.Fatal("sourceTree gives no error for an output above the limit")
		}
	})

	t.Run("prints a tree within the limit", func(t *testing.T) {
		t.Parallel()

		if out, err := sourceTree(markdown.Parse([]byte("#  a\n")), MaxOutput); err != nil || string(out) != "# a\n" {
			t.Fatalf("sourceTree = %q, %v, want %q", out, err, "# a\n")
		}
	})
}

func TestSource(t *testing.T) {
	t.Parallel()

	t.Run("formats a link title in text that holds its quotes", func(t *testing.T) {
		t.Parallel()

		for _, src := range []string{
			`[](0 "[](0 "")")`,
			`[[](0 '](0 '')')`,
			`[](0 "[](0 ')')`,
		} {
			checkSource(t, []byte(src))
		}
	})

	t.Run("formats a dialect span whose lines hold tabs and deep markers", func(t *testing.T) {
		t.Parallel()

		for _, src := range []string{
			strings.Repeat("- ", 100) + "a\n\n" + strings.Repeat("\t", 50) + "b\n",
			strings.Repeat(">", 99) + "* ",
			strings.Repeat("* ", 100) + "*\x00",
			"-  " + strings.Repeat("- ", 99) + "a\n\n\t£_b_£\n",
		} {
			checkSource(t, []byte(src))
		}
	})

	t.Run("cases", func(t *testing.T) {
		t.Parallel()

		for _, c := range readCases(t) {
			t.Run(strings.ReplaceAll(c.name, "-", " "), func(t *testing.T) {
				t.Parallel()

				if got := checkSource(t, c.in); !bytes.Equal(got, c.out) {
					t.Errorf("Source(%q)\n got: %q\nwant: %q", c.in, got, c.out)
				}
			})
		}
	})

	t.Run("has a case for each github printer fixture", func(t *testing.T) {
		t.Parallel()

		cases := readCases(t)
		for name := range readFixtures(t, "../markdown/testdata/github/printer.txt") {
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

	t.Run("renders the output of each github printer fixture as github renders its input", func(t *testing.T) {
		t.Parallel()

		// The outputs and their HTML come from the GitHub Markdown API
		// (testdata/github/README.md in internal/markdown).
		inputs := readFixtures(t, "../markdown/testdata/github/printer.txt")
		outputs := readFixtures(t, "../markdown/testdata/github/printer-output.txt")
		cases := readCases(t)
		for name, in := range inputs {
			out, ok := outputs[name]
			i := slices.IndexFunc(cases, func(c testCase) bool { return c.name == "github-"+strings.ReplaceAll(name, "/", "-") })
			switch {
			case !ok || i < 0 || out.markdown != string(cases[i].out):
				t.Errorf("printer-output.txt has no current output of fixture %s: capture it again", name)
			case runID.ReplaceAllString(out.html, "") != runID.ReplaceAllString(in.html, ""):
				t.Errorf("GitHub renders the output of fixture %s differently from its input\n  input: %q\n output: %q", name, in.html, out.html)
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

// runID matches the attribute that GitHub gives math with a new value for each
// request.
var runID = regexp.MustCompile(` data-run-id="[0-9a-f]*"`)

type fixture struct {
	markdown, html string
}

// readFixtures reads the examples of a GitHub fixture file, by section name.
func readFixtures(t testing.TB, path string) map[string]fixture {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	const fence = "````````````````````````````````"
	fixtures := make(map[string]fixture)
	var name string
	var f fixture
	state := 0 // 0 text, 1 Markdown, 2 HTML
	for line := range strings.Lines(string(data)) {
		switch {
		case state == 0 && strings.HasPrefix(line, "## "):
			name = strings.TrimSpace(line[3:])
		case state == 0 && line == fence+" example\n":
			state, f = 1, fixture{}
		case state == 1 && line == ".\n":
			state = 2
		case state == 1:
			f.markdown += line
		case state == 2 && line == fence+"\n":
			state, fixtures[name] = 0, f
		case state == 2:
			f.html += line
		}
	}
	if len(fixtures) == 0 {
		t.Fatalf("%s: no fixtures", path)
	}
	return fixtures
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
