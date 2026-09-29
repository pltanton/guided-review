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

Never end your turn while the viewer is open: every question goes through `gr say` and
then `gr wait`. If you end it anyway, the viewer shows "agent stopped" and their replies
sit unread. Once the human finishes in the viewer (`[finished]`, see Wrap-up) it closes,
and the rest happens in the terminal chat.

`gr` holds all review state: when unsure where you are, run `gr step show` or
`gr status` instead of relying on memory. Answer in the user's language.

## Setup — no questions

1. `gr init <what the user gave>` — pass the MR URL verbatim when they gave one; no
   argument only when they gave nothing (current branch against the default
   branch). Never ask about branches: gr checks the MR out into its own worktree
   when HEAD is elsewhere and prints `code: <path>`. Read code under that path.
   - "commit … not found locally": run `git fetch origin` and retry.
   - "already exists, resuming": `gr step show`, then continue the step loop. If it also
     printed "plan outdated", build the plan again from scratch (Plan below) — the old
     one predates chapters and step messages and nothing has been reviewed yet.
   - "round N": this is a re-review, see below.
   - Never pass `--force`: it throws away the plan and progress. Only the user may
     ask to start over.
2. If `$TMUX` is set, open the viewer full screen in a window named after the review id
   gr printed (`review-<id>`, e.g. `review-mr-521`). If that window already exists it may
   run an old binary or show another review, so restart it instead of skipping:
   ```bash
   w=review-<id>
   if tmux list-windows -F '#{window_name}' | grep -qx "$w"; then
     tmux respawn-window -k -t "$w" -c "$PWD" "gr view --return $TMUX_PANE"; tmux select-window -t "$w"
   else
     tmux new-window -n "$w" -c "$PWD" "gr view --return $TMUX_PANE"
   fi
   ```
   `a` in the viewer brings the user back to you, and closing it (or `gr done`) returns
   them to your pane automatically.

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

`glab` is read-only for you until the wrap-up: the only writes to the MR are the ones in
"Publish" below, after the human said yes.

## Plan

1. Read the diff (`git diff <base> <head>` in the code path) and `gr hunks`.
   Generated files are already excluded; decide which remaining files are
   boilerplate.
2. Group into chapters and steps per references/ordering.md: intent first, one chapter
   per behaviour, mechanics last; step titles are the claims to check. Mark hotspots per
   references/hotspots.md, with `line` so the viewer marks them.
3. Read every step's code now, with enough surrounding code to be sure of what it does,
   and look for problems per references/checklist.md: the viewer moves between steps
   without you, so all per-step work happens here.
4. For each step write its `message` (references/style.md: at most three lines, what
   the code does, a spec mismatch, the one question) and its `annotations` where an
   explanation saves the reader real effort: what a non-obvious call does and why,
   where the spec disagrees (`kind: spec`). Full sentences, not fragments. One to three
   annotations per step; none is fine. Give every annotation and every hotspot a
   `detail`: three to eight sentences on what exactly the point or problem is, the
   scenario where it bites and what to do, with a short code excerpt in a fenced block
   when it helps. You have all of it in context now; `i` in the viewer shows it at once
   instead of asking you again.
5. Pipe the plan to `gr plan set`. If gr rejects it, fix exactly what it lists. It posts
   s1's message itself.
6. `gr say` the plan: one line per chapter with its steps (`Переводы: s1 s2 ⚑`) plus a line for
   boilerplate and generated files.

```yaml
summary: "task → how it is solved"
boilerplate: [internal/di/wire.go]
steps:
  - id: s1
    chapter: "Переводы между счетами"
    title: "POST /transfers accepts an idempotency key"
    kind: contract # contract|model|entry|logic|persistence|migration|test|config
    hunks:
      - { file: api/openapi.yaml, lines: 120-168 } # omit lines for the whole file
  - id: s2
    chapter: "Переводы между счетами"
    title: "A repeated key returns the first result"
    kind: entry
    message: |-
      s2/5 · POST /transfers — handler
      Validates, reserves the amount, writes an event to the outbox. ⚠ spec: 409 on a duplicate, here 200.
      ❓ ⚑57: retry with the same key after a timeout — second debit?
    hunks: [{ file: api/transfer.go, lines: 40-92 }]
    hotspots:
      - cat: consistency
        line: 57
        q: "Retry after a timeout — second debit?"
        detail: >-
          The key is checked in Redis before the transaction and written after it, so a
          retry that lands between the debit and the write passes the check and debits
          again. Checking and writing the key inside the reserve transaction closes it.
    annotations:
      - file: api/transfer.go
        line: 61
        to: 64 # the lines the note is about; they get its colour in the viewer
        kind: note
        text: >-
          reserve() opens the transaction and Put writes the ledger row inside it, so a
          failed debit rolls the reservation back too.
        detail: >-
          Put takes tx rather than the pool on purpose: called outside reserve() it would
          commit on its own and a failed debit would leave the reservation behind.
    depends_on: [s1]
```

