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

# Annotations and explanations (`gr note add`, plan `annotations`)

The human reads a note next to the code, in the middle of the review, without your
context. The viewer folds long notes (`o`), so write something that stands on its own:
two to four full sentences, not telegraphic fragments.

- Say what this code does in the feature, in domain terms, and why it is here.
- Name what it relies on or what relies on it: the function, the table, the other step.
- For a risk, give the concrete scenario: which input or sequence of events, and what
  goes wrong.
- Never restate the line it sits on.

Bad: `Put runs in the reserve transaction`
Good: `reserve() opens the transaction and Put writes the ledger row inside it, so a
failed debit rolls the reservation back too. That is why Put takes tx instead of the
pool: called outside reserve() it would commit on its own.`

# Review comments (`gr comment add` BODY)

This text goes to the MR author, who was not in the review. Keep the human's point and
tone, but make it readable on its own:

- What is wrong or unclear, pointing at the identifier, not "here".
- Why it matters: the scenario or consequence in one sentence.
- What to do, or the question to answer.

Two to five sentences; a nit can be one, but a full sentence. Write in the language the
human used. Do not add points they did not make.

Bad: `nil check noise`
Good: `The nil check in Hndl can never fire: every handler is a literal in the same
package, so a nil can only come from a typo that the compiler already catches. It adds a
branch and a test for nothing — let's drop both.`

# Never

Never: praise, restating the code line by line, generic advice, lists of "things to
consider", more than one question per step.
