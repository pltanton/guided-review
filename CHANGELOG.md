# Changelog

## Unreleased

- An agent killed while `gr wait` ran left the viewer on "your turn" for good; the marker
  now expires with the wait.
- A key conflict or an unknown theme in the config shows in the viewer on start, not only
  in `gr config`.
- `gr help` lists `plan set --route` and `plan fill`.

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
