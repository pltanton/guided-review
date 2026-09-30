# guided-review

Agents write code faster than people can review it. Merge requests got longer and more
frequent, reading each one properly takes an hour nobody has, and review turns into a
skim and "LGTM". guided-review keeps the human review and takes the grind out of it.

An agent reads the merge request first and cuts it into small steps in an order that
makes sense. You go through the steps in a terminal viewer and decide what is wrong. The
agent explains each piece, points at the places where a bug would be expensive, turns
your remarks into review comments and, when you are done, posts them.

It does not review for you. Every step needs your "next" or a skip with a reason, and
nothing reaches the merge request before you have seen it.

Works with GitLab (`glab`) and GitHub (`gh`), Claude Code or Codex as the agent, tmux for
the viewer.

## Install

**Ask your agent.** Paste this into Claude Code or Codex:

> Install guided-review from https://github.com/pltanton/guided-review, follow its INSTALL.md

It checks the prerequisites, installs `gr` and the skills, and tells you what is left
for you to do (logging in to `glab` or `gh`, adding Go's bin directory to `PATH`).

**By hand**, in a shell:

    go install github.com/pltanton/guided-review/cmd/gr@latest
    claude plugin marketplace add pltanton/guided-review
    claude plugin install guided-review@guided-review

or, for the plugin, inside a Claude Code session:

    /plugin marketplace add pltanton/guided-review
    /plugin install guided-review@guided-review

Codex, language servers and the prerequisites are in [INSTALL.md](INSTALL.md).

## Use

In tmux, inside the repository:

    /guided-review https://gitlab.example.com/group/project/-/merge_requests/123

The merge request is checked out into its own worktree, so your branch stays as it is.
The viewer opens in a `review-<id>` tmux window; the agent keeps its own pane. `h` in the
viewer lists every key.

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

## License

Apache-2.0, see [LICENSE](LICENSE).
