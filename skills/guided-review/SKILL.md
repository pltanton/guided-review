---
name: guided-review
description: Use when the user wants to review a merge request, branch or change set step by step — "проведи по ревью", "давай поревьюим MR", "guided review", "review this MR with me", a GitLab MR URL together with a request to review it. Plans the route through the change, walks the reviewer through small ordered pieces with the gr CLI and a tmux viewer pane, records their comments.
---

# Guided review

You lead a human through a code review. They judge the code. You plan the route,
explain each piece in a few lines, catch spec mismatches and risky spots, and record
their remarks.

The viewer pane (`gr view`) is the main interface: the human sees the plan, the code
with your annotations, your messages, and types replies there. You talk to them with
`gr say` and listen with `gr wait`. Keep terminal chat output to a line or two per
turn — they are not looking at it.

`gr` holds all review state: when unsure where you are, run `gr step show` or
`gr status` instead of relying on memory. Answer in the user's language.

## Setup — no questions

1. `gr init <what the user gave>` — pass the MR URL verbatim when they gave one; no
   argument only when they gave nothing (current branch against the default
   branch). Never ask about branches: gr checks the MR out into its own worktree
   when HEAD is elsewhere and prints `code: <path>`. Read code under that path.
   - "commit … not found locally": run `git fetch origin` and retry.
   - "already exists, resuming": `gr step show`, then continue the step loop.
   - "round N": this is a re-review, see below.
   - Never pass `--force`: it throws away the plan and progress. Only the user may
     ask to start over.
2. If `$TMUX` is set and no window is named `review`
   (`tmux list-windows -F '#{window_name}'`), open the viewer full screen in its own
   window: `tmux new-window -n review gr view`. It takes focus; tell the user in one
   chat line that `prefix l` (or `a` in the viewer) brings them back to you.

Before anything that takes more than a few seconds — reading the diff, building the
plan, reading a step's code, answering an explain — run
`gr progress "<what you are doing>"` (e.g. `строю план: читаю diff, 58 файлов`). The
viewer shows it with a spinner and a timer; without it the human stares at a frozen
screen. `gr say` and `gr plan set` clear it.

## Intake — fast and short

Use only what is cheap: the MR description (`glab mr view <iid>`), a linked spec,
the unresolved MR discussions (`gr discussions` prints them in full), and
`git diff --stat <base> <head>`. Do not read code yet.

`gr say` at most three lines: the task in one line, how the change solves it in one
line, then «верно понял?». Everything else you noticed — failing checks, open
discussions, spec mismatches, stale examples in the description — is not intake:
keep it for the step it belongs to and put it there as a `spec` annotation or the
step's question. Then `gr wait` (below). Read code only after they confirm.

`glab` is read-only for you: never create notes, discussions or approvals with it.

## Plan

1. Read the diff (`git diff <base> <head>` in the code path) and `gr hunks`.
   Generated files are already excluded; decide which remaining files are
   boilerplate.
2. Order and size steps per references/ordering.md — the MR's own spec or design doc,
   if the diff has one, is always s1; mark hotspots per
   references/hotspots.md, with `line` so the viewer marks them.
3. Add `annotations` where one line saves the reader real effort: what a non-obvious
   call does, where the spec disagrees (`kind: spec`). One to three per step; none
   is fine.
4. Pipe the plan to `gr plan set`. If gr rejects it, fix exactly what it lists.
5. `gr say` the plan in one line per step (`s2 Handler ⚑`) plus a line for
   boilerplate and generated files, then start with s1.

```yaml
summary: "task → how it is solved"
boilerplate: [internal/di/wire.go]
steps:
  - id: s1
    title: "POST /transfers — contract"
    kind: contract # contract|model|entry|logic|persistence|migration|test|config
    hunks:
      - { file: api/openapi.yaml, lines: 120-168 } # omit lines for the whole file
  - id: s2
    title: "Handler"
    kind: entry
    hunks: [{ file: api/transfer.go, lines: 40-92 }]
    hotspots:
      - {
          cat: consistency,
          q: "Retry with the same key after a timeout — second debit?",
          line: 57,
        }
    annotations:
      - {
          file: api/transfer.go,
          line: 61,
          kind: note,
          text: "Put runs in the reserve transaction",
        }
    depends_on: [s1]
```

## Step loop

1. Read the step's hunks with enough surrounding code to be sure of what they do.
2. `gr say` the step message (references/style.md). Add annotations with
   `gr note add --file F --line N [--kind spec] TEXT` if you find something worth a
   line only now.
3. `gr wait` — run it with the Bash tool timeout at 600000 ms. "no input yet" means
   nothing happened: run it again. Each printed line is one event from the viewer:

   | Event                           | What to do                                                                                                                                                                                                                                                                                                     |
   | ------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
   | `[message] sN file:lines: text` | A remark → pick severity (blocker changes the approach, major is local rework, minor, nit), `gr comment add --file F --lines L --severity S [--suggestion TEXT] BODY`, then `gr say` one line: `записал: nit, transfer.go:57–58`. A question → `gr say` a short answer. Unsure which → ask one short question. |
   | `[message] sN: text`            | Same, without a line anchor; ask for the lines if a remark needs them.                                                                                                                                                                                                                                         |
   | `[explain] sN file:lines`       | Read that code, `gr note add --file F --line <first line> TEXT` with one to three lines on its role in the feature, not its syntax.                                                                                                                                                                            |
   | `[next] sN`                     | If a hotspot question on this step is unanswered, `gr say` it once more and wait; otherwise `gr step next` and go to 1.                                                                                                                                                                                        |
   | `[skip] sN: reason`             | `gr step skip --reason "<reason>"`, go to 1.                                                                                                                                                                                                                                                                   |
   | `[goto] sN`                     | `gr step goto sN`, go to 1.                                                                                                                                                                                                                                                                                    |

   `--suggestion` is the full replacement text for the lines; use it only for a nit
   or minor with an obvious fix.

4. After a blocker gr prints the stale steps. `gr say` once: «дальше смотрим
   независимые (N шагов) или завершаем?» and wait.
5. If the human writes in the terminal chat instead, handle it the same way, then
   return to `gr wait`.

## Re-review (`gr init` printed "round N")

1. The diff is only what changed since the last round (or, after a rebase, the files
   whose patch changed). Open comments from earlier rounds are listed.
2. First step, before any plan: check each open comment against the new code.
   `gr comment resolve ID` when it is addressed; otherwise keep it open. `gr say` a
   summary: resolved, still open, new questions.
3. Then plan and walk only the round diff as usual.

## MR discussions

`gr init`, `gr status` and `gr sync` list unresolved discussions of other reviewers
by their first line; `gr discussions` prints them in full. The viewer marks the ones
on lines. Mention them in intake, and on a step that touches
a discussed line, say whether the code answers it.

## Wrap-up

1. `gr status --gate`. If it fails, `gr say` the pending steps and ask: review them or
   skip each with a reason.
2. `gr say` the verdict in one line: approve / changes requested / blocked. Then
   blockers and majors, one line each, the nit count, and the coverage line from
   `gr status`.
3. Publishing to GitLab is not available yet: print `gr comment list` in the chat as a
   ready-to-paste summary.
4. When they confirm the review is over, `gr done` removes the worktree; the state
   stays for a re-review.
