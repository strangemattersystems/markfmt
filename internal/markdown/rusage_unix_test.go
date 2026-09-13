//go:build unix

package markdown

import (
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"syscall"
	"testing"
)

// maxrssBytes returns the peak resident set size of the process that ps
// describes, in bytes, and whether the platform reports it. Linux reports
// Maxrss in KiB, and darwin in bytes.
func maxrssBytes(ps *os.ProcessState) (int64, bool) {
	ru, ok := ps.SysUsage().(*syscall.Rusage)
	if !ok {
		return 0, false
	}
	maxrss := int64(ru.Maxrss) //nolint:unconvert // Maxrss is int32 on some platforms.
	if runtime.GOOS == "darwin" {
		return maxrss, true
	}
	return maxrss << 10, true
}

func TestMaxrssBytes(t *testing.T) {
	if size := os.Getenv("MARKFMT_TOUCH_BYTES"); size != "" {
		n, err := strconv.Atoi(size)
		if err != nil {
			t.Fatal(err)
		}
		b := make([]byte, n)
		for i := 0; i < n; i += 4096 {
			b[i] = 1
		}
		return
	}
	t.Parallel()

	if os.Getenv("MARKFMT_LONG") != "1" {
		t.Skip("set MARKFMT_LONG=1 to run")
	}
	run := func(n int) int64 {
		cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestMaxrssBytes$") //nolint:gosec // The command is the test binary itself.
		cmd.Env = append(os.Environ(), "MARKFMT_TOUCH_BYTES="+strconv.Itoa(n))
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("child that touches %d bytes: %v\n%s", n, err, out)
		}
		rss, ok := maxrssBytes(cmd.ProcessState)
		if !ok {
			t.Skip("the platform reports no peak resident set size")
		}
		return rss
	}

	t.Run("gives the bytes that a child touches", func(t *testing.T) {
		const touched = 256 << 20
		if got := run(touched) - run(0); got < touched*3/4 || got > touched*2 {
			t.Fatalf("maxrssBytes grows by %d bytes for %d bytes touched", got, touched)
		}
	})
}
