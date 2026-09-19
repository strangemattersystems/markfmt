package markfmt_test

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/strangemattersystems/markfmt"
)

func TestFormat(t *testing.T) {
	t.Parallel()

	t.Run("formats a document", func(t *testing.T) {
		t.Parallel()

		var out bytes.Buffer
		if err := markfmt.Format(&out, strings.NewReader("a\r\n")); err != nil || out.String() != "a\n" {
			t.Fatalf("Format = %q, %v, want \"a\\n\", nil", out.String(), err)
		}
	})

	t.Run("rejects an input above the default limit", func(t *testing.T) {
		t.Parallel()

		var out bytes.Buffer
		err := markfmt.Format(&out, io.LimitReader(zeros{}, markfmt.DefaultMaxInput+1))
		var tooLarge *markfmt.InputTooLargeError
		if !errors.As(err, &tooLarge) || tooLarge.Limit != markfmt.DefaultMaxInput || out.Len() > 0 {
			t.Fatalf("Format wrote %d bytes, error %v, want no output and an InputTooLargeError of %d", out.Len(), err, markfmt.DefaultMaxInput)
		}
	})
}

func TestFormatter_Format(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		maxInput int64
		in       string
		limit    int64 // the Limit of the error, or 0 for none
	}{
		{"formats an input at its limit", 2, "a\n", 0},
		{"rejects an input above its limit", 2, "ab\n", 2},
		{"takes an input above the default limit with a negative limit, which is 1 GiB", -1, strings.Repeat("a", markfmt.DefaultMaxInput) + "\n", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var out bytes.Buffer
			err := markfmt.Formatter{MaxInput: tt.maxInput}.Format(&out, strings.NewReader(tt.in))
			var tooLarge *markfmt.InputTooLargeError
			switch {
			case tt.limit == 0 && (err != nil || out.String() != tt.in):
				t.Fatalf("Format = %q, %v, want %q, nil", out.String(), err, tt.in)
			case tt.limit != 0 && (!errors.As(err, &tooLarge) || tooLarge.Limit != tt.limit || out.Len() > 0):
				t.Fatalf("Format wrote %d bytes, error %v, want no output and an InputTooLargeError of %d", out.Len(), err, tt.limit)
			}
		})
	}
}

type zeros struct{}

func (zeros) Read(b []byte) (int, error) {
	clear(b)
	return len(b), nil
}
