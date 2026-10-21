# Changelog

## Unreleased

- FLOW is gone; the code follows the cursor instead. The line under the step title shows
  the class and function you are in and who calls that function (`+N` for the ones that
  do not fit); HERE under the files shows the hover of the word under the cursor.
- LSP: a definition (or a list item) that this step shows moves the cursor there instead
  of opening a peek; `C-o` goes back, `S-enter` peeks anyway. LSP windows say so: an
  `LSP` badge in the statusline and `LSP ·` in the title.
- The viewer no longer stalls on every state change: picking the syntax highlighter
  globbed every lexer for each removed line. A reload of a 950-row step took 820 ms, now
  16 ms; the whole-MR view 11.8 s, now 1.4 s on first open.
- The finish preview: `v` and `a` no longer error out. `v` keeps a partial review
  partial and skips approve while your threads are open; `a` turns approve on together
  with the approve verdict, or says which threads stand in the way.
- The plan arrives whole again: the agent reads every step and sends route and notes in
  one `gr plan set`, so you never walk steps the notes have not reached. `gr plan set
  --route` and `gr plan fill` are gone.
- Your MR threads are a step now, not a screen: on a re-review the viewer opens «your
  threads» first, each thread in the current code with the whole conversation, the
  agent's take and your decision under it. `n`/`N` walk them, `enter` decides, the reply
  to the author is edited in place, `c` asks the agent about the thread, `R` comes back.
- The LSP code peek searches: `/` jumps as you type, `n`/`N` walk the matches.
- Editing a comment (`enter` on it) changes its severity too: `S-tab` cycles it, the
  prompt shows it.
- The chat reads cleaner: `code` and **bold** are styled instead of shown as markup,
  `⚑57` reads `line 57`, paths shrink to the file name, and your messages lead with a
  short dim `file:line · risk «…»` instead of the full path and the whole risk text.
- A suggestion whose first line starts with a tab (most Go code) broke the review state:
  every later `gr` command failed with «found a tab character where an indentation space
  is expected».
- Verdict `approve` without approving the MR no longer reads as an approval: the summary
  says «no blockers, not approved» and the author's notification says the same.
- Holding `j`/`k` on a big step no longer lags: a frame took tens of milliseconds on
  bookkeeping that is now done once, without strings or styling.
- The chapter intro lives in the plan: `p` shows every chapter with its steps, and an
  unfolded chapter shows its intro under the heading. A chapter's first step opens the
  plan with the cursor on that step and the intro in view; `enter` goes on, `I` brings
  it back; chapters you passed stay folded; `h`/`l` fold and unfold while you browse.
- Intake no longer asks «got it right?» and waits: the agent says what the MR is about
  in two lines and goes straight to the plan.
- A "keep open" after the author's new reply left the comment of an earlier "resolve"
  resolved for good; a published "keep open" no longer counts as closed for approve.
- `gr prepare` after a partial publish forgot what already went out, so the next export
  approved or posted the verdict twice.
- A new round refuses to drop an export that is published but not yet marked.
- `diff.submodule=log` in the git config broke every diff with a submodule bump.
- Plans: steps without a chapter are accepted between chapters; the 300-line limit
  counts deleted files.
- A language server that stopped answering made every request wait 90 s; it is now
  restarted at once.
- An agent killed while `gr wait` ran left the viewer on "your turn" for good; the marker
  now expires with the wait.
- A key conflict or an unknown theme in the config shows in the viewer on start, not only
  in `gr config`.
- Viewer: `esc` drops a half-written suggestion, an empty chat message closes the chat
  again, the selection survives a switch to split view, `?` is ignored where no overlay
  can draw, cards close when the review or step disappears.
- Gone: `:sym` (workspace symbol search), `ctrl+r` in the composer (`tab` does it); `:skip`
  runs `gr step skip` like `S` does. `gr help` lists `plan set --route` and `plan fill`.
- Gone: the reading of state files and MR threads written before 2026-10-01 (step
  `details:` lists, threads without the `gr:comment` marker).

## v0.1.1

- **Threads in the code.** A thread on a line shows whose it is and what was decided:
  `@kai`, your `YOU ↩2` (two replies), then a green `✓ RESOLVE` or `↩ OPEN`. `enter` on it
  shows the conversation and the agent's assessment: resolve, reply and keep open, take
  the agent's suggestion, or undo.
- When answered threads of yours wait for a decision, the card before moving on and the
  finish say so; `R` opens them.
- A checked risk and a resolved comment wear a green `✓ DONE` badge on the left.
- A comment on a risk's line checks the risk off, whichever step it is filed under.
- The approve question is one choice, `1` approve or `2` do not, and moves straight on.
- Choices in cards are numbered: a digit picks one, arrows still work.
- **Themes.** `:theme` tries catppuccin, gruvbox, nord, dracula, tokyonight, one-dark,
  solarized and github live; `enter` saves the choice as `view.theme`.
- **What's new.** After an update the viewer shows this list once; `:changelog` brings it
  back, `gr version` prints the version.
- With the chat folded, the files and the call flow get the rest of the right column.

## v0.1.0

- **A simpler screen.** Code on the left; the step's files and a small chat block on the
  right. One-line header. The chapter intro, the plan (`p`), the keys (`?`) and note
  details open as cards; LSP results open over the code, `/` filters them.
- **`enter` does the obvious thing.** On code it opens a comment box under the line: saved
  as typed, `S-tab` severity, `C-s` suggestion, `tab` for an ai comment the agent writes,
  `tab` again for a question. On your comment it edits, on a note it shows the details,
  on the last line of a step it moves on.
- **Risks you check off** with `x` or a comment on their line; moving on with open risks
  or unseen lines asks first.
- **Done without the agent:** next and skip, the question after a blocker, the verdict and
  decisions at the finish, the approve question, and publishing with `gr publish`.
- `c` chats with the agent and stays open until `esc`; `:` is a palette of actions; `a`
  asks, `A` goes to the agent, `q` backs out, `Q` quits, `<` shows the previous step.
