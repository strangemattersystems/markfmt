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

	t.Run("rejects an input above the input limit", func(t *testing.T) {
		t.Parallel()

		var out bytes.Buffer
		err := markfmt.Format(&out, io.LimitReader(zeros{}, markfmt.MaxInput+1))
		if !errors.Is(err, markfmt.ErrTooLarge) || out.Len() > 0 {
			t.Fatalf("Format wrote %d bytes, error %v, want no output and ErrTooLarge", out.Len(), err)
		}
	})

	t.Run("returns a panic as an internal error with its stack", func(t *testing.T) {
		t.Parallel()

		var out bytes.Buffer
		err := markfmt.Format(&out, panicReader{})
		var ie *markfmt.InternalError
		if !errors.As(err, &ie) || ie.Value != "boom" || !bytes.Contains(ie.Stack, []byte("goroutine")) {
			t.Fatalf("Format error = %#v, want an InternalError with the panic value and a stack", err)
		}
		if strings.Contains(err.Error(), "goroutine") {
			t.Fatalf("Error() = %q, want no stack", err.Error())
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
