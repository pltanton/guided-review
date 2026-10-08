---
name: guided-review
description: Use when the user wants to review a merge request, branch or change set step by step — "проведи по ревью", "давай поревьюим MR", "guided review", "review this MR with me", "review this PR", a GitLab MR or GitHub PR URL together with a request to review it. Plans the route through the change, walks the reviewer through small ordered pieces with the gr CLI and a tmux viewer pane, records their comments.
---

# Guided review

You lead a human through a code review. They judge the code. You plan the route,
explain each piece in a few lines, catch spec mismatches and risky spots, and record
their remarks.

Speak the human's language: the one they write to you in (in self mode, the one the
prompt names). Every quoted phrase in this skill is an English example — translate it.

The viewer pane (`gr view`) is the main interface: the human sees the plan, the code
with your annotations, your messages, and types replies there. You talk to them with
`gr say` and listen with `gr wait`. Keep terminal chat output to a line or two per
turn: the human glances at it, so it has to stay readable at a glance.

When you ask a question with a few likely answers, offer them:
`gr say --option "yes" --option "no, let me fix" "…"`. They become buttons in the viewer
(the human can still type anything). Two to four options, a few words each, in the
human's language; the chosen text arrives as an ordinary `[message]`.

Never end your turn while the viewer is open: every question goes through `gr say` and
then `gr wait`. If you end it anyway, the viewer shows "agent stopped" and their replies
sit unread. Once the human finishes in the viewer (`[finished]`, see Wrap-up) it closes,
and the rest happens in the terminal chat.

`gr` holds all review state: when unsure where you are, run `gr step show` or
`gr status` instead of relying on memory.

## Setup — no questions

Before the first `gr` call run `type gr`. If it is an alias or a function (oh-my-zsh's git
plugin defines `gr` as `git remote`), write `command gr` everywhere this skill says `gr`.

1. `gr init <what the user gave>` — pass the MR or PR URL verbatim when they gave one; no
   argument only when they gave nothing (current branch against the default
   branch). Never ask about branches: gr checks the MR out into its own worktree
   when HEAD is elsewhere and prints `code: <path>`. Read code under that path.
   - "commit … not found locally": run the fetch commands it names and retry.
   - "glab is not available" / "gh is not available … Reviewing !N from git": go on, it
     is a full review of the MR's code. Tell the human in one line that without glab/gh
     there are no MR discussions and the result is a local review.md to post by hand, and
     that installing glab/gh and logging in gives the full flow — do not install it unasked.
   - "already exists, resuming": `gr step show`, then continue the step loop. If it also
     printed "plan outdated", build the plan again from scratch (Plan below) — the old
     one predates chapters and step messages and nothing has been reviewed yet.
   - "round N": this is a re-review, see below.
   - Never pass `--force` on your own: it throws away the plan and progress. Only the
     user may ask to start over.
   - "state of review … is unreadable": show the human the error as gr printed it (here
     in the terminal, the viewer is not open yet) and offer «start over (--force)» or
     «I'll sort it out». Run `gr init --force` only after they chose it.
2. `gr open` — opens the viewer full screen in a tmux window `review-<id>` next to you,
   restarting it if it is already there. It finds your tmux pane by itself, also when
   `$TMUX` is not set in your shell. If it prints "not in tmux", give the human the
   command it printed to run the viewer in another terminal and go on.
   `A` in the viewer brings the user back to you, and closing it (or `gr done`) returns
   them to your pane automatically.

Before anything that takes more than a few seconds — reading the diff, building the
plan, reading a step's code, answering an explain — run
`gr progress "<what you are doing>"` (e.g. `planning: reading the diff, 58 files`). The
viewer shows it with a spinner and a timer; without it the human stares at a frozen
screen. `gr say` and `gr plan set` clear it.

## Intake — fast and short

