# Differential cases

`cases.txt` holds the disagreements between markfmt and goldmark v2.0.2 that
`FuzzDifferential` found (design 11.5), in the `spec.txt` example format.
A `␀` in an input is NUL. `TestParse` runs them as conformance. `TestGoldmarkDiffers` checks the verdict
in the section of each case: goldmark still disagrees on a "goldmark deviates"
case, and `goldmarkDiffers` gives the section as its reason; goldmark agrees on
a "fixed in markfmt" case.

The cases are written for markfmt, under the license of this repository. The
expected HTML is the output of cmark 0.31.1, or of cmark-gfm 0.29.0.gfm.13
for a GFM rule.
