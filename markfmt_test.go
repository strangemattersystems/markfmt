package markfmt_test

import (
	"bytes"
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

	t.Run("rejects an input above the input limit", func(t *testing.T) {
		t.Parallel()

		var out bytes.Buffer
		err := markfmt.Format(&out, io.LimitReader(zeros{}, 8<<20+1))
		if err == nil || out.Len() > 0 {
			t.Fatalf("Format wrote %d bytes, error %v, want no output and an error", out.Len(), err)
		}
	})

	t.Run("returns a panic as an error with its stack", func(t *testing.T) {
		t.Parallel()

		var out bytes.Buffer
		err := markfmt.Format(&out, panicReader{})
		if err == nil || !strings.Contains(err.Error(), "boom") || !strings.Contains(err.Error(), "goroutine") {
			t.Fatalf("Format error = %v, want the panic value and a stack", err)
		}
	})
}

type zeros struct{}

func (zeros) Read(b []byte) (int, error) {
	clear(b)
	return len(b), nil
}

type panicReader struct{}

func (panicReader) Read([]byte) (int, error) {
	panic("boom")
}
