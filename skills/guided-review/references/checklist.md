# What to look for while planning

You read every step's code before the human does. Find the problems now and put them
where the human will see them: a hotspot for a question they must settle, an annotation
for what they should understand, a `detail` behind each for the full story.

Read each change in context: the whole function, its callers when behaviour changes, the
callee when a call does something non-obvious. Verify by reading instead of guessing.

## In this order

1. **Functionality.** Does it do what the task and spec say, for every input: empty,
   zero, negative, maximum, duplicate, out of order, concurrent, retried, partially
   failed, timed out? Do error paths return, roll back and leave state consistent?
2. **Design.** Is it in the right place and layer, in the repo's existing patterns? Does
   it duplicate a helper that exists? Is it built for an imagined future instead of the
   problem at hand?
3. **Complexity.** Can a reader understand it quickly? Deep nesting, long functions,
   clever code, hidden coupling, surprising side effects.
4. **Tests.** Do they cover the new behaviour and its edge and failure cases? Would they
   fail if the code broke — do they assert outcomes rather than calls? Anything flaky:
   time, ordering, randomness, sleeps, shared state?
5. **Security.** Input validated at trust boundaries; authorisation on every entry point
   (no object reachable by id alone); injection (SQL, shell, template, path); secrets or
   personal data in logs and errors; crypto and randomness misuse; SSRF; unsafe
   deserialisation.
6. **Data and concurrency.** Transaction boundaries, idempotency, races and lost updates,
   event and outbox ordering; migrations that lock large tables or break the running
   version; unbounded or N+1 queries, missing indexes, pagination.
7. **Operability and observability.** Errors carry context; failures are logged or counted;
   a new path or failure mode gets a log line, metric or trace span someone can alert on,
   with ids to correlate it and no personal data; timeouts on external calls; resources
   closed and contexts cancelled; config defaults and feature flags that make sense.
8. **Naming, comments, consistency.** Names say what things are, comments say why, the
   code reads like its neighbours. These are nits: never let them crowd out 1–7, and skip
   what a formatter or linter already enforces.

## Findings

- Every finding points at code and a concrete scenario: which input or sequence of
  events, and what goes wrong. Without a scenario it is a question, not a finding.
- When you cannot settle it by reading, make it a hotspot question; when you can, say it
  in an annotation. Do not assert what you have not checked.
- One finding per real problem; group repeats of the same issue.
- Size the severity honestly (blocker changes the approach, major is local rework, minor,
  nit); do not inflate to get attention.

## Pace

People find most defects in 200–400 lines per sitting, reading slower than about 500 lines
an hour; past that the defect rate found drops sharply. That is why steps stay under 300
changed lines, and why the step message should say where to look.

The rules behind each item, distilled from the sources, are in `sources.md`; read the
section for an item when a step touches it.
