# guided-review guide

What happens during a review, and how to configure it. Installation is in
[INSTALL.md](../INSTALL.md); every key is listed by `?` in the viewer.

## How it works

1. **You start reading at once.** `gr init` takes the diff and the viewer opens on it:
   the whole change as one diff, a file tree on the right.
2. **The agent reads next to you.** As it goes, the tree fills in: a number per file in
   the order it suggests, a line on why the file changed, ⚑ on risky lines, notes on
   tricky ones. A briefing on top says what the change is about.
3. **You say what is wrong.** In your own words; the agent turns it into review comments.
4. **Nothing is hidden.** The tree marks the files you have not seen and the ones the
   agent has not read; the summary lists both.
5. **You approve what gets posted.** `gr` writes the result locally; the agent posts it
   only after your yes.
6. **Next round shows only what changed**, starting with the replies to your threads.

<details>
<summary>Getting around</summary>

The screen is the diff on the left and, on the right, the file tree with a small chat
block at the bottom (`c` opens it wide, `esc` folds it). `j`/`k` (or the mouse) move,
`enter` acts on the line under the cursor, `esc` or `q` backs out (`Q` or `:q` quits the
viewer):

- on code it opens an input right under the line for a comment, saved as you write it
  (`shift+tab` sets its severity); `tab` makes it an ai comment, which the agent writes
  from your words, and `tab` again a question (an empty one asks the agent to explain the
  line);
- on your own comment it edits it, on an agent's note it shows the longer explanation,
  on `⋯` or `▸` it opens the hidden lines, on a folded group (boilerplate, generated) it
  opens the group.

`]`/`[` jump between changes, `}`/`{` between files in the tree's order, `n`/`N` between
notes: flags, threads, comments. `f` focuses the tree; `enter` there jumps to the file.

A tree row shows the file, its order number once the agent set one, `⚑N` for its flags
and `●` while it has changes you have not seen; a file the agent has not read yet is dim.
Boilerplate and generated files are two folded groups at the end of the tree and of the
diff. A risk (`RISK`) stays open until you check it off with `x`, on its line or in its
details, or until you comment on its line. The footer shows `NORMAL`, or `VISUAL` with
the number of selected lines.

`?` shows the main keys and `?` again all of them, and `:` finds any action by name.
Every other key is a shortcut for something these already reach.

</details>

<details>
<summary>The agent's notes</summary>

The order follows behaviour, not files: spec first, then the contract, each entry point
with the code it calls, tests right after the code they test, mechanics last. The agent
sends what it has as it reads, so the tree fills in file by file; on a tiny change it
only looks for flags and says so in the chat.

Notes explain non-obvious calls; ⚑ hotspots mark where a mistake is expensive: money,
security, consistency, migrations. Each has a longer explanation with the code it
mentions, so it opens instantly.

</details>

<details>
<summary>Talking to the agent</summary>

Point at a line or a selection, press Enter and write under it: Enter sends, Ctrl+J
starts a new line. Alt+Enter and Shift+Enter do too where the terminal passes them;
in tmux Shift+Enter needs `set -g extended-keys on` in `~/.tmux.conf`
(plus `set -as terminal-features 'xterm*:extkeys'` if the outer terminal does not
announce them), otherwise tmux delivers it as plain Enter. The line under the input shows
the mode (COMMENT, AI COMMENT, ASK, EDIT…), where the comment goes and the keys that work
right now. A comment goes out exactly as you typed it; an ai comment is turned by the
agent into a comment with a severity and, for an obvious fix, a suggestion. You can also
just ask about code without leaving a comment. When the agent needs a decision, it
offers answers you pick with one key. `t` moves into the chat, where `v` selects lines
and `y` copies them; dragging the mouse over chat lines copies them too.

</details>

<details>
<summary>No rubber stamp</summary>

The viewer tracks which changed lines you scrolled past: the tree marks the files with
some left, the finish card lists them with every risk you have not checked off, and the
posted summary says which files were not read. Nothing blocks publishing except your own
undecided MR threads.