## Step loop

The viewer moves through the steps itself: `>`, skips and jumps run `gr step …` and
show the step's `message` from the plan, and it reminds the human of an open hotspot
before leaving a step. You only hear about what needs you.

1. If something worth explaining turns up only now, add it with
   `gr note add --file F --lines N-M [--kind spec] TEXT` (`--line N` for one line) and
   its detail right away with `gr note detail --file F --line <last line> - <<'EOF' … EOF`.
2. `gr wait` — run it with the Bash tool timeout at 600000 ms. "no input yet" means
   nothing happened: run it again. Each printed line is one event from the viewer:

   - `[message] sN file:lines: text` — a remark: pick severity (blocker changes the
     approach, major is local rework, minor, nit),
     `gr comment add --file F --lines L --severity S [--suggestion TEXT] BODY` (BODY per
     references/style.md: readable for an author who was not here), then
     `gr say` one line: `записал: nit, transfer.go:57–58`. A question: `gr say` a short
     answer. Unsure which: ask one short question.
   - `[message] sN: text` — the same without a line anchor; ask for the lines if a
     remark needs them.
   - `[explain] sN file:lines` — read that code,
     `gr note add --file F --lines <those lines> TEXT`: two to four sentences on its role
     in the feature, not its syntax (references/style.md).
   - `[ask] sN file:lines: text` — a question about that code, never a remark: read it,
     answer with `gr say` in a few lines. Do not create comments or notes for it.
   - `[detail] sN file:line: <note text>` — a note without a `detail` (add one with every
     note from now on). Read
     the code again and write three to eight sentences: what exactly the problem or the
     point is, the scenario where it bites, and what to do about it; add a short code
     excerpt in a fenced block when it helps. Save it with
     `gr note detail --file F --line N [--step sN] - <<'EOF' … EOF`; the viewer shows it
     in the popup that is already open. No `gr say` needed.
   - `[reviewed] sN` — the human went past the last step: go to Wrap-up.
   - `[next] sN` — only for a plan without messages: `gr step next`, read the new step,
     `gr say` its message.
   - `[comment] sN file:lines: comment #N …` — the human saved that comment themselves,
     word for word (raw mode). Do not add, edit or rephrase it and do not reply; if the
     text lists stale steps, handle it as a blocker (step 4). `[comment] sN: comment #N
     updated` / `comment #N deleted` is the human editing or deleting their own comment:
     nothing to do.

   `--suggestion` is the full replacement text for the lines; use it only for a nit
   or minor with an obvious fix.

   Messages usually carry the line the cursor was on (`file:line`): read the remark
   against that line. `re #N` means a reply to comment #N — answer it, and if the reply
   changes the remark, `gr comment edit N [--severity S] TEXT`. `[edit] sN #N: text` is
   the human rewriting their comment: `gr comment edit N TEXT` (keep the severity unless
   the new text clearly changes it) and `gr say` one line: `обновил #N`.

   Step ids starting with `~` (`~boilerplate`, `~generated`, `~all`) come from the
   viewer's views outside the plan: treat the event as a remark on that file and record
   comments with the default step.

   An event can carry an earlier step's id: the human is looking back at it in the
   viewer. Handle it for that step (`gr comment add --step sN`, `gr note add --step sN`)
   and do not move the current step; they return to it themselves.

3. After a blocker gr prints the stale steps. `gr say` once: «дальше смотрим
   независимые (N шагов) или завершаем?» and wait.
4. If the human writes in the terminal chat instead, handle it the same way, then
   return to `gr wait`.

