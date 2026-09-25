# guided-review

An agent walks you through a merge request in small, ordered steps. You read the code
in a viewer pane and reply in chat; the agent explains each step in a few lines,
flags risky spots and records your remarks.

## Install

    go install github.com/aplotnikov/guided-review/cmd/gr@latest

Claude Code:

    /plugin marketplace add aplotnikov/guided-review
    /plugin install guided-review@guided-review

Codex:

    ln -s "$PWD/skills/guided-review" ~/.codex/skills/guided-review

Requirements: git, tmux for the viewer pane, `glab` (authenticated) for MR URLs.

## Use

In tmux, in the repository, on the MR branch:

    > review https://gitlab.example.com/group/project/-/merge_requests/123

Optional `.review.yaml` in the repository root:

    domain: finance          # raises the bar for money-related changes
    generated:               # extra generated-file patterns
      - api/gen/**

## Viewer keys

`j`/`k` move, `]`/`[` next/previous hunk, `tab` more context, `e` open `$EDITOR` at
the line (tmux popup), `q` quit.

Design: [docs/specs/2026-09-25-guided-review-design.md](docs/specs/2026-09-25-guided-review-design.md)
