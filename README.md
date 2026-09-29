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

Requirements: git, tmux for the viewer pane, `glab` (authenticated) for MR URLs. LSP
navigation uses `gopls`, `kotlin-lsp`, `basedpyright-langserver` and friends when they
are on `PATH`; the server starts in the review's worktree on first use.

## Use

In tmux, in the repository (any branch — the MR is checked out into its own worktree):

    > /guided-review https://gitlab.example.com/group/project/-/merge_requests/123

The viewer opens full screen in a `review` tmux window. The bottom line shows what the
agent is doing (spinner, progress text, timer) or `● your turn` when it is your turn.

At the end the agent publishes to the MR with `gr publish` — inline comments (nits with an
obvious fix as GitLab suggestions) and a summary comment with the verdict, decisions,
the step table and coverage — as draft notes released in one batch, after you confirm the
preview. It can also draft a Slack message to the author.

After the author pushes fixes, run the same command again: the agent reviews only what
changed and checks the open comments first.

Optional `.review.yaml` in the repository root:

    domain: finance          # raises the bar for money-related changes
    diff: histogram          # histogram (default) | patience | myers | minimal
    lsp:                     # override LSP servers per language
      kotlin: [kotlin-lsp, --stdio]
    generated:               # extra generated-file patterns
      - api/gen/**

## Reading the diff

The unified view shows the resulting code: large removed blocks fold into
`▸ N lines removed`, code moved elsewhere is marked `↕` and its old copy folds into
`↕ N lines moved to file:line`, whitespace-only changes are marked `≈`. When a line
changed only partly, just the changed words are highlighted. Split (`s`) always shows
both sides in full.

## Settings

- `h` in the viewer lists every key, grouped.
- `gr config init` writes `~/.config/guided-review/config.yaml` with every setting commented
  out: default view (split, plan panel, mouse, context lines, syntax style), diff algorithm,
  LSP servers and the key map (`action: [keys]`, sequences like `"g d"`).
- `gr config` shows the effective settings and reports unknown actions or key conflicts.
- A repository's `.review.yaml` overrides `diff` and `lsp`.

## Command line and search

`:` opens a vim-style command line (`tab` completes, `↑`/`↓` history):

| Command | Does |
|---|---|
| `:42` | go to line 42 of the file under the cursor |
| `:s3`, `:all`, `:boilerplate`, `:generated` | open a step or view |
| `:f transfer` | jump to the first file of the step whose path contains the text |
| `:set context=10`, `:set diff=patience` | context lines, diff algorithm |
| `:msg text`, `:skip reason`, `:q` | message the agent, skip the step, quit |
| `:split`, `:next`, `:definition`, … | any action by name (see `h`) |

`/text` searches the current step (smart case); `n`/`N` then walk the matches, `esc` clears the search and gives `n`/`N` back to annotations.

## Viewer keys (defaults)

| Key | Action |
|---|---|
| `j`/`k`, wheel | move |
| `]`/`[` | next/previous hunk |
| `n`/`N` | next/previous annotation |
| `v`, mouse drag | select lines |
| `c` / `enter` | message the agent; the cursor line (or selection) is attached, `backspace` on empty input (or `ctrl+x`) detaches it; on one of your comments it is a reply |
| `ctrl+r` while writing | raw mode: the text is saved as a comment exactly as typed, without the agent (`tab` picks the severity); works for `E` edits too |
| `E` | edit the comment under the cursor |
| `D` `D` | delete the comment under the cursor (not yet published) |
| `t` / `T` | enlarge / shrink the chat (small → half → full screen) |
| `P` | publish: first press shows the full preview, second posts it to the MR (after the agent prepared it) |
| `?` | ask the agent about the line or selection (answer only, no comment); `enter` alone asks it to explain the code |
| `>` | next step |
| `S` | skip the step with a reason |
| `H`/`L`, click a step in the plan | look at an earlier/later step without losing progress; `esc` returns |
| `L` past the last step | `boilerplate`, `generated` and `all changes` views: every diff of the MR, nothing left out |
| `o`, click | open what is under the cursor: `⋯ N hidden lines` of unchanged code or a `▸ removed lines hidden` block |
| `O` | show every removed line / fold large removed blocks again |
| `d` | next diff algorithm (histogram → patience → myers → minimal) |
| `s` | split / unified diff |
| `p` | show/hide the plan |
| `m` | mouse capture on/off (off lets the terminal select text) |
| `w` / `b`, click a word | move the symbol cursor within the line |
| `g` | shows what can follow it (`gg`, `gd`, `gr`); `esc` cancels |
| `gd` / `gr` / `K` | LSP: definition (peek), references (list with a code preview of the selected one, `enter` peeks), hover; `esc` / `ctrl+o` back, `e` opens the editor there |
| `tab` / `shift+tab` | more / default context |
| `e` | open `$EDITOR` at the line (tmux popup) |
| `a` | back to the agent's tmux window |
| `q` | quit |

Design: [docs/specs/2026-09-25-guided-review-design.md](docs/specs/2026-09-25-guided-review-design.md)
