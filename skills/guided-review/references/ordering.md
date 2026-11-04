# Reading order

The reviewer sees the whole change as one diff, in the order you give the files. Order
so that the change reads like a short story: what it is for, each thing it does, then
the rest. `order` is a number per file, 1 first; `why` says in about ten words what the
file's change is for, so the tree reads as a table of contents.

In this order:

1. **Intent** — the MR's own spec, design doc, ADR or acceptance criteria
   (`docs/**/specs/*`, design notes, an acceptance-criteria file and the like). Always
   first: the reviewer judges the code against it. In a re-review it comes first when
   it changed.
2. **Preparatory refactoring**, when the MR first moves or reshapes code and then
   changes behaviour. Its files together, with a `why` that says behaviour must not
   change; they read fast and keep the rest clean.
3. **One behaviour at a time** — a request the MR now handles, a rule it now enforces, a
   failure it now survives. Inside, follow the data: contract (API spec, proto,
   migration, public interface) → entry point (handler, consumer, job) → logic → storage
   → the tests that pin it down. A contract or model shared by several behaviours goes
   with the first behaviour that needs it. Riskiest behaviour first.
4. **Leftovers** reachable from no behaviour — config, utilities, infra.
5. **Mechanics** last: renames, moves, import churn, formatting. Still read, but fast;
   say so in the `why`. The viewer marks moved code `↕` and format-only lines `≈`.

gr puts tests, CI and repository plumbing (`tests`, `other`) and generated files into
their own groups by path. Boilerplate (DI wiring, DTO mappers, route registration,
trivial config) is yours to name: `tier: boilerplate` on the file. Every group sits
folded at the end of the tree, one keypress away; `tier: core` brings a file back.

Within a file the reviewer reads top to bottom: there is no order inside a file. A file
that serves two behaviours goes with the first; say so in its `why`.
