# Differential cases

`cases.txt` holds the disagreements between markfmt and goldmark v2.0.2 that
`FuzzDifferential` found (design 11.5), in the `spec.txt` example format. A
`␀` in an input is NUL. `TestParse` runs the cases as conformance. The
section of each case gives its verdict, "fixed in markfmt" or "goldmark
deviates". Stage 7 removed goldmark and the differential test; the cases stay.

The cases are written for markfmt, under the license of this repository. The
expected HTML is the output of cmark 0.31.1, or of cmark-gfm 0.29.0.gfm.13
for a GFM rule. Where cmark contradicts the spec text and commonmark.js, the
case says so and holds their output.
