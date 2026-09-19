# markfmt

## Product rules

1. markfmt reads Markdown and writes Markdown in one canonical style. It has
   no options.
2. It canonicalizes syntax and preserves content: prose line breaks, code, raw
   HTML, front matter and link destinations.
3. Formatting is idempotent: `format(format(x)) == format(x)`.
4. Formatting never changes the meaning of a document. A top-level block whose
   output markfmt cannot show to keep its meaning prints as written.
5. Output has LF line endings. Input CRLF, CR and LF are all line endings.
6. The root `go.mod` has no requirements.

## Working rules

- Use Conventional Commits.
- Run `task ci` before a commit. Use `task lint`, not a `golangci-lint` on
  `PATH`.
- Fix root causes. For each bug, add a failing case to the component that
  owns it (parser, printer or comparison), and fix it where the rule lives. Do
  not normalize output to hide a difference.

## Documentation and comments

Write Go doc comments in the standard library voice.

- Start the comment with the identifier: `// Format rewrites ...`.
- Be concise and stay on topic. Say what the identifier does, not how you
  wrote it.
- Use doc links in place of a plain name: `[Format]`, `[io.Writer]`.
- Leave out filler: "This function", "simply", "basically", "Note that",
  "helper to".

Keep a comment only when it carries value:

- a constraint that is not obvious from the code;
- a decision that a reader can undo by mistake;
- the reason that code which looks wrong is correct.

Delete every other comment. Do not rewrite a low-value comment to read better.
Do not restate the code or the identifier name.

The reason for a change goes in the commit message, not in the source.

At the end of a change, do a comment pass over the Go code you added or
changed. Apply the rules above to each doc comment and inline comment.
