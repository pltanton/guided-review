# guided-review

Agents write code faster than people can review it. Merge requests got longer and more
frequent, reading each one properly takes an hour nobody has, and review turns into a
skim and "LGTM". guided-review keeps the human review and takes the grind out of it.

An agent reads the MR first and cuts it into small steps in an order that makes sense:
the spec, then contracts, then each entry point and the code it calls, tests right after
the code they test. You go through the steps in a terminal viewer and decide what is
wrong. The agent explains each piece in a couple of lines, points at the places where a
bug would be expensive, writes down your remarks and, when you are done, posts them to
the MR.

It does not review for you. Every step needs your "next" or a skip with a reason, and
nothing goes to GitLab before you have seen it. On the next round you only see what
changed, starting with whether your comments were addressed.

GitLab via `glab`, Claude Code or Codex as the agent, tmux for the viewer.

## Install

    go install github.com/pltanton/guided-review/cmd/gr@latest

Claude Code:

    /plugin marketplace add pltanton/guided-review
    /plugin install guided-review@guided-review

Codex:

    ln -s "$PWD/skills/guided-review" ~/.codex/skills/guided-review

Requirements: git, tmux for the viewer pane, `glab` (authenticated) for MR URLs. LSP
navigation uses `gopls`, `kotlin-lsp`, `basedpyright-langserver` and friends when they
are on `PATH`; the server starts in the review's worktree on first use.

## Use

In tmux, in the repository (any branch — the MR is checked out into its own worktree):

    > /guided-review https://gitlab.example.com/group/project/-/merge_requests/123

The viewer opens full screen in a `review-<id>` tmux window (restarted if it already exists). The bottom line shows what the
agent is doing (spinner, progress text, timer) or `● your turn` when it is your turn.

`gr` never writes to GitLab. At the end `P` in the viewer (or `gr export`) writes the
result to `/tmp/guided-review/<id>/`: `review.md` to read, `review.json` and one ready
GitLab draft-note body per inline comment (nits with an obvious fix as suggestions) plus a
summary with the verdict, decisions, the step table and coverage. The agent shows what
goes out, asks, posts the drafts with `glab` in one batch and runs `gr mark-published`.
It can also draft a Slack message to the author.

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
| `42G` / `42gg`, `5j`, `3]` | vim counts: go to line 42 of the file, repeat a move; typed keys show at the bottom right, `esc` drops them |
| `]`/`[` | next/previous hunk |
| `n`/`N` | next/previous annotation |
| `v`, mouse drag | select lines |
| `c` / `enter` | message the agent; the cursor line (or selection) is attached, `backspace` on empty input (or `ctrl+x`) detaches it; on one of your comments it is a reply |
| `ctrl+r` while writing | raw mode: the text is saved as a comment exactly as typed, without the agent (`tab` picks the severity); works for `E` edits too |
| `C` | message the agent without attaching a line |
| `E` | edit the comment under the cursor |
| `D` `D` | delete the comment under the cursor (not yet published) |
| `t` | bigger / smaller chat at the bottom |
| `T` | chat in a column on the right, full height |
| `ctrl+y` / `ctrl+e`, wheel | scroll the chat |
| `P` | finish: first press previews the result, second writes it to `/tmp/guided-review/<id>/`, hands it to the agent and closes the viewer; the agent asks you in its chat and then publishes it to the MR |
| `?` | ask the agent about the line or selection (answer only, no comment); `enter` alone asks it to explain the code |
| `>` | next step |
| `S` | skip the step with a reason |
| `H`/`L`, click a step in the plan | look at an earlier/later step without losing progress; `esc` returns |
| `L` past the last step | `boilerplate`, `generated` and `all changes` views: every diff of the MR, nothing left out |
| `o`, click | open what is under the cursor: `⋯ N hidden lines` of unchanged code or a `▸ removed lines hidden` block; on a note or comment, fold it to one line and back |
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
