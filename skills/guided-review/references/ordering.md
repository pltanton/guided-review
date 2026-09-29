# Route through a change

Outside in, then depth first along the changed code.

0. The MR's own spec — a design doc, ADR, or acceptance-criteria file changed in this
   MR (`docs/**/specs/*`, `*.md` design notes, `AGENTS-ACC.md` and the like). Always
   the first step: the reviewer judges the code against it. In a re-review it goes
   first too when it changed.
1. Contracts — what the outside world sees: API specs (OpenAPI, proto), DB schema
   migrations, public interfaces, message formats. Generator sources (.proto,
   openapi.yaml) are contracts even when their output is generated.
2. Domain model — its own step right after contracts when the change is about the
   model (new aggregate, changed invariants). Otherwise show a model change at its
   first use in step 3.
3. Entry points — handler, consumer, job, CLI command. Order them by hotspot density,
   riskiest first. From each entry point go depth first along the call chain, but
   only through changed code. Unchanged callees are context, not steps.
4. Tests — right after the unit they cover.
5. Leftovers not reachable from any entry point — config, utilities, infra —
   grouped into a few steps.
6. Boilerplate (DI wiring, DTO mappers, route registration, trivial config) goes into
   `boilerplate:`, not into steps.

Step size: one idea, ideally 40–150 changed lines, never more than 300 — past that a
reviewer stops taking it in, and `gr plan set` rejects the step. Split along meaning,
not line counts: per function or method, per layer (contract / handler / storage), per
concern (validation, persistence, events), tests apart from the code. Merge small hunks
that serve one idea. Only when a piece truly cannot be split (one migration, one
generated-like table) keep it whole and say why in the step's `why_big`.

`depends_on`: step B depends on A when a blocker in A would likely rewrite B.
Contracts are usually roots; handlers depend on the contract; logic depends on the
handler that calls it. Independent leftovers depend on nothing, so they stay
reviewable after a blocker.
