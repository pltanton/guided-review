# Hotspots

A hotspot is a place where a mistake is expensive. Mark it with a category, a concrete
question, the file and the line (0 for the whole file).

| cat         | look for                                                                                     |
| ----------- | -------------------------------------------------------------------------------------------- |
| security    | authn/authz checks, input validation, secrets or PII in logs, injections, SSRF               |
| consistency | transaction boundaries, races and double spend, idempotency, outbox and event order, retries |
| money       | rounding, float vs decimal, currency and scale, sign of amounts, limits                      |
| migration   | locks on large tables, backfill, backward compatibility with the running version             |

With `domain: finance` in `.review.yaml` the bar is higher: every change that moves,
reserves or displays money gets a `money` hotspot.

Ask, do not assert. The question must be answerable by reading the code:

- good: «Retry with the same idempotency key after a timeout — a second charge?»
- good: «Amount is a float64 until it hits the DB — where is it rounded?»
- bad: «There may be a race here» (no scenario)
- bad: «Check the security» (no question)

A hotspot is never boilerplate. Only the human checks it off, or a comment of theirs on
its line does.
