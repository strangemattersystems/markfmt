# GFM spec extension examples

`spec.txt` is an unchanged copy of the
[GitHub Flavored Markdown Spec](https://github.github.com/gfm/), version 0.29,
from `test/spec.txt` in [github/cmark-gfm](https://github.com/github/cmark-gfm)
at tag `0.29.0.gfm.13` (commit `587a12bb54d95ac37241377e6ddc93ea0e45439b`).

Only the examples in sections marked "(extension)" run. The other examples are
older copies of CommonMark examples; `../commonmark` has the current ones. The
reader skips the two examples marked "disabled", as cmark-gfm does, but counts
them in the example numbers.

The GFM Spec is licensed under
[CC BY-SA 4.0](https://creativecommons.org/licenses/by-sa/4.0/), as its front
matter states. It extends the CommonMark Spec, Copyright (C) 2014-16 John
MacFarlane.

To fetch the copy again, run this from the repository root:

```sh
curl -fsSL -o internal/markdown/testdata/gfm/spec.txt \
  https://raw.githubusercontent.com/github/cmark-gfm/0.29.0.gfm.13/test/spec.txt
```