Use only what is cheap: the description (`glab mr view <iid>` for GitLab,
`gh pr view <n> --repo <owner/repo>` for GitHub), a linked spec,
the unresolved MR discussions (`gr discussions` prints them in full), and
`git diff --stat <base> <head>`. Do not read code yet.

`gr say` at most three lines: the task in one line, how the change solves it in one
line, then «got it right?» with options such as «yes» and «no, let me fix». When `gr init`
listed generated files, add one line naming them — the ⚠ ones first, they are generated
only by a comment in the file — and ask «ok to skip them?». Everything else you noticed — failing checks, open
discussions, spec mismatches, stale examples in the description — is not intake:
keep it for the step it belongs to and put it there as a `spec` annotation or the
step's question. Then `gr wait` (below). Read code only after they confirm.

`glab` and `gh` are read-only for you until the wrap-up: the only writes to the MR are the ones in
"Publish" below, after the human said yes.

## Plan

The human waits in the viewer while you plan, so give them the route first and write the
explanations while they read.

1. Read the diff (`git diff <base> <head>` in the code path) and `gr hunks`.
   Generated files are excluded; one the human wants to see goes into a step like any
   other file. Decide which remaining files are boilerplate.
2. Group into chapters and steps per references/ordering.md: intent first, one chapter
   per behaviour, mechanics last; step titles are the claims to check. Pipe this route —
   `summary`, `boilerplate`, and per step `id`, `title`, `kind`, `chapter`, `hunks`,
   `depends_on`, `why_big` — to `gr plan set --route`. If gr rejects it, fix exactly what
   it lists. The viewer shows the steps at once; `gr say` the plan: one line per chapter
   with its steps (`Transfers: s1 s2`) plus a line for boilerplate and generated files.
3. Fill s1 now: read its code with enough surrounding code to be sure of what it does,
   look for problems per references/checklist.md, write its explanations (below) and pipe
   them to `gr plan fill`. It posts s1's message.
