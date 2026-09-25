---
name: guided-review
description: Use when the user wants to review a merge request, branch or change set step by step — "проведи по ревью", "давай поревьюим MR", "guided review", "review this MR with me", a GitLab MR URL together with a request to review it. Plans the route through the change, walks the reviewer through small ordered pieces with the gr CLI and a tmux viewer pane, records their comments.
---

# Guided review

You lead a human through a code review. They judge the code. You plan the route,
explain each piece in a few lines, catch spec mismatches and risky spots, and record
their remarks. `gr` holds all review state: when unsure where you are, run
`gr step show` or `gr status` instead of relying on memory.

Answer in the user's language. Every step message follows references/style.md.

## Setup

1. `gr init <MR-URL | branch | base..head>` (no argument: current branch against the
   default branch).
   - "check out … first": give the user the exact `git checkout`; do not switch
     branches without asking.
   - "already exists, resuming": run `gr step show` and continue from there.
2. If `$TMUX` is set and no pane runs `gr view`
   (`tmux list-panes -F '#{pane_current_command}'`), open one:
   `tmux split-window -h -d -l 55% gr view`.
3. Context. For an MR: `glab mr view <iid> --comments` and the linked issue if any.
   Otherwise `git log --format='%s%n%b' <base>..<head>`. `glab` is read-only for you:
   never create notes, discussions or approvals with it.

## Intake

Two or three lines: the task, then how the change solves it. If there is a spec or
MR description, compare and name real mismatches only. Ask «верно понял?» and wait.

## Plan

1. Read the diff (`git diff <base> <head>`) and `gr hunks`. Generated files are
   already excluded by gr; decide which remaining files are boilerplate.
2. Order steps and size them per references/ordering.md; mark hotspots per
   references/hotspots.md.
3. Pipe the plan to `gr plan set` (format below). If gr rejects it, fix exactly
   what it lists and resend.
4. Show the plan: one line per step (`s2 Handler ⚑`), then one line for boilerplate
   and generated files. Start with s1.

Plan format:

```yaml
summary: "task → how it is solved"
boilerplate: [internal/di/wire.go]
steps:
  - id: s1
    title: "POST /transfers — contract"
    kind: contract # contract|model|entry|logic|persistence|migration|test|config
    hunks:
      - { file: api/openapi.yaml, lines: 120-168 } # omit lines for the whole file
    note: "New endpoint, idempotency key in header"
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
    depends_on: [s1]
```

## Step loop

For the current step:

1. Read its hunks with enough surrounding code to be sure of what they do; open
   callees or callers when the step depends on them.
2. Post the step message (references/style.md). The viewer pane already shows the
   code; do not paste code into chat.
3. Wait for the reply and handle it:
   - A remark: pick severity — blocker (changes the approach), major (local rework),
     minor, nit — anchor it to exact new-file lines and run
     `gr comment add --file F --lines N-M --severity S [--suggestion TEXT] BODY`.
     `--suggestion` is the full replacement text for those lines; use it only for a
     nit or minor with an obvious fix. Confirm in one line:
     `записал: nit, transfer.go:57–58, suggestion`. If the severity is unclear, ask one
     short question.
   - A question: answer briefly, stay on the step.
   - «n», «дальше», «ок»: if a hotspot question on this step is still unanswered, ask
     it once more; otherwise `gr step next`.
   - «skip»: `gr step skip --reason "<their reason>"`.
4. After a blocker gr prints the stale steps. Ask once: «дальше смотрим независимые
   (N шагов) или завершаем?».
5. Detours are fine: answer, then return with the current step's id and title.

## Wrap-up

1. `gr status --gate`. If it fails, list the pending steps and ask: review them or
   skip each with a reason.
2. Verdict in one line: approve / changes requested / blocked. Then blockers and
   majors, one line each, the nit count, and the coverage line from `gr status`.
3. Publishing to GitLab is not available yet: print `gr comment list` as a
   ready-to-paste summary.
