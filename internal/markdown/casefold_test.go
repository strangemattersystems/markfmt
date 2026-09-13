package markdown

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"
)

func TestCaseFolds(t *testing.T) {
	t.Parallel()

	t.Run("holds the full case folding rows of CaseFolding.txt", func(t *testing.T) {
		t.Parallel()

		data, err := os.ReadFile("testdata/unicode/CaseFolding.txt")
		if err != nil {
			t.Fatal(err)
		}
		var want []caseFold
		for line := range strings.Lines(string(data)) {
			fields := strings.Split(line, "; ")
			if strings.HasPrefix(line, "#") || len(fields) < 3 || fields[1] != "C" && fields[1] != "F" {
				continue
			}
			var r rune
			if _, err := fmt.Sscanf(fields[0], "%x", &r); err != nil {
				t.Fatal(err)
			}
			var to []rune
			for code := range strings.FieldsSeq(fields[2]) {
				var c rune
				if _, err := fmt.Sscanf(code, "%x", &c); err != nil {
					t.Fatal(err)
				}
				to = append(to, c)
			}
			want = append(want, caseFold{r, string(to)})
		}
		if got := caseFolds[:]; !slices.Equal(got, want) {
			t.Fatalf("caseFolds has %d rows, want the %d C and F rows of CaseFolding.txt: run go generate", len(got), len(want))
		}
	})
}
