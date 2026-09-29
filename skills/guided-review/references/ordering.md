# Route through a change

The reviewer has to see every changed line with their own eyes and still understand
what they are looking at. Group so that each piece answers one question and the whole
reads like a short story: what the change is for, each thing it does, then the rest.

## Chapters

Every step has a `chapter`. A chapter is one thing the MR changes, told end to end; two
to four chapters of two to four steps read better than nine steps in a row. Keep a
chapter's steps together (`gr plan set` rejects a split chapter). Name chapters in the
human's language, a few words each.

In this order:

1. **Intent** — the MR's own spec, design doc, ADR or acceptance criteria
   (`docs/**/specs/*`, design notes, `AGENTS-ACC.md` and the like). Always first: the
   reviewer judges the code against it. In a re-review it comes first when it changed.
2. **Preparatory refactoring**, when the MR first moves or reshapes code and then
   changes behaviour. Its own chapter, titled so it is clear that behaviour must not
   change; it reads fast and keeps the rest clean.
3. **One chapter per behaviour** the MR adds or changes — a request it now handles, a
   rule it now enforces, a failure it now survives. Inside, follow the data: contract
   (API spec, proto, migration, public interface) → entry point (handler, consumer, job)
   → logic → storage → the tests that pin it down. A contract or model shared by several
   behaviours goes into the first chapter that needs it. Order chapters by risk, riskiest
   first.
4. **Leftovers** reachable from no behaviour — config, utilities, infra — in one chapter.
5. **Mechanics** last: renames, moves, import churn, formatting. Still read, but fast;
   say so in the step message. The viewer marks moved code `↕` and format-only lines `≈`.

Boilerplate (DI wiring, DTO mappers, route registration, trivial config) goes into
`boilerplate:`, not into steps; generated files are excluded by gr. Both stay visible in
the viewer's extra views.

## Steps

- **Title is the claim the reviewer checks**, not a list of files: "Duplicate intent is
  rejected", not "Register handlers: canonical". The summary reads as a list of what was
  verified.
- **Size**: one idea, ideally 40–150 changed lines, never more than 300 — past that a
  reviewer stops taking it in, and `gr plan set` rejects the step. Split along meaning
  (per function, per layer, per concern), not by line count; merge small hunks that
  serve one idea. Only a piece that truly cannot be split (one migration, one
  generated-like table) stays whole, with the reason in `why_big`.
- **Tests** close their chapter as the proof. Put them first only when they read as the
  clearest statement of the behaviour.
- Depth first along the changed code only: unchanged callees are context, not steps.

## depends_on

Step B depends on A when a blocker in A would likely rewrite B. Contracts are usually
roots, handlers depend on the contract, logic on the handler that calls it. Independent
chapters depend on nothing, so they stay reviewable after a blocker.
