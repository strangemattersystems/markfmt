# Spec corpus

These files are copies from goldmark v2.0.2:

- `spec.json`: the examples from the [CommonMark Spec 0.31.2](https://spec.commonmark.org/0.31.2/), licensed under [CC BY-SA 4.0](https://creativecommons.org/licenses/by-sa/4.0/). goldmark extracted them into JSON.
- `extra.txt`: goldmark's extra CommonMark cases.
- `linkify.txt`, `strikethrough.txt`, `table.txt`, `tasklist.txt`: goldmark's GFM extension cases.

goldmark is licensed under the MIT License. See `LICENSE.goldmark`.

To refresh the copies after a goldmark upgrade, run this from the repository root:

```sh
d=$(go list -m -f '{{.Dir}}' github.com/yuin/goldmark/v2)
cp "$d"/_test/{spec.json,extra.txt} "$d"/extension/_test/{linkify,strikethrough,table,tasklist}.txt internal/format/testdata/spec/
cp "$d"/LICENSE internal/format/testdata/spec/LICENSE.goldmark
chmod 644 internal/format/testdata/spec/*
```
