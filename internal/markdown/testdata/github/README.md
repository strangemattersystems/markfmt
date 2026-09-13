# GitHub fixtures

`github.txt` and `printer.txt` hold the HTML that the
[GitHub Markdown API](https://docs.github.com/en/rest/markdown/markdown) gives
for each input in `input/` (design 11.4). Both use the `spec.txt` example
format, with one section per input.

- `TestParse` runs `github.txt` as conformance: footnotes, task lists, tables,
  strikethrough, extended autolinks, and one fixture for each row of
  `../dialect.md`. The GitHub normalizer in `html_test.go` removes GitHub's
  decorations from both sides before the comparison.
- `printer.txt` holds math, alert and plain text fixtures (emoji, mentions,
  issue references) for the stage 6 printer. No test reads it yet.

The API renders with `mode=gfm`, which renders task lists. That mode also
writes `<br>` for each soft break, as GitHub renders comments, so the
normalizer does not tell soft breaks from hard breaks. `mode=markdown` renders
task list items as text. Without a repository context, `#1` is not a link.

## Rules for inputs

- Send only inputs written for these fixtures, never user documents.
- Each input ends with a line ending. No line is only `.` or a fence of 32
  backticks, because those lines end a part of an example.
- Write a tab as a tab, never as `→`, which the example reader reads as a tab.

## GitHub docs examples

The inputs whose names start with `docs-` are examples copied from
[github/docs](https://github.com/github/docs), which is licensed under
[CC BY 4.0](https://creativecommons.org/licenses/by/4.0/):

- `footnotes/docs-footnotes.md` and `alerts/docs-alerts.md`:
  `content/get-started/writing-on-github/getting-started-with-writing-and-formatting-on-github/basic-writing-and-formatting-syntax.md`
  at commit `6ed0ad8e50adda37a6b596bda8789788e28a0c0d`.
- `math/docs-*.md`:
  `content/get-started/writing-on-github/working-with-advanced-formatting/writing-mathematical-expressions.md`
  at commit `5e4bf35023f07b0183bfa3da08e0582324af9440`.

The output of the GitHub Markdown API is used under the
[GitHub Terms of Service](https://docs.github.com/en/site-policy/github-terms/github-terms-of-service).

## Capture

Capture the fixtures again before each release. Run this from this directory,
with `gh` logged in:

```sh
fence='````````````````````````````````'
for file in github printer; do
  {
    printf '# GitHub fixtures\n\nCaptured %s with `gh api markdown -f mode=gfm -F text=@INPUT`.\n' "$(date -u +%F)"
    for input in input/$file/*/*.md; do
      name=${input#input/$file/}
      printf '\n## %s\n\n%s example\n' "${name%.md}" "$fence"
      cat "$input"
      printf '.\n'
      html=$(gh api markdown -f mode=gfm -F text=@"$input")
      if [ -n "$html" ]; then printf '%s\n' "$html"; fi
      printf '%s\n' "$fence"
    done
  } > "$file.txt"
done
```
