//go:build !unix

package markdown

import "os"

// maxrssBytes reports that the platform gives no peak resident set size.
func maxrssBytes(*os.ProcessState) (int64, bool) {
	return 0, false
}
