# guided-review

An agent walks you through a merge request in small, ordered steps. Everything
happens in one viewer pane: the plan, the code with the agent's annotations, its
messages, and your replies. The agent explains each step in a few lines, flags risky
spots, records your remarks, and on the next round checks whether they were fixed.

## Install

    go install github.com/aplotnikov/guided-review/cmd/gr@latest

Claude Code:

    /plugin marketplace add aplotnikov/guided-review
    /plugin install guided-review@guided-review

Codex:

    ln -s "$PWD/skills/guided-review" ~/.codex/skills/guided-review

Requirements: git, tmux for the viewer pane, `glab` (authenticated) for MR URLs.

## Use

In tmux, in the repository (any branch — the MR is checked out into its own worktree):

    > /guided-review https://gitlab.example.com/group/project/-/merge_requests/123

The viewer opens full screen in a `review` tmux window. The bottom line shows what the
agent is doing (spinner, progress text, timer) or `● your turn` when it is your turn.

After the author pushes fixes, run the same command again: the agent reviews only what
changed and checks the open comments first.

Optional `.review.yaml` in the repository root:

    domain: finance          # raises the bar for money-related changes
    generated:               # extra generated-file patterns
      - api/gen/**

## Viewer keys

| Key | Action |
|---|---|
| `j`/`k`, wheel | move |
| `]`/`[` | next/previous hunk |
| `n`/`N` | next/previous annotation |
| `v`, mouse drag | select lines |
| `c` / `enter` | message the agent (the selection is attached) |
| `?` | ask the agent to explain the selection or the line |
| `>` | next step |
| `S` | skip the step with a reason |
| click a step in the plan | go to it |
| `s` | split / unified diff |
| `p` | show/hide the plan |
| `m` | mouse capture on/off (off lets the terminal select text) |
| `tab` / `shift+tab` | more / default context |
| `e` | open `$EDITOR` at the line (tmux popup) |
| `a` | back to the agent's tmux window |
| `q` | quit |

Design: [docs/specs/2026-09-25-guided-review-design.md](docs/specs/2026-09-25-guided-review-design.md)
