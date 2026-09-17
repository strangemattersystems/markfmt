# markfmt

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
