# guided-review guide

What happens during a review, and how to configure it. Installation is in
[INSTALL.md](../INSTALL.md); every key is listed by `h` in the viewer.

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

Point at a line or a selection and write. The agent writes the comment with a severity
and, for an obvious fix, a suggestion. You can also just ask about code without leaving
a comment, or save a comment exactly as typed. When the agent needs a decision, it
offers answers you pick with one key.

</details>

<details>
<summary>No rubber stamp</summary>

The viewer tracks which changed lines you scrolled past and reminds you once about the
rest and about hotspots nobody discussed. Nothing can be published until every step is
reviewed or skipped.

</details>

<details>
<summary>Reading the diff</summary>

The unified view shows the resulting code. Large removals fold away, moved code is
marked as moved, whitespace-only changes and renames are marked instead of highlighted,
and only the changed words inside a line are coloured. With a language server you get
definitions, references, implementations, callers, and a call flow for the step.

</details>

<details>
<summary>Publishing</summary>

`gr` never writes to GitLab or GitHub. At the end it writes inline comments and a
summary (verdict, decisions, what was skipped and why) to `/tmp/guided-review/<id>/`.
You review and edit it, then the agent posts it: GitLab draft notes in one batch or one
GitHub review. A `guided-review-notify` skill, if present, then tells the author.

</details>

<details>
<summary>Next round and your threads</summary>

Run the same command after the author pushes: you see only what changed. Your open
threads come first with the author's replies. The agent checks each reply against the
new code and suggests resolve or keep open with a reply; you decide. Approve is refused
while any of your threads stays open.

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

