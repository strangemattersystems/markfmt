# Unicode case folding

`CaseFolding.txt` is an unchanged copy of the Unicode Character Database file
of that name, version 17.0.0, the Unicode version of Go 1.27's `unicode`
package. `gen_casefold.go` generates the full case folding table in
`casefold.go` from its C and F rows, and `casefold_test.go` checks the table
against it.

Copyright © 2025 Unicode®, Inc. The file is licensed under the
[Unicode License v3](https://www.unicode.org/license.txt), and its terms of
use are at <https://www.unicode.org/terms_of_use.html>.

To fetch the copy again, run this from the repository root:

```sh
curl -fsSL -o internal/markdown/testdata/unicode/CaseFolding.txt \
  https://www.unicode.org/Public/17.0.0/ucd/CaseFolding.txt
```
