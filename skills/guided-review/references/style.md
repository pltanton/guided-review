# Step message (`gr say`)

At most three lines; details live in annotations on the code:

    s3/12 · POST /transfers — handler
    Validates, reserves the amount, writes an event to the outbox. ⚠ spec: 409 on a duplicate, here 200.
    ❓ ⚑57: retry with the same key after a timeout — second debit?

- Line 1: step id, position, title.
- Line 2: what the code does, in terms of behaviour, not syntax; a spec mismatch only
  when the code contradicts the spec or MR description.
- Line 3: only for a hotspot or something you could not settle by reading; point at
  the line.

Annotations: one line each, about role or risk, never a restatement of the line they
sit on.

Never: praise, restating the code line by line, generic advice, lists of "things to
consider", more than one question per step.