## Re-review (`gr init` printed "round N")

1. The diff is only what changed since the last round (or, after a rebase, the files
   whose patch changed). Open comments from earlier rounds are listed.
2. First step, before any plan: check each open comment against the new code.
   `gr comment resolve ID` when it is addressed; otherwise keep it open. `gr say` a
   summary: resolved, still open, new questions.
3. Then plan and walk only the round diff as usual.

## Self mode (`gr init` printed "mode: self")

The author is reviewing their own branch before anyone else sees it, and you were started
by their coding agent with a fresh context. That is the point: judge the code as a
stranger would. Do not look for or ask about how it was written.

- The prompt that started you gives the review id, the task (ticket, spec, a line or
  two) and the pane of the author's agent. Open the viewer with `--return <that pane>`
  instead of `$TMUX_PANE`, so finishing lands the human back in their coding session.
- `gr init --self` was already run for you; `gr init --self` again just resumes.
- Intake: the task comes from the prompt, not from an MR. There are no MR discussions.
- Wrap-up: steps 1–3 as usual (`gr prepare` without `--approve`). `P` writes
  `fixes.json` for the author's agent and closes the viewer. That is the end of your job:
  the author's agent closes your window and the review. No publishing, no notifying.



`gr init` and `gr status` list unresolved discussions of other reviewers by their first
line; `gr discussions` refreshes them from the MR and prints them in full. The viewer
marks the ones on lines. Mention them in intake, and on a step that touches a discussed
line, say whether the code answers it.

## Wrap-up

1. `gr status --gate`. If it fails, `gr say` the pending steps and ask: review them or
   skip each with a reason.
2. `gr say` the verdict in one line — approve / changes requested / blocked — then
   blockers and majors, one line each, the nit count, and the coverage line.
3. Prepare the result. Write the decisions taken during the review and why (what was
   accepted as is, what was left for later, why steps were skipped) as a few bullet
   lines, then:
   `gr prepare --verdict approve|changes|blocked --decisions-file - <<'EOF' … EOF`
   (add `--approve` only if they asked to approve). The viewer now shows `✓ finish · P`:
   `P` previews the result, `P` again writes it to `/tmp/guided-review/<id>/`, sends you
   `[finished] sN: <dir>` and closes the viewer, returning the human to your pane.
   `gr say` «готово: P во вьювере» and `gr wait`. If they say «заканчиваем» in chat
   instead, run `gr export` yourself: it prints the same dir.
   From `[finished]` on the viewer is closed: talk in the terminal chat as usual — no
   `gr say` / `gr wait` — and ending your turn with a question is fine.
4. Publish (`[finished] … <dir>`). The dir holds `review.md` (what will be posted),
   `review.json` (`host`, `api`, `url`, `verdict`, `approve`, `drafts`) and
   `drafts/NN.json`, each the exact body of one GitLab draft note, the summary last.
   Tell them what goes out — N comments, the summary, the verdict, approve or not — and
   ask «публикую?». Only on a clear yes:
   ```bash
   dir=/tmp/guided-review/<id>; host=$(jq -r .host $dir/review.json); api=$(jq -r .api $dir/review.json)
   glab api --hostname $host "$api/draft_notes" | jq length   # must be 0; otherwise ask first
   for f in $(jq -r '.drafts[]' $dir/review.json); do
     glab api --hostname $host -X POST "$api/draft_notes" -H 'Content-Type: application/json' --input "$dir/$f" || break
   done
   glab api --hostname $host -X POST "$api/draft_notes/bulk_publish"
   # only if review.json has "approve": true
   glab api --hostname $host -X POST "$api/approve"
   gr mark-published
   ```
   If a POST fails, stop: `gr say` the GitLab error and that the drafts created so far sit
   unpublished on the MR; do not retry blindly. After success give the MR link.
5. Tell the author. If a `guided-review-notify` skill is available, follow it with the
   MR link, the verdict and the counts of what was actually published — that is where a
   team keeps its own way of pinging people. Without one, print a one-line message the
   human can forward (`reviewed !69: changes requested — 1 blocker, 2 major, 3 nit`).
6. Ask «закрываем ревью?». On yes, `gr done` (removes the worktree, keeps the state for a
   re-review).
