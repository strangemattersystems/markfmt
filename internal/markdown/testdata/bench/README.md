# Benchmark inputs

Frozen copies of real documents, so a benchmark result compares two revisions
of the code and not two revisions of its input.

| File        | Source                                    | Bytes   |
| ----------- | ----------------------------------------- | ------- |
| `design.md` | `docs/design/parser.md` at commit 7a3d575 | 101,365 |
| `small.md`  | `README.md` at commit 7a3d575             | 922     |

Both files are part of this repository, under its licence. Update one only
when it no longer looks like the documents markfmt formats, and say so in the
commit message: every benchmark number before that commit compares against a
different input.
