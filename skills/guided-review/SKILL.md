---
name: guided-review
description: Use when the user wants to review a merge request, branch or change set with an agent at their side — "проведи по ревью", "давай поревьюим MR", "guided review", "review this MR with me", "review this PR", a GitLab MR or GitHub PR URL together with a request to review it. Opens the whole change in a tmux viewer pane, reads it in parallel with the reviewer, marks the risky lines and a reading order with the gr CLI, records their comments.
---

# Guided review

You review a change together with a human. They judge the code. You read it next to
them, suggest a reading order, flag where a mistake is expensive, explain the
non-obvious lines, catch spec mismatches, and record their remarks.

Speak the human's language: the one they write to you in (in self mode, the one the
prompt names). Every quoted phrase in this skill is an English example — translate it.

The viewer pane (`gr view`) is the main interface: the human sees the whole diff with a
file tree, your flags and notes on the lines, your messages, and types replies there.
You talk to them with `gr say` and listen with `gr wait`. Keep terminal chat output to a
line or two per turn: the human glances at it, so it has to stay readable at a glance.

When you ask a question with a few likely answers, offer them:
`gr say --option "yes" --option "no, let me fix" "…"`. They become buttons in the viewer
(the human can still type anything). Two to four options, a few words each, in the
human's language; the chosen text arrives as an ordinary `[message]`.

Never end your turn while the viewer is open: every question goes through `gr say` and
then `gr wait`. If you end it anyway, the viewer shows "agent stopped" and their replies
sit unread. Once the human finishes in the viewer (`[finished]`, see Wrap-up) it closes,
and the rest happens in the terminal chat.

`gr` holds all review state: when unsure where you are, run `gr status` instead of
relying on memory. It lists the files in reading order with what you have read (`·`
marks the rest), what the human has seen (`●` marks the rest) and the flags.

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
   - "already exists, resuming": `gr status`, then go on reading the files marked `·`.
   - "round N": this is a re-review, see below.
   - Never pass `--force` on your own: it throws away your notes and the progress. Only
     the user may ask to start over.
   - "state of review … is unreadable": show the human the error as gr printed it (here
     in the terminal, the viewer is not open yet) and offer «start over (--force)» or
     «I'll sort it out». Run `gr init --force` only after they chose it.
   If the human said where to announce the result — a chat thread link, a channel, a
   person — run `gr notify set <that>` now; without it the result goes to the MR author.
2. `gr open` at once, before you read anything — the viewer needs nothing from you: the
   human starts reading the diff while you work. It opens the viewer full screen in a
   tmux window `review-<id>` next to you, restarting it if it is already there. It finds
   your tmux pane by itself, also when `$TMUX` is not set in your shell. If it prints
   "not in tmux", give the human the command it printed to run the viewer in another
   terminal and go on. `A` in the viewer brings the user back to you, and closing it (or
   `gr done`) returns them to your pane automatically.

Before anything that takes more than a few seconds — reading the diff, reading a file,
answering an explain — run `gr progress "<what you are doing>"` (e.g. `reading
transfer.go, 3 of 12`). The viewer shows it with a spinner and a timer; without it the
human stares at a frozen screen. `gr say` and `gr plan set` clear it.

`glab` and `gh` are read-only for you until the wrap-up: the only writes to the MR are
the ones in "Publish" below, after the human said yes.

## Reading — in small writes, while the human reads

Everything you learn goes to the viewer through `gr plan set` as soon as it is ready,
in parts: a part is a YAML document with any of `briefing`, `files`, `hotspots`,
`annotations`. Parts merge: a file named again gets its new `why`, `order` and `tier`, a hotspot at the same file and line replaces the earlier one (checked stays
checked), an annotation at the same file, line and kind replaces the earlier one,
`briefing` replaces the earlier briefing. gr rejects a part with a path or a line that is
not in the diff; fix exactly what it lists and send it again.

Pick the depth from the stats `gr init` printed:

- **Tiny** (a few hunks in one or two files): read the diff once, send the hotspots if
  there are any, and `gr say` one line: «3 lines, nothing to flag» or «one flag,
  transfer.go:57». No briefing, no order, no `why`. Name every file in `files` anyway,
  so the tree shows you read it.
- **Normal**: steps 1–4 below.
- **Large** (over ~600 changed lines or more than ~15 core files): steps 1–2 yourself,
  then "Parallel reading" for step 3.