</details>

<details>
<summary>Reading the diff</summary>

The unified view shows the resulting code. Large removals fold away, moved code is
marked as moved, whitespace-only changes and renames are marked instead of highlighted,
and only the changed words inside a line are coloured. Long lines wrap under the code
column; `W`, `:set nowrap` or `no_wrap: true` under `view:` in the config cuts them at `›`
instead, and `h`/`l` scroll sideways. With a language server you get
definitions, references, implementations, callers (`g` lists them). A target inside the
change moves the cursor there (`C-o` back, `S-enter` peeks anyway); others open over the
code, where `/` searches and `esc` closes. The line under the title follows the
cursor: the class and function you are in and who calls that function; HERE under the
files shows the type and docs of the word under the cursor.

</details>

<details>
<summary>Publishing</summary>

`P` derives the verdict from your comments (blocker → blocked, major or an open thread
→ changes, else approve), lists the files you did not read and the risks you did not
check, and asks whether to approve the MR. It writes inline comments
and the summary to `<git common dir>/guided-review/exports/<id>/` (`gr export --dir`
prints it). You review and edit it (in the preview `v` changes the verdict, `a` approve,
`s` a comment's severity, and the verdict follows severity until you change it), then
publish it from the viewer or hand it to the agent; both run `gr publish`, which needs
`glab` or `gh` and `jq`: GitLab draft notes in one batch or one GitHub review. If posting
stops halfway, the next attempt sends only what did not go out. A `guided-review-notify` skill, if present, then
tells the author. A local branch without an MR ends in `fixes.json` instead, which the
agent applies right away or leaves for later (`gr list` shows where it is).

</details>

<details>
<summary>Next round and your threads</summary>

Run the same command after the author pushes: you see only what changed. Your open
threads come first with the author's replies. The agent checks each reply against the
new code and suggests resolve or keep open with a reply; until it has, a thread says
«agent is checking». Each thread sits in the code with the whole conversation, the
agent's take and your decision under it. `n`/`N` walk the notes, `enter` decides
(resolve, keep open and reply to the author, as the agent proposes) and moves to the
next undecided thread, `c` asks the agent about it. Undecided threads do not hold up the
reading, only publishing. Only you decide: the agent has no command for it. Approve is
refused while any of your threads stays open.

You can finish before reading everything: the summary lists what you did not read, and
the next round brings those files back after the round's own changes.

</details>

<details>
<summary>Self-review</summary>

In the session where you wrote the code, run `/guided-selfreview`. A fresh reviewer
that knows only the task reads the branch next to you. The fixes you agree on go back to
your session, which applies them and offers another round.

</details>

## Settings

- `gr config init` writes `~/.config/guided-review/config.yaml` with every setting
  commented out: default view, diff algorithm, language servers, key bindings.
- `gr config` shows the effective settings and reports key conflicts.
- Colours: `:theme` in the viewer tries each theme live and `enter` saves it as
  `theme:` under `view:`. Besides the default there are catppuccin (mocha, latte),
  gruvbox (dark, light), nord, dracula, tokyonight, one-dark, solarized (dark, light)
  and github (dark, light); `style:` still picks the code highlighting on its own.
- `.review.yaml` in a repository:

      domain: finance          # raises the bar for money-related changes
      diff: histogram          # histogram (default) | patience | myers | minimal
      lsp:                     # language servers per language
        kotlin: [kotlin-lsp, --stdio]
      generated:               # extra generated-file patterns
        - api/gen/**

## Extending

Team-specific steps stay out of this repository. After publishing, the agent looks for
a skill named `guided-review-notify` and follows it with the merge request link, the
verdict and the comment counts — for example to message the author in your team chat.
Put it in `~/.claude/skills/guided-review-notify/SKILL.md` (or the Codex equivalent);
without it the agent prints a line to forward.
