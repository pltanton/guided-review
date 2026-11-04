# Annotations and explanations (`annotations` in a `gr plan set` part)

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
tone, in their language, and give it this shape — at most three short lines, each on its
own line, no paragraph:

1. **The problem**, about a dozen words, naming the identifier in backticks.
2. **Why it matters**: the concrete scenario or consequence. Skip it for a nit.
3. **The ask**: what to do, or the question to answer. A `--suggestion` replaces it when
   the fix is obvious.

Around forty words in all. No filler ("I think", "maybe consider"), no line numbers (the
comment sits on the line), no praise, nothing the human did not say. The longer reasoning
stays in your note's `detail`, not in the comment.

Bad (a wall):
`The nil check in Hndl can never fire because every handler is declared as a literal in the
same package, so a nil could only come from a typo that the compiler already catches, which
means it adds a branch and a test for nothing and we should drop both.`

Good:
```
`Hndl` checks for a nil handler that can never be nil.
Handlers are literals in this package; a typo would not compile.
Drop the check and its test.
```

Good nit: `` `cnt` → `count`: the rest of the file spells it out. ``

# Never

Never: praise, restating the code line by line, generic advice, lists of "things to
consider", a flag without a concrete question.
