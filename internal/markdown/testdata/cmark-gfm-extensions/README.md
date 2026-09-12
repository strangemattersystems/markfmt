# cmark-gfm extension tests

`extensions.txt` is an unchanged copy of `test/extensions.txt` from
[github/cmark-gfm](https://github.com/github/cmark-gfm) at tag `0.29.0.gfm.13`
(commit `587a12bb54d95ac37241377e6ddc93ea0e45439b`). It tests tables,
strikethrough, autolinks, the HTML tag filter, footnotes and task lists.

Its front matter names Yuki Izumi as the author and gives the license as
[CC BY-SA 4.0](https://creativecommons.org/licenses/by-sa/4.0/).

To fetch the copy again, run this from the repository root:

```sh
curl -fsSL -o internal/markdown/testdata/cmark-gfm-extensions/extensions.txt \
  https://raw.githubusercontent.com/github/cmark-gfm/0.29.0.gfm.13/test/extensions.txt
```
