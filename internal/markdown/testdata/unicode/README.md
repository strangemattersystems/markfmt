# Unicode data

`CaseFolding.txt` and `EastAsianWidth.txt` are unchanged copies of the Unicode
Character Database files of those names, version 17.0.0, the Unicode version
of Go 1.27's `unicode` package.

- `gen_casefold.go` generates the full case folding table in `casefold.go`
  from the C and F rows of `CaseFolding.txt`, and `casefold_test.go` checks
  the table against it.
- `gen_width.go` generates the wide character ranges in `width.go` from the W
  and F rows of `EastAsianWidth.txt`, and `width_test.go` checks the ranges
  against it.

Copyright © 2025 Unicode®, Inc. The files are licensed under the
[Unicode License v3](https://www.unicode.org/license.txt), and their terms of
use are at <https://www.unicode.org/terms_of_use.html>.

To fetch the copies again, run this from the repository root:

```sh
for f in CaseFolding.txt EastAsianWidth.txt; do
  curl -fsSL -o "internal/markdown/testdata/unicode/$f" \
    "https://www.unicode.org/Public/17.0.0/ucd/$f"
done
```
