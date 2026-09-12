# commonmark.js regression tests

`regression.txt` is an unchanged copy of `test/regression.txt` from
[commonmark/commonmark.js](https://github.com/commonmark/commonmark.js) at tag
`0.31.2` (commit `cb2c2303d3550ec6ef28ceb2841f148e8761eebf`).

commonmark.js is Copyright (c) 2014, John MacFarlane, and is licensed under the
BSD 2-Clause License. `LICENSE` is an unchanged copy of its license file.

To fetch the copies again, run this from the repository root:

```sh
base=https://raw.githubusercontent.com/commonmark/commonmark.js/0.31.2
curl -fsSL -o internal/markdown/testdata/commonmark-js-regression/regression.txt "$base/test/regression.txt"
curl -fsSL -o internal/markdown/testdata/commonmark-js-regression/LICENSE "$base/LICENSE"
```
