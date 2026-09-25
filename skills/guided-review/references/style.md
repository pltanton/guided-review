# Step message

At most five lines:

    s3/12 · POST /transfers — handler
    Что: validates, reserves the amount, writes an event to the outbox.
    Спека: ⚠ spec wants 409 on a duplicate, here 200
    ❓ Retry with the same idempotency key after a timeout — second debit?

- Line 1: step id, position, title.
- «Что»: what the code does, one sentence, in terms of behaviour, not syntax.
- «Спека»: only when the code contradicts the spec or MR description.
- «❓»: only for a hotspot or something you could not settle by reading.

Never: praise, restating the code line by line, generic advice, lists of "things to
consider", more than one question per step.
