# HTML named character references

`entities.json` is an unchanged copy of the WHATWG HTML Standard's list of
named character references, fetched on 2026-09-13 from
<https://html.spec.whatwg.org/entities.json> (`Last-Modified: Wed, 12 Nov 2025
00:20:03 GMT`, SHA-256
`d741d877ac77c4194c4ad526b5b4a19aef8dfe411ab840a466891cdbb9f362e6`).
`gen_entities.go` generates the entity table in `entities.go` from its names
that end with `;`, and `entities_test.go` checks the table against it.

Copyright © WHATWG (Apple, Google, Mozilla, Microsoft). The file is licensed
under the [Creative Commons Attribution 4.0 International
License](https://creativecommons.org/licenses/by/4.0/).

To fetch the copy again, run this from the repository root:

```sh
curl -fsSL -o internal/markdown/testdata/entities/entities.json \
  https://html.spec.whatwg.org/entities.json
```
