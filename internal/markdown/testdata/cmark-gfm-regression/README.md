# cmark-gfm regression tests

`regression.txt` is an unchanged copy of `test/regression.txt` from
[github/cmark-gfm](https://github.com/github/cmark-gfm) at tag `0.29.0.gfm.13`
(commit `587a12bb54d95ac37241377e6ddc93ea0e45439b`). Some examples have CR and
CRLF line endings on purpose.

cmark-gfm is Copyright (c) 2014, John MacFarlane, and is licensed under the BSD
2-Clause License. `COPYING` is an unchanged copy of cmark-gfm's license file.

To fetch the copies again, run this from the repository root:

```sh
base=https://raw.githubusercontent.com/github/cmark-gfm/0.29.0.gfm.13
curl -fsSL -o internal/markdown/testdata/cmark-gfm-regression/regression.txt "$base/test/regression.txt"
curl -fsSL -o internal/markdown/testdata/cmark-gfm-regression/COPYING "$base/COPYING"
```
