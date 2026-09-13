package markdown

import (
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"
)

func TestEntities(t *testing.T) {
	t.Parallel()

	t.Run("holds the names of entities.json that end with a semicolon", func(t *testing.T) {
		t.Parallel()

		data, err := os.ReadFile("testdata/entities/entities.json")
		if err != nil {
			t.Fatal(err)
		}
		var refs map[string]struct{ Characters string }
		if err := json.Unmarshal(data, &refs); err != nil {
			t.Fatal(err)
		}
		var want []entity
		for ref, v := range refs {
			if name, ok := strings.CutSuffix(strings.TrimPrefix(ref, "&"), ";"); ok {
				want = append(want, entity{name, v.Characters})
			}
		}
		slices.SortFunc(want, func(a, b entity) int { return strings.Compare(a.name, b.name) })
		if got := entities[:]; !slices.Equal(got, want) {
			t.Fatalf("entities has %d rows, want the %d names of entities.json that end with a semicolon: run go generate", len(got), len(want))
		}
	})

	t.Run("fits every name and value in its bound", func(t *testing.T) {
		t.Parallel()

		for _, e := range entities {
			if len(e.name) > maxEntityName || len(e.chars) > len(valueReader{}.char) {
				t.Errorf("entity %q has %d name bytes and %d value bytes, want at most %d and %d",
					e.name, len(e.name), len(e.chars), maxEntityName, len(valueReader{}.char))
			}
		}
	})
}
