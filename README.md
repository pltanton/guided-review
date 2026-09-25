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
    diff: histogram          # histogram (default) | patience | myers | minimal
    generated:               # extra generated-file patterns
      - api/gen/**

## Reading the diff

The unified view shows the resulting code: large removed blocks fold into
`▸ N lines removed`, code moved elsewhere is marked `↕` and its old copy folds into
`↕ N lines moved to file:line`, whitespace-only changes are marked `≈`. When a line
changed only partly, just the changed words are highlighted. Split (`s`) always shows
both sides in full.

## Viewer keys

| Key | Action |
|---|---|
| `j`/`k`, wheel | move |
| `]`/`[` | next/previous hunk |
| `n`/`N` | next/previous annotation |
| `v`, mouse drag | select lines |
| `c` / `enter` | message the agent; the cursor line (or selection) is attached, `ctrl+x` detaches it; on one of your comments it is a reply |
| `E` | edit the comment under the cursor |
| `?` | ask the agent to explain the selection or the line |
| `>` | next step |
| `S` | skip the step with a reason |
| `H`/`L`, click a step in the plan | look at an earlier/later step without losing progress; `esc` returns |
| `o` / `O` | unfold the removed block under the cursor / show all removed lines |
| `d` | next diff algorithm (histogram → patience → myers → minimal) |
| `s` | split / unified diff |
| `p` | show/hide the plan |
| `m` | mouse capture on/off (off lets the terminal select text) |
| `tab` / `shift+tab` | more / default context |
| `e` | open `$EDITOR` at the line (tmux popup) |
| `a` | back to the agent's tmux window |
| `q` | quit |

Design: [docs/specs/2026-09-25-guided-review-design.md](docs/specs/2026-09-25-guided-review-design.md)
