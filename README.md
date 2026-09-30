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

Easiest: ask your agent.

> Install guided-review from https://github.com/pltanton/guided-review, follow its INSTALL.md

By hand:

    go install github.com/pltanton/guided-review/cmd/gr@latest
    claude plugin marketplace add pltanton/guided-review
    claude plugin install guided-review@guided-review

Codex, other language servers and the prerequisites are in [INSTALL.md](INSTALL.md).

## Use

In tmux, inside the repository:

    /guided-review https://gitlab.example.com/group/project/-/merge_requests/123

The merge request is checked out into its own worktree, so your branch stays as it is.
The viewer opens in a `review-<id>` tmux window; the agent keeps its own pane. `h` in the
viewer lists every key.

## How it works

**Plan.** The agent reads the whole change before you see anything and groups it into
chapters by behaviour — "transfers", "limits", "migration" — not by file. Inside a
chapter the steps go spec, contracts, then each entry point and the code it calls, tests
right after the code they test. A step is small: about 300 changed lines at most, unless
the agent says why it can't be split. Boilerplate and generated files are kept out of
the steps but stay one keypress away.

**Step.** Each step shows its code and a short message from the agent: what the code
does, where it disagrees with the spec, the one question worth asking. Some lines carry
notes that explain a non-obvious call; hotspots (⚑) mark the places where a mistake is
expensive — money, security, consistency, migrations. Behind every note and hotspot is
a longer explanation with the code it refers to, written while the agent planned, so it
opens instantly.

**Conversation.** You talk to the agent from the viewer. Point at a line or a selection
and write in your own words; the agent turns it into a review comment with a severity
and, for an obvious fix, a suggestion. You can also ask about code without leaving a
comment, or save a comment exactly as typed. When the agent needs a decision it offers
answers you pick with one key.

**No rubber stamp.** A step counts as reviewed only when you move past it or skip it
with a reason. The viewer tracks which changed lines you actually scrolled past and
reminds you once about the rest, or about a hotspot nobody discussed. Before anything is
published, every step must be reviewed or skipped.

**Reading the diff.** The unified view shows the resulting code. Large removals fold
away, moved code is marked as moved instead of shown twice, whitespace-only changes and
identifier renames are marked rather than highlighted, and only the words that changed
inside a line are coloured. With a language server installed you can jump to
definitions, references, implementations and callers, and the plan pane shows who calls
the functions changed in this step.

**Publishing.** `gr` never writes to GitLab or GitHub. When you finish, it writes the
result — inline comments, a summary with the verdict and the decisions you made, what
was skipped and why — to `/tmp/guided-review/<id>/`. You see it, edit or drop comments,
and the agent posts it only after you say yes: GitLab draft notes published in one
batch, or one GitHub review. A `guided-review-notify` skill, if you have one, then tells
the author (see Extending).

**Next round.** After the author pushes, run the same command again. You see only what
changed since your last round. Your open threads on the merge request come first, with
the author's replies: the agent checks each reply against the new code and suggests
resolve or keep open with a reply; you decide, and the replies and resolves go out with
the rest. Approving is refused while any of your threads stays open.

**Self-review.** In the session where the code was written, ask for a self-review
(`/guided-selfreview`). The agent that wrote the code does not review it: a fresh
reviewer gets only the task, no history, and walks you through the branch in the same
viewer. At the end the fixes you agreed on go back to the original session, which
applies them and offers another round.

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
