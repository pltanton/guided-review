# Installing guided-review

Written for a coding agent asked to "install guided-review from
https://github.com/pltanton/guided-review". Run the steps in order, check each one, and
report what was installed and what the human still has to do. Do not use `sudo` or change
shell profiles without asking.

## 1. Prerequisites

Check each; install the missing ones with the system package manager (`brew` on macOS,
`apt`/`dnf`/`pacman` on Linux) after telling the human what you are about to install.

| Tool      | Check              | Needed for                                    |
| --------- | ------------------ | --------------------------------------------- |
| Go ≥ 1.25 | `go version`       | building `gr`                                 |
| git       | `git --version`    | everything                                    |
| tmux      | `tmux -V`          | the viewer; reviews run inside a tmux session |
| jq        | `jq --version`     | the publish recipes in the skill              |
| glab      | `glab auth status` | GitLab MR title, discussions, publishing (recommended) |
| gh        | `gh auth status`   | GitHub PR title, discussions, publishing (recommended) |

`glab` and `gh` are optional: without them gr fetches the MR or PR from git and the review
result stays a local review.md. Recommend the one for the host the human reviews on. If it is
installed but not logged in, the human runs `glab auth login` or `gh auth login`
themselves (it is interactive); suggest they type `! glab auth login` in the prompt.

If `type gr` shows an alias (oh-my-zsh's git plugin maps `gr` to `git remote`), tell the
human: the skill copes with it, and they can run `unalias gr` in their shell config to use
`gr` by hand.

## 2. The `gr` CLI

```bash
go install github.com/pltanton/guided-review/cmd/gr@latest
command -v gr || echo "add $(go env GOPATH)/bin to PATH"
gr help | head -3
```

If `gr` is not found after installing, `$(go env GOPATH)/bin` is not on `PATH`: tell the
human which line to add to their shell profile rather than editing it yourself.

## 3. The skills

**Claude Code** — the plugin brings both skills (`guided-review`, `guided-selfreview`) and
the Stop hook that tells the viewer when the agent ends its turn:

```bash
claude plugin marketplace add pltanton/guided-review
claude plugin install guided-review@guided-review
claude plugin list | grep guided-review
```

The skills load in new sessions; tell the human to restart Claude Code.

**Codex** — clone the repository and link the skills:

```bash
git clone https://github.com/pltanton/guided-review ~/.local/share/guided-review
mkdir -p ~/.codex/skills
ln -sfn ~/.local/share/guided-review/skills/guided-review ~/.codex/skills/guided-review
ln -sfn ~/.local/share/guided-review/skills/guided-selfreview ~/.codex/skills/guided-selfreview
```

## 4. Optional: code navigation

The viewer's go-to-definition, references and call flow use a language server when one
is on `PATH`: `gopls` (Go), `kotlin-lsp` (Kotlin), `basedpyright-langserver` (Python),
`typescript-language-server` (TypeScript), `jdtls` (Java), `rust-analyzer` (Rust). Offer to install the
ones for the languages the human works in; nothing breaks without them.

## 5. Optional: Shift+Enter for a new line

Alt+Enter and Ctrl+J always start a new line in the viewer's input. Shift+Enter does too,
but tmux passes it on only with extended keys on; otherwise it arrives as plain Enter and
sends. Offer to add to `~/.tmux.conf`:

```
set -g extended-keys on
set -as terminal-features 'xterm*:extkeys'
```

The second line is needed when the outer terminal does not announce extended keys itself.
Reload with `tmux source-file ~/.tmux.conf`.

## 6. Done

Tell the human how to start: in tmux, inside a repository,

```
/guided-review <merge request or pull request URL>
```

and that `?` in the viewer lists every key.
