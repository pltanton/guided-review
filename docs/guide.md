# guided-review guide

What happens during a review, and how to configure it. Installation is in
[INSTALL.md](../INSTALL.md); every key is listed by `?` in the viewer.

## How it works

1. **The agent plans.** It reads the whole change and splits it into small steps,
   grouped by behaviour, in reading order.
2. **You walk the steps.** Each step is a piece of code with a two-line explanation,
   notes on tricky lines and ⚑ on risky ones.
3. **You say what is wrong.** In your own words; the agent turns it into review comments.
4. **Nothing is skipped silently.** Every step needs your "next" or a skip with a reason.
5. **You approve what gets posted.** `gr` writes the result locally; the agent posts it
   only after your yes.
6. **Next round shows only what changed**, starting with the replies to your threads.

<details>
<summary>Getting around</summary>

The screen is the step's code on the left and, on the right, the step's files with a
small chat block at the bottom (`c` opens it wide, `esc` folds it). `j`/`k` (or the mouse) move, `enter` acts on the line under the
cursor, `esc` or `q` backs out (`Q` or `:q` quits the viewer):

- on code it opens an input right under the line for a comment, saved as you write it
  (`shift+tab` sets its severity); `tab` makes it an ai comment, which the agent writes
  from your words, and `tab` again a question (an empty one asks the agent to explain the
  line);
- on your own comment it edits it, on an agent's note it shows the longer explanation,
  on `⋯` or `▸` it opens the hidden lines;
- on the last line of a step, `✓ end of s3`, it moves to the next step.

A risk (`RISK`) stays open until you check it off with `x`, on its line or in its
details. Moving on with a risk still open, or with changed lines you have not seen, asks
first and lets you check the risks off right there. The summary counts only the risks
you checked. The footer shows `NORMAL`, or `VISUAL` with the number of selected lines.

`p` opens the plan: chapters with their steps, and under an unfolded chapter its intro.
It opens by itself when a chapter starts, on that chapter; `I` brings it back there.
`?` shows the main keys and `?` again all of them, and `:` finds any action by name.
Every other key is a shortcut for something these already reach.

</details>

<details>
<summary>Plan and steps</summary>

Chapters follow behaviour ("transfers", "limits", "migration"), not files. Inside a
chapter: spec, contracts, each entry point with the code it calls, tests right after
the code they test. A step is up to about 300 changed lines. Boilerplate and generated
files stay out of the steps but are one keypress away.

Notes explain non-obvious calls; ⚑ hotspots mark where a mistake is expensive: money,
security, consistency, migrations. Each has a longer explanation with the code it
mentions, written during planning, so it opens instantly.

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

The viewer tracks which changed lines you scrolled past and, before you move on, lists
the rest and every risk you have not checked off. Nothing can be published until every step is
reviewed or skipped; steps a blocker made stale are listed in the summary as not reviewed.

</details>

<details>
<summary>Reading the diff</summary>

The unified view shows the resulting code. Large removals fold away, moved code is
marked as moved, whitespace-only changes and renames are marked instead of highlighted,
and only the changed words inside a line are coloured. Long lines wrap under the code
column; `W`, `:set nowrap` or `no_wrap: true` under `view:` in the config cuts them at `›`
instead, and `h`/`l` scroll sideways. With a language server you get
definitions, references, implementations, callers (`g` lists them), and a call flow for
the step under its files. Results open over the code; `/` filters a list, `esc` closes.

</details>

<details>
<summary>Publishing</summary>

After the last step the viewer derives the verdict from your comments (blocker →
blocked, major or an open thread → changes, else approve), lists what was skipped and
which risks you checked, and asks whether to approve the MR. It writes inline comments
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
new code and suggests resolve or keep open with a reply; you decide in the viewer, with
`R` for all of them or `enter` on a thread where it sits in the code (read it, resolve,
reply and keep open, take the agent's suggestion), and only there: the agent has no
command to decide for you. Approve is refused
while any of your threads stays open.

On a large change you can finish early: `gr prepare --partial` sends what you reviewed
so far and lists the rest as not reviewed yet; the next round brings those steps back
after the round's own changes.

</details>

<details>
<summary>Self-review</summary>

In the session where you wrote the code, run `/guided-selfreview`. A fresh reviewer
that knows only the task walks you through the branch. The fixes you agree on go back to
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
