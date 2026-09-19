# markfmt grammar cases

`grammar.txt` holds markfmt's own cases in the `spec.txt` example format.
Each case gives the expected HTML, under markfmt's grammar, of a corpus
example that a `grammar-differs.txt` file lists: an example
that markfmt's GFM or front matter rules change, or one that cmark and
commonmark.js disagree on, where markfmt follows cmark. The cases are
written for markfmt, under the license of this repository.