4. Fill the rest in reading order. With subagents (Claude Code's Agent tool) start them
   in the background, one per chapter, as in "Parallel filling" below, and go to the step
   loop while they work. Without subagents fill one step at a time with `gr plan fill`,
   and between steps run `gr wait --timeout 1s` to answer the human if they wrote;
   once all are filled, go to the step loop.

Explanations of a step, all in one `gr plan fill` entry with its `id`:
- `intro` on the first step of each chapter: two to four lines on how the behaviour was
  before and how it is after, and, when it helps, the path the request takes
  (`Handler → reserve() → ledger.Put → outbox`).
- `message` (references/style.md): at most three lines — what the code does, a spec
  mismatch, the one question.
- `hotspots` per references/hotspots.md, with `line` so the viewer marks them (and `file`
  when the step has several files).
- `annotations` where an explanation saves the reader real effort: what a non-obvious
  call does and why, where the spec disagrees (`kind: spec`). Full sentences, not
  fragments. One to three per step; none is fine.
- A `detail` on every annotation and hotspot in three short paragraphs, one or two
  sentences each — `**Problem.** …`, `**When it bites.** …` (the scenario),
  `**What to do.** …` — plus a short code excerpt in a fenced block when it helps. Name
  other code as `path:line` (`UserIdentityService.kt:88`), never by bare name: the viewer
  shows a snippet of each under the detail and opens it on 1–9. `i` in the viewer shows
  it at once instead of asking you again.

A step the human opens before it is filled shows "agent is writing notes"; its notes
appear when its fill lands. `gr plan set` without `--route` still takes a whole plan with
explanations at once.

The full shape, route and explanations together:

```yaml
summary: "task → how it is solved"
boilerplate: [internal/di/wire.go]
steps:
  - id: s1
    chapter: "Transfers between accounts"
    intro: |- # first step of a chapter only: before → after, and the path the request takes
      Before: a retried POST /transfers debited twice.
      After: the idempotency key returns the first result.
      Handler → reserve() → ledger.Put → outbox
    kind: contract # contract|model|entry|logic|persistence|migration|test|config
    hunks:
      - { file: api/openapi.yaml, lines: 120-168 } # omit lines for the whole file
  - id: s2
    chapter: "Transfers between accounts"
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

## Parallel filling

1. Write each chapter's route steps — `id`, `title`, `kind`, `chapter`, `hunks` — to
   `/tmp/gr-plan/<id>/chapter-N.yaml` as a top-level `steps:` list (leave out s1, already
   filled). These are scratch files for you and the subagents.
2. In one message start one general-purpose subagent per chapter in the background, so
   they run at once. Give each: the code path, base and head, the task in two lines, its
   chapter file, and the path of this skill's `references/` directory. Its job: read
   `style.md`, `hotspots.md` and `checklist.md` there, read its steps' code, rewrite its
   chapter file with the same ids plus the explanations (see Plan), run
   `gr plan fill -f <its file>` from the code path, fix whatever gr rejects, and reply
   with one line when done.
3. Go to the step loop right away. When a subagent reports a failure, fill its steps
   yourself.

## Step loop

The viewer moves through the steps itself: `>`, skips and jumps run `gr step …` and
show the step's `message` from the plan, and before leaving a step it asks the human to
check off each open hotspot (`x`); `gr step show` marks the checked ones. You only hear about what needs you.

1. If something worth explaining turns up only now, add it with
   `gr note add --file F --lines N-M [--kind spec] TEXT` (`--line N` for one line) and
   its detail right away with `gr note detail --file F --line <last line> - <<'EOF' … EOF`.
2. `gr wait` — run it with the Bash tool timeout at 600000 ms. "no input yet" means
   nothing happened: run it again. If your turn was interrupted and the human's next
   prompt is "Continue the guided review: run gr wait.", the viewer did that with `ctrl+c`:
   just run `gr wait`, their message is waiting there. Each printed line is one event from
   the viewer:

   - `[message] sN file:lines: text` — a remark: pick severity (blocker changes the
     approach, major is local rework, minor, nit),
     `gr comment add --file F --lines L --severity S [--suggestion TEXT] BODY` (BODY per
     references/style.md: readable for an author who was not here), then
     `gr say` one line: `noted: nit, transfer.go:57–58`. A question: `gr say` a short
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
     the code again and write it in the same three short paragraphs — `**Problem.**`,
     `**When it bites.**`, `**What to do.**` — with a code excerpt when it helps, other code named
     as `path:line`. Save it with
     `gr note detail --file F --line N [--step sN] - <<'EOF' … EOF`; the viewer shows it
     in the popup that is already open. No `gr say` needed.
   - `[reviewed] sN` — the human went past the last step: go to Wrap-up.
   - `[next] sN` — only for a plan without messages: `gr step next`, read the new step,
     `gr say` its message.
   - `[comment] sN file:lines: comment #N …` — the human saved that comment themselves,
     word for word (the viewer's default for a comment; an ai comment reaches you as a
     message instead). Do not add, edit or rephrase it and do not reply; if the
     text lists stale steps, handle it as a blocker (step 4). `[comment] sN: comment #N
     updated` / `comment #N deleted` is the human editing or deleting their own comment:
     nothing to do.

   `--suggestion` is the full replacement text for the lines; use it only for a nit
   or minor with an obvious fix.

   Messages usually carry the line the cursor was on (`file:line`): read the remark
   against that line. `re #N` means a reply to comment #N — answer it, and if the reply
   changes the remark, `gr comment edit N [--severity S] TEXT`. `[edit] sN #N: text` is
   the human rewriting their comment: `gr comment edit N TEXT` (keep the severity unless
   the new text clearly changes it) and `gr say` one line: `updated #N`.

   Step ids starting with `~` (`~boilerplate`, `~generated`, `~all`) come from the
   viewer's views outside the plan: treat the event as a remark on that file and record
   comments with the default step.

   An event can carry an earlier step's id: the human is looking back at it in the
   viewer. Handle it for that step (`gr comment add --step sN`, `gr note add --step sN`)
   and do not move the current step; they return to it themselves.

3. After a blocker gr prints the stale steps. `gr say` once, with both answers as
   options: «continue with the independent ones (N steps) or finish?», and wait.
4. If the human writes in the terminal chat instead, handle it the same way, then
   return to `gr wait`.

## Re-review (`gr init` printed "round N")

1. The diff is only what changed since the last round (or, after a rebase or a merge
   of the target branch, the files whose patch changed). Open comments from earlier rounds are listed.
2. First step, before any plan: check each open comment against the new code.
   `gr comment resolve ID` when it is addressed; otherwise keep it open. `gr say` a
   summary: resolved, still open, new questions.
3. Then plan and walk only the round diff as usual.
4. "carried, not reviewed in an earlier round: r1-s7 …" lists steps the last round left
   pending (a `--partial` finish, or stale after a blocker). gr appends them after your
   plan with their ids, titles, messages and hotspots; `gr step show` marks them
   "carried". Do not plan them again, and do not reuse their ids. A file the round
   touched loses its line ranges and notes there and is shown whole, so read it fresh
   when the human reaches that step. With carried steps, files changed in the round show
   their whole patch against the base, as after a rebase. If the round diff itself is
   empty, `echo 'steps: []' | gr plan set` starts the carried steps.

## Your threads on the MR (`gr init` printed "your threads: …")

The reviewer's own open threads, with every reply, are in `gr thread list`. The
human decides each one; you prepare the decision. Before planning:

1. For every thread read the replies and the current code at that line. Then
   `gr thread assess ID --propose resolve|open [--reply TEXT] - <<'EOF' … EOF`: one or two
   sentences — fixed (where), the answer convinces (why), or it does not (what is still
   wrong). Propose `--reply` whenever the thread stays open, and for a resolve when the
   author asked something; keep it to the style of a review comment.
2. `gr say` one line per answered thread (`d1 a.go:5: not fixed, suggest keeping it open`)
   and ask them to press `R` to decide. Decisions are the human's: the viewer records
   them itself and there is no command for it.
3. `[message] … re thread ID: …` is a question about that thread: answer with `gr say`;
   if your view changed, assess it again.

`gr prepare` refuses while an answered thread has no decision from the viewer, and refuses
`approve` while any of your threads stays open. A decided resolve also resolves the matching
gr comment; undoing the decision (`u`) takes that back.

## Self mode (`gr init` printed "mode: self")

The author is reviewing their own branch before anyone else sees it, and you were started
by their coding agent — as a subagent or in its own window — with a fresh context. That is the point: judge the code as a
stranger would. Do not look for or ask about how it was written.

- The prompt that started you gives the review id, the task (ticket, spec, a line or
  two), the author's language and the pane of the author's agent. Open the viewer with
  `gr open --return <that pane>`, so finishing lands the human back in their coding session.
- `gr init --self` was already run for you; `gr init --self` again just resumes.
- Intake: the task comes from the prompt, not from an MR. There are no MR discussions.
- Wrap-up: steps 1–3 as usual (`gr prepare` without `--approve`). `P` writes
  `fixes.json` for the author's agent and closes the viewer. That is the end of your job:
  as a subagent, reply with the export dir as your final answer; in a window, just end
  your turn (the author's agent closes it). No publishing, no notifying.

`gr init` and `gr status` list unresolved discussions of other reviewers by their first
line; `gr discussions` refreshes them from the MR and prints them in full. The viewer
marks the ones on lines. Mention them in intake, and on a step that touches a discussed
line, say whether the code answers it.

## Wrap-up

1. `gr status --gate`. If it fails, `gr say` the pending steps and ask: review them,
   skip each with a reason, or send what is reviewed so far — then add `--partial` to
   `gr prepare` in step 3: the result lists the rest as not reviewed yet, and the next
   round brings those steps back.
2. `gr say` the verdict in one line — approve / changes requested / blocked — then
   blockers and majors, one line each, the nit count, and the coverage line.
3. Prepare the result. Write the decisions taken during the review and why (what was
   accepted as is, what was left for later, why steps were skipped) as a few bullet
   lines, then:
   `gr prepare --verdict approve|changes|blocked --decisions-file - <<'EOF' … EOF`
   (add `--approve` only if they asked to approve; it goes only with `--verdict approve`). The viewer now shows `finish · P`:
   `P` previews the result, `P` again writes it to the review's export dir
   (`gr export --dir` prints it), sends you `[finished] sN: <dir>` and closes the viewer,
   returning the human to your pane.
   `gr say` «done: P in the viewer» and `gr wait`. While the human reads the result they
   can message you from it — `re #N …` about one comment (`gr comment edit N`), or about
   the summary or decisions (run `gr prepare` again with the new text); the viewer
   refreshes the preview by itself. If they say «let's finish» in chat
   instead, run `gr export` yourself: it prints the same dir.
   From `[finished]` on the viewer is closed: talk in the terminal chat as usual — no
   `gr say` / `gr wait` — and ending your turn with a question is fine.
4. A local branch without an MR (`gr init` without a URL, not self mode): there is
   nothing to publish. The dir holds `fixes.json` (`verdict`, `decisions`, `fixes` with
   `id`, `severity`, `file`, `lines`, `body`, `suggestion`) and `review.md`. Tell them the
   counts and ask «apply now / leave». Apply now: every fix was agreed during
   the review, `suggestion` is the exact replacement, lines refer to the reviewed commit
   (find the spot by content if the file changed); report one line per fix and run the
   tests. Leave: give the `fixes.json` path; `gr list` prints it (and `gr status` while
   the review is open), so any agent in this repo can pick it up later. Then step 7.
5. Publish (`[finished] … <dir>`). The dir holds `review.md` (what will be posted) and
   `review.json` (`provider`, `host`, `api`, `url`, `verdict`, `approve`, …). Tell them what
   goes out — N comments, thread replies and resolves, the summary, the verdict, approve or
   not — and ask «publish?». Only on a clear yes, run the script for the provider from
   this skill's `scripts/` directory, in the repository:
   `bash <this skill's dir>/scripts/publish-gitlab.sh` or `…/publish-github.sh`.
   It posts the review (GitLab: draft notes, then one bulk publish, then approve; GitHub:
   one review request with the summary, the verdict and every comment), then replies to and
   resolves your threads, logs each write that went through to `<dir>/published.jsonl`, and
   ends with `gr mark-published`, which marks exactly what is logged and prints
   `not published: …` for the rest.
   If it stops, say the error and what went out. Once the cause is fixed (a login, the
   network), run `gr export` and the script again: the new export holds only what did not
   go out, and on GitLab the script reuses drafts that already sit on the MR instead of
   posting them twice. If the MR holds draft notes that are not from this export, the
   script stops before publishing, because bulk publish would post them too: ask the
   reviewer what to do with them. Never write `published.jsonl` or run `gr mark-published`
   by hand. GitHub refuses APPROVE and REQUEST_CHANGES on your own PR: then set
   `"event": "COMMENT"` in `review-request.json` and run the script again without
   `gr export`. After success give the link.
6. Tell the author. If a `guided-review-notify` skill is available, follow it with the
   MR link, the verdict and the counts of what was actually published — that is where a
   team keeps its own way of pinging people. Without one, print a one-line message the
   human can forward (`reviewed !69: changes requested — 1 blocker, 2 major, 3 nit`).
7. Ask «close the review?». On yes, `gr done` (removes the worktree, keeps the state for a
   re-review).
