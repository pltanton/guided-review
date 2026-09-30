---
name: guided-selfreview
description: Use when the author wants to review their own change before sending it — "давай сами поревьюим перед отправкой", "проверь мой MR свежим взглядом", "self-review", "guided self-review" — in the session where the code was written. Starts a fresh-context reviewer that walks the author through the branch in the guided-review viewer, then applies the fixes it returns.
---

# Guided self-review

You wrote this change, so you are the wrong one to review it: you know what the code was
meant to do and read that into it. Hand the review to a fresh agent, let the human walk
through it in the viewer, and take back a list of fixes. Needs tmux and `gr`.

## Start

1. `gr` reviews commits (default branch → HEAD). If `git status` shows changes, ask
   whether to commit them (a WIP commit is fine); never stash or commit silently.
2. `gr init --self` in the repository. Note the id it prints (`review self-<branch>`).
3. Write the reviewer's prompt:
   ```
   Guided review, self mode. Use the guided-review skill.
   Review id: <id>. Author agent pane: <your $TMUX_PANE>.
   Task: <one to three lines: what the change must do, where the ticket or spec is>
   ```
   Only the goal. Not how you implemented it and not what you think is fragile: the
   reviewer is useful exactly because it does not know.
4. Start the reviewer:
   - **Claude Code:** a general-purpose subagent with that prompt, in the background. It
     starts without your context. Tell the human in one line that the reviewer is
     running as a subagent and its viewer opens in a `review-<id>` window; they talk to
     it there until they press `P`. You are told when it finishes — do not poll.
   - **Codex or no subagents:** write the prompt to `/tmp/guided-review/<id>.prompt`, then
     ```bash
     tmux new-window -P -F '#{window_id}' -n selfreview -c "$PWD" "codex \"\$(cat /tmp/guided-review/<id>.prompt)\""
     ```
     and wait, with the Bash tool timeout at 600000 ms, repeating until the file appears:
     ```bash
     timeout 590 sh -c 'until [ -f /tmp/guided-review/<id>/fixes.json ]; do sleep 5; done'
     ```
     then `tmux kill-window -t <window id>`.
5. When the reviewer is done: `gr done`.

## Apply

`/tmp/guided-review/<id>/fixes.json` has `verdict`, `decisions` and `fixes` (`id`,
`severity`, `file`, `lines`, `body`, `suggestion`); `review.md` next to it is the same for
reading.

- Every fix was agreed with the human during the review: apply it. `suggestion` is the
  exact replacement for those lines.
- Disagree only with a concrete reason — it breaks X, the spec says Y — in one line. Do
  not defend the original code.
- Lines refer to the reviewed commit; if the file changed since, find the spot by content.
- Report one line per fix: done, or not done and why. Run the tests.
- Offer another round: after a commit, `gr init --self` shows only what changed and starts
  by checking these fixes. Same flow from step 3.