The human can change the depth in words at any time («go deep on the migration», «skip
the reading, I'll ask»): do what they said.

1. Intake, cheap only: the description (`glab mr view <iid>` for GitLab,
   `gh pr view <n> --repo <owner/repo>` for GitHub), a linked spec, the unresolved MR
   discussions (`gr discussions` prints them in full), `git diff --stat <base> <head>`.
   Do not read code yet.
2. Send the first part: `briefing` (two to four lines: the task, how the change solves
   it, what to watch; generated files `gr init` listed with ⚠ named, they are generated
   only by a comment in the file), and every core file with `order` per
   references/ordering.md and a `why` of about ten words. gr already put tests, CI and
   repository plumbing into their own groups (`gr init` labels them `tests`, `other`);
   name the boilerplate (DI wiring, mappers, route registration) with `tier: boilerplate`,
   and move a file gr grouped wrongly with `tier: core`. This part should reach the
   viewer within a minute of `gr open`.
3. Read the files in that order, each with enough surrounding code to be sure of what it
   does, looking for problems per references/checklist.md. After each file (or two or
   three small ones) send a part with its `hotspots` and `annotations`, and the file in
   `files` again if its `why` changed. Between files run `gr wait --timeout 1s` and
   handle what the human wrote (see Events).
4. When the last file is sent, `gr say` one line: «read everything: N flags», then go
   to Events.

What goes into a part:

- `hotspots` per references/hotspots.md: `cat`, `file`, `line` (0 for the whole file),
  a concrete question `q`, and a `detail`.
- `annotations` where an explanation saves the reader real effort: what a non-obvious
  call does and why, where the spec disagrees (`kind: spec`). Full sentences, not
  fragments. One to three per file; none is fine.
- A `detail` on every annotation and hotspot in three short paragraphs, one or two
  sentences each — `**Problem.** …`, `**When it bites.** …` (the scenario),
  `**What to do.** …` — plus a short code excerpt in a fenced block when it helps. Name
  other code as `path:line` (`UserIdentityService.kt:88`), never by bare name: the viewer
  shows a snippet of each under the detail and opens it on 1–9. `i` in the viewer shows
  it at once instead of asking you again.

The full shape of a part:

```yaml
briefing: |-
  Before: a retried POST /transfers debited twice.
  After: the idempotency key returns the first result.
  Watch the key check around the reserve transaction.
files:
  - { path: internal/di/wire.go, tier: boilerplate }
  - {
      path: api/openapi.yaml,
      order: 1,
      why: "the contract: 409 on a duplicate key",
    }
  - {
      path: api/transfer.go,
      order: 2,
      why: "handler: reserves, writes the outbox event",
    }
  - { path: api/transfer_test.go, order: 3, why: "pins the duplicate-key case" }
hotspots:
  - cat: consistency
    file: api/transfer.go
    line: 57
    q: "Retry after a timeout — second debit?"
    detail: |- # |- keeps the blank lines between the paragraphs; >- would fold them into one
      **Problem.** The key is checked in Redis before the transaction and written after it.

      **When it bites.** A retry that lands between the debit and the write passes the check and debits again.

      **What to do.** Check and write the key inside the reserve transaction (`api/transfer.go:61`).
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
```

## Parallel reading

1. Send the first part yourself (briefing, files with tier, order and why).
2. Split the core files into groups of related files, three to six groups. In one
   message start one general-purpose subagent per group, so they run at once. Give
   each: the code path, base and head, the task in two lines, its file list, and the
   path of this skill's `references/` directory. Its job: read `style.md`, `hotspots.md`
   and `checklist.md` there, read its files' code, write a part with `hotspots` and
   `annotations` for them (and `files` with a better `why` where it has one) to
   `/tmp/gr-plan/<id>/group-N.yaml`, never run `gr` itself, and reply with one line when
   done.
3. As each subagent finishes, `gr plan set -f /tmp/gr-plan/<id>/group-N.yaml` right
   away; `gr wait --timeout 1s` meanwhile, to answer the human. If gr rejects something,
   fix it in that file yourself. A group whose subagent failed you read yourself.

## Events

1. `gr wait` — run it with the Bash tool timeout at 600000 ms. "no input yet" means
   nothing happened: run it again. If your turn was interrupted and the human's next
   prompt is "Continue the guided review: run gr wait.", the viewer did that with `ctrl+c`:
   just run `gr wait`, their message is waiting there. Each printed line is one event from
   the viewer:

   - `[message] file:lines: text` — a remark: pick severity (blocker changes the
     approach, major is local rework, minor, nit),
     `gr comment add --file F --lines L --severity S [--suggestion TEXT] BODY` (BODY per
     references/style.md: readable for an author who was not here), then
     `gr say` one line: `noted: nit, transfer.go:57–58`. A question: `gr say` a short
     answer. Unsure which: ask one short question.
   - `[message]: text` — the same without a line anchor; ask for the lines if a
     remark needs them.
   - `[explain] file:lines` — read that code and send an annotation on those lines:
     two to four sentences on its role in the feature, not its syntax
     (references/style.md).
   - `[ask] file:lines: text` — a question about that code, never a remark: read it,
     answer with `gr say` in a few lines. Do not create comments or notes for it.
   - `[detail] file:line: <note text>` — a note without a `detail` (add one with every
     note from now on). Read the code again and write it in the same three short
     paragraphs — `**Problem.**`, `**When it bites.**`, `**What to do.**` — with a code
     excerpt when it helps, other code named as `path:line`. Send the note again with
     the `detail` filled in; the viewer shows it in the popup that is already open. No
     `gr say` needed.
   - `[reply] thread ID: directions` — the human wants a reply on that MR thread, yours
     or another reviewer's, written along those directions. Read the thread (`gr thread
     list`) and the code it is about, write the reply in the style of a review comment,
     and send it with `gr thread assess ID --propose open --reply TEXT "<one line on why>"`
     (keep `--propose resolve` if that was your take on your own thread). The viewer shows
     the draft in the thread card, where the human sends it or edits it; `gr say` one line:
     `reply drafted for thread ID`.
   - `[comment] file:lines: comment #N …` — the human saved that comment themselves,
     word for word (the viewer's default for a comment; an ai comment reaches you as a
     message instead). Do not add, edit or rephrase it and do not reply. `[comment]:
comment #N updated` / `comment #N deleted` is the human editing or deleting their
     own comment: nothing to do.

   `--suggestion` is the full replacement text for the lines; use it only for a nit
   or minor with an obvious fix.

   Messages usually carry the line the cursor was on (`file:line`): read the remark
   against that line. `re #N` means a reply to comment #N — answer it, and if the reply
   changes the remark, `gr comment edit N [--severity S] TEXT`. `[edit] #N: text` is
   the human rewriting their comment: `gr comment edit N TEXT` (keep the severity unless
   the new text clearly changes it) and `gr say` one line: `updated #N`.

2. A comment on a flagged line checks the flag off by itself. A blocker changes nothing
   for you: keep reading, the human decides when to finish.
3. If the human writes in the terminal chat instead, handle it the same way, then
   return to `gr wait`.

## Re-review (`gr init` printed "round N")

1. The diff is what changed since the last round (or, after a rebase or a merge of the
   target branch, the files whose patch changed), plus the files the human did not read
   last round, shown whole. Open comments from earlier rounds are listed.
2. First, before any reading: check each open comment against the new code. A comment
   that went out to the MR is one of your threads: assess it as below and leave the
   decision to the human. Only a comment that never reached the MR is yours to close:
   `gr comment resolve ID` when it is addressed. `gr say` a summary: resolved, still
   open, new questions.
3. Then read the round as usual, in parts. Your flags and notes from the last round are
   gone with the old diff: flag what still matters again.

## Your threads on the MR (`gr init` printed "your threads: …")

The reviewer's own open threads, with every reply, are in `gr thread list`. The
human decides each one; you prepare the decision. Do this before anything else in the
round, before reading the rest of the diff: a thread shows "agent is checking" in the
viewer until it has your assessment or you stop.

1. For every thread read the replies and the current code at that line. Then
   `gr thread assess ID --propose resolve|open [--reply TEXT] - <<'EOF' … EOF`: one or two
   sentences — fixed (where), the answer convinces (why), or it does not (what is still
   wrong). Propose `--reply` whenever the thread stays open, and for a resolve when the
   author asked something; keep it to the style of a review comment.
2. `gr say` one line per answered thread (`d1 a.go:5: not fixed, suggest keeping it open`).
   The threads sit on their lines in the viewer with the whole conversation and your
   take; `n`/`N` walk them and `enter` decides. Undecided threads block nothing. Decisions are the human's: the viewer records them itself and there
   is no command for it.
3. `[message] … re thread ID: …` is a question about that thread: answer with `gr say`;
   if your view changed, assess it again. Threads of other reviewers get replies too: the
   human replies from the viewer or asks you for a draft (`[reply]` above); both are
   posted with the review.

Nothing waits for the decisions: an undecided thread stays as it is on the MR. A decided
resolve also resolves the matching gr comment; undoing the decision takes that back.

## Self mode (`gr init` printed "mode: self")

The author is reviewing their own branch before anyone else sees it, and you were started
by their coding agent — as a subagent or in its own window — with a fresh context. That is the point: judge the code as a
stranger would. Do not look for or ask about how it was written.

- The prompt that started you gives the review id, the task (ticket, spec, a line or
  two), the author's language and the pane of the author's agent. Open the viewer with
  `gr open --return <that pane>`, so finishing lands the human back in their coding session.
- `gr init --self` was already run for you; `gr init --self` again just resumes.
- Intake: the task comes from the prompt, not from an MR. There are no MR discussions.
- Wrap-up: the viewer finishes as usual and, with no MR, asks nothing about approve. `P` writes
  `fixes.json` for the author's agent and closes the viewer. That is the end of your job:
  as a subagent, reply with the export dir as your final answer; in a window, just end
  your turn (the author's agent closes it). No publishing, no notifying.

`gr init` and `gr status` list unresolved discussions of other reviewers by their first
line; `gr discussions` refreshes them from the MR and prints them in full. The viewer
marks the ones on lines. Mention them in the briefing, and in a note on a discussed
line say whether the code answers it.

## Wrap-up

The viewer finishes on its own: on `P` it shows the verdict it derives from the comments
(blocker → blocked, major or an open thread of theirs → changes, else approve), the
risks checked and not checked, the files not read, asks whether to approve the MR, runs
`gr prepare`, previews the result and writes it with `P`. Nothing of that waits for you:
the human may finish while you are still reading. You only hear `[finished]: <dir>`,
after which the viewer is closed: talk in the terminal chat as usual — no `gr say` /
`gr wait` — and ending your turn with a question is fine.

While the preview is open the human can still message you — `re #N …` about one comment
(`gr comment edit N`), or about the summary (run `gr prepare` again with the verdict and
approve shown in the preview and the new decisions text); the viewer refreshes the
preview by itself. If they ask in the terminal to finish without the viewer, do it by
hand: `gr status`, `gr prepare --verdict approve|changes|blocked --decisions-file -`
with `--approve` only when they asked for it, then `gr export`, which prints the dir.
The summary lists the files the human did not read by itself.

After `[finished]`:

1. A local branch without an MR (`gr init` without a URL, not self mode): there is
   nothing to publish. The dir holds `fixes.json` (`verdict`, `decisions`, `fixes` with
   `id`, `severity`, `file`, `lines`, `body`, `suggestion`, and `unreviewed`: the files
   not read) and `review.md`. Tell them the counts and ask «apply now / leave». Apply
   now: every fix was agreed during the review, `suggestion` is the exact replacement,
   lines refer to the reviewed commit (find the spot by content if the file changed);
   report one line per fix and run the tests. Leave: give the `fixes.json` path; `gr
list` prints it (and `gr status` while the review is open), so any agent in this repo
   can pick it up later. Then step 4.
2. Publish. `[finished]: published <dir>` means the human already published it from the
   viewer: go to step 3 with the MR link. `[finished]: <dir>` means they handed it to
   you. The dir holds `review.md` (what will be posted) and
   `review.json` (`provider`, `host`, `api`, `url`, `verdict`, `approve`, …). Tell them what
   goes out — N comments, thread replies and resolves, the summary, the verdict, approve or
   not — and ask «publish?». Only on a clear yes, run `gr publish` in the repository.
   It posts the review (GitLab: draft notes, then one bulk publish, then approve; GitHub:
   one review request with the summary, the verdict and every comment), then replies to and
   resolves your threads, logs each write that went through to `<dir>/published.jsonl`, and
   ends with `gr mark-published`, which marks exactly what is logged and prints
   `not published: …` for the rest.
   If it stops, say the error and what went out. Once the cause is fixed (a login, the
   network), run `gr export` and `gr publish` again: the new export holds only what did not
   go out, and on GitLab the script reuses drafts that already sit on the MR instead of
   posting them twice. If the MR holds draft notes that are not from this export, the
   script stops before publishing, because bulk publish would post them too: ask the
   reviewer what to do with them. Never write `published.jsonl` or run `gr mark-published`
   by hand. GitHub refuses APPROVE and REQUEST_CHANGES on your own PR: then set
   `"event": "COMMENT"` in `review-request.json` and run `gr publish` again without
   `gr export`. After success give the link.
3. Tell the author. Run `gr notify`: it prints JSON with `to` (where the human said to
   announce it, empty for the author), `author`, `author_name`, `url`, `label`, `verdict`,
   `approved`, the `comments` counts by severity that actually went out, `replies` and
   `resolves`. If a `guided-review-notify` skill is available, follow it with that JSON:
   it holds the team's fixed message shape and its way of sending; write nothing of your
   own. Without one, print one line the human can forward
   (`reviewed !69: changes requested — 1 blocker, 2 major, 3 nit`). Verdict `approve`
   with `approved` false reads `no blockers, not approved`, never `approve`.
4. Ask «close the review?». On yes, `gr done` (removes the worktree, keeps the state for a
   re-review).
