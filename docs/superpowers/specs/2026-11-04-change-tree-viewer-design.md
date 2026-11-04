# Change tree viewer

Date: 2026-11-04. Replaces the step walk.

## Problem

After live runs the reviewer does not know what they look at. A step shows a slice
of code without the whole change around it, the per-step explanation does not say
what to check, and the chrome (steps, chapters, plan modal, cards) is what they parse
instead of the code. The ⚑ flags are the part that works.

## Decision

The viewer shows the whole change as one diff with a file tree. The agent does not
gate the start: the viewer opens on the diff right after `gr init`, and the agent's
findings land in the tree while the reviewer reads. Guidance is a suggested reading
order, flags on lines, notes on tricky lines, one line per file and one briefing per
change. Nothing is enforced except the reviewer's own open MR threads before
publishing.

## Screen

Left: the diff of the whole change in one scroll. File headers between files, folded
removals, moved and whitespace marks, intraline colours, LSP peeks and the HERE line
stay as they are. Flags, notes, the reviewer's comments and MR threads hang under
their lines.

Right: the tree of files by path, and under it the chat block. A tree row shows the
file name, the order number when the agent set one, the flag count and a dot while
the file has unseen hunks. A file the agent has not read yet is dim. Boilerplate and
generated files are two folded groups at the bottom of the tree.

Top: one line with the MR title, the round and the agent status from `gr progress`.
The briefing, when it arrives, is a short block above the first file; a key opens it
again.

No step title, chapter, plan modal, intake screen, next or skip card.

## Start

`gr init` computes the diff and the tiers and opens the viewer. The tree is in path
order with no numbers and no flags. The viewer does not wait for the agent and works
with no agent at all.

## Moving

- `j`/`k`, counts, `/`, `:`, the LSP keys: unchanged.
- `]`/`[`: next and previous file in the agent's order; path order until it exists.
- `}`/`{`: next and previous flag.
- `n`/`N`: next and previous MR thread of the reviewer.
- `f`: focus the tree; enter on a row jumps to the file; `esc` back to the diff.
- `B`: open the briefing again.

Comments, ai comments, questions and the chat work as today.

## What the agent writes

One command, `gr plan set`, takes a partial plan and merges it: a file named in
`files` gets its `why` and `order` replaced, `hotspots` and `annotations` are appended,
`briefing` is replaced. The agent calls it several times per review.

```yaml
briefing: |-
  two to four lines: what the change is about and what to watch
files:
  - path: internal/transfer/limit.go
    order: 1
    why: daily limit moved from the handler into the domain
hotspots:
  - file: internal/transfer/limit.go
    line: 42
    cat: money
    q: the limit is read before the lock, can a parallel transfer pass twice?
    detail: |-
      longer reasoning with path:line references
annotations:
  - file: internal/transfer/limit.go
    line: 58
    kind: note
    text: ...
    detail: ...
```

`gr plan set` validates each path and line against the diff, rejects the bad entry
with its reason and keeps the rest. A hotspot or annotation at the same file, line and
kind replaces the earlier one, so the agent adds a `detail` later by sending the entry
again. A file the agent read with nothing to say gets `why` and no flags, and that
marks it read. `gr note add` and `gr note detail` go: an answer to a question is one
annotation through `gr plan set`.

Removed from the state file: `steps`, `carried`, `current`, and in them chapter,
intro, kind, message, why_big, depends_on, status, skip reason, may_change,
from_round. `Comment.Step` goes with them.

## Sizing, in the skill

The agent reads the stats `gr init` prints and picks a depth:

- Tiny (a few hunks in one or two files): no order, no file lines, no briefing. One
  read of the diff, flags if any, one line in the chat such as "3 lines, nothing to
  flag".
- Normal: briefing, file lines and order first, then flags and notes while reading,
  file by file, each written as soon as it is ready.
- Large: the same, with subagents reading groups of files in parallel and the main
  agent merging their output through `gr plan set`.

The reviewer overrides in words at any time ("go deep on the migration", "skip the
reading"). The skill loses everything about walking, skipping, stale steps after a
blocker, carried steps and `gr wait` between steps.

## Coverage and finishing

The viewer keeps marking hunks the reviewer scrolled past. `F` opens the finish card:
comments by severity, the derived verdict, unchecked flags, files with unseen hunks,
and "agent still reading N files" when it is. None of these block. An undecided MR
thread of the reviewer blocks publishing, as today. The summary posted to the MR lists
unread files and unchecked flags.

`gr prepare --partial` and carried steps go. The next round shows the round's diff
with the reviewer's comments and threads on their lines; files the reviewer did not
read in the previous round are dim in the tree.

## Rounds and threads

Unchanged in behaviour: the agent checks each reply against the new code first and
writes its take; a thread shows "agent is checking" until then. The threads are no
longer a separate step: they sit on their lines, `n`/`N` walk them, enter decides.

## Testing

- Unit and e2e tests in the repository follow the change: the plan merge and its
  validation, the tree order and dimming, `]`/`}`/`n` jumps, the finish card without
  steps, the next round without carried steps.
- Acceptance is a live run: a real daedalus MR with the reviewer, in the usual tmux
  session, with feedback recorded in the Obsidian run log. One tiny MR to check the
  tiny path and one normal MR to check that the tree fills in while reading.

## Out of scope

Ordering hunks inside a file; grouping the tree by behaviour; a viewer rewrite.
