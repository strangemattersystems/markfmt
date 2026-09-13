package markdown

import (
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
)

func TestWideRanges(t *testing.T) {
	t.Parallel()

	t.Run("holds the merged W and F rows of EastAsianWidth.txt", func(t *testing.T) {
		t.Parallel()

		data, err := os.ReadFile("testdata/unicode/EastAsianWidth.txt")
		if err != nil {
			t.Fatal(err)
		}
		var want []runeRange
		for line := range strings.Lines(string(data)) {
			row, _, _ := strings.Cut(line, "#")
			codes, width, ok := strings.Cut(row, ";")
			if width = strings.TrimSpace(width); !ok || width != "W" && width != "F" {
				continue
			}
			first, last, ranged := strings.Cut(strings.TrimSpace(codes), "..")
			if !ranged {
				last = first
			}
			lo, err := strconv.ParseInt(first, 16, 32)
			if err != nil {
				t.Fatal(err)
			}
			hi, err := strconv.ParseInt(last, 16, 32)
			if err != nil {
				t.Fatal(err)
			}
			if n := len(want); n > 0 && want[n-1].hi+1 == rune(lo) {
				want[n-1].hi = rune(hi)
				continue
			}
			want = append(want, runeRange{rune(lo), rune(hi)})
		}
		if got := wideRanges[:]; !slices.Equal(got, want) {
			t.Fatalf("wideRanges has %d ranges, want the %d merged W and F ranges of EastAsianWidth.txt: run go generate", len(got), len(want))
		}
	})
}

func TestDisplayWidth(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		s    string
		want int
	}{
		{"counts an ascii character as 1 column", "a", 1},
		{"counts a wide character as 2 columns", "日本", 4},
		{"counts a fullwidth character as 2 columns", "Ａ", 2},
		{"counts an emoji of wide presentation as 2 columns", "\U0001F600", 2},
		{"counts a nonspacing mark as 0 columns", "é", 1},
		{"counts an enclosing mark as 0 columns", "a⃝", 1},
		{"counts a control character as 0 columns", "\x00a" + string(rune(0x85)), 1},
		{"counts an invalid byte as 1 column", "\xff", 1},
		{"counts an ambiguous character as 1 column", "§", 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := DisplayWidth([]byte(tt.s)); got != tt.want {
				t.Fatalf("DisplayWidth(%q) = %d, want %d", tt.s, got, tt.want)
			}
		})
	}
}
