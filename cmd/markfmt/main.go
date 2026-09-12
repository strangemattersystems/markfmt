// Markfmt formats Markdown files in one canonical style.
//
// Usage:
//
//	markfmt [-check] [path ...]
//
// Markfmt rewrites each path in place. With no path, or the path "-", it
// reads standard input and writes standard output.
//
// With -check, markfmt rewrites nothing. It prints each input that is not
// formatted and exits with status 1.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/strangemattersystems/markfmt"
)

func main() {
	check := flag.Bool("check", false, "print unformatted inputs and exit with status 1; rewrite nothing")
	flag.Parse()

	paths := flag.Args()
	if len(paths) == 0 {
		paths = []string{"-"}
	}

	status := 0
	for _, path := range paths {
		changed, err := run(path, *check)
		switch {
		case err != nil:
			fmt.Fprintf(os.Stderr, "markfmt: %s: %v\n", path, err)
			status = 2
		case changed && *check:
			fmt.Println(path)
			if status == 0 {
				status = 1
			}
		}
	}
	os.Exit(status)
}

// run formats one input and reports whether formatting changed it.
func run(path string, check bool) (bool, error) {
	var src []byte
	var err error
	if path == "-" {
		src, err = io.ReadAll(os.Stdin)
	} else {
		src, err = os.ReadFile(path)
	}
	if err != nil {
		return false, err
	}

	var out bytes.Buffer
	if err := markfmt.Format(&out, bytes.NewReader(src)); err != nil {
		return false, err
	}
	changed := !bytes.Equal(src, out.Bytes())

	switch {
	case check:
		return changed, nil
	case path == "-":
		_, err := os.Stdout.Write(out.Bytes())
		return changed, err
	case changed:
		return true, writeFile(path, out.Bytes())
	}
	return false, nil
}

// writeFile replaces the file at path with data. It renames a temporary file
// over the target, so an interrupted write leaves the original file intact.
func writeFile(path string, data []byte) error {
	// Resolve symlinks so the rename replaces the target, not the link.
	path, err := filepath.EvalSymlinks(path)
	if err != nil {
		return err
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".markfmt-*")
	if err != nil {
		return err
	}

	_, err = tmp.Write(data)
	if err == nil {
		err = tmp.Chmod(info.Mode().Perm())
	}
	if err == nil {
		err = tmp.Sync()
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(tmp.Name(), path)
	}
	if err != nil {
		_ = os.Remove(tmp.Name())
	}
	return err
}
