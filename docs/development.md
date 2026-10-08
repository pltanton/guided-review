# Developing guided-review

## The shell

`nix develop`, or `direnv allow` once with the `.envrc` in the repository, gives Go,
gopls, golangci-lint, staticcheck, goimports, errcheck, git, tmux, jq, glab and gh — the
tools the checks below and a live review need. Without Nix any Go from `go.mod` up
works, with the linters installed by hand.

The checks before a commit:

```bash
gofmt -l . && go vet ./... && go test -race ./... && golangci-lint run
go fix -diff ./...   # stale forms; empty means clean
nix flake check      # the package builds and its tests pass in the sandbox
```

## Trying a change in real reviews

`gr` and the skills come from two places, and each can point at the working copy:

- **`gr`**: `go install ./cmd/gr` puts the build in `$(go env GOPATH)/bin`. Keep that
  directory before the Nix profile on `PATH`; the installed `gr` then wins until
  `rm "$(go env GOPATH)/bin/gr"` hands back the packaged one. An open viewer keeps the
  old binary: `Q` and `gr open`, or respawn its tmux pane.
- **The skills**: with the home-manager module, set
  `programs.guided-review.checkout` to the working copy. `~/.claude/skills` and
  `~/.codex/skills` then link into it, so an edit to a skill applies on the agent's next
  start without a switch. Without the module, symlink `skills/guided-review` and
  `skills/guided-selfreview` into those directories by hand.

To build the package from the working copy rather than GitHub, point the configuration's
input at it for a while: `url = "path:/path/to/guided-review";`, switch, and put the
GitHub URL back before committing the configuration.

## Changing dependencies

`nix/package.nix` pins `vendorHash`. After a change to `go.mod` the Nix build fails with
the expected hash: replace `vendorHash` with the `got:` value it prints. CI runs
`nix flake check` and `nix build` on every push and catches a stale hash.

## Releasing

Bump `.claude-plugin/plugin.json` and `Version` in `guidedreview.go` together (a test
checks they match), add the section to `CHANGELOG.md` — the viewer shows it once after
the update — and publish a GitHub release with the tag `v<version>`; the site rebuilds
its recordings from the newest tag.
