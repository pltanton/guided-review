# Simple viewer

People new to `gr view` find it overloaded: three columns, a tall header, a footer full
of hints, about sixty key bindings. The goal is a simple path a newcomer can follow with
`j/k`, `enter` and `esc`, while every existing key binding keeps working for experts.

Out of scope: the finish preview (`P`) and the threads screen (`R`).

## Layout: two columns

```
s3/7 ●●○○○○○ Transfers › validate amount           ? keys
──────────────────────────────────────┬──────────────────
 ⚑ retry with same key → 2nd charge?  │ FILES  s3
 transfer.go                          │ ● transfer.go
  55   if amt <= 0 {                  │   limits.go
+ 56     return ErrAmount             │ ─────────────────
  ┃ why not ErrInvalid?▏              │ agent: checks the
  ┃ comment · tab ask · enter send    │ sign before the…
+ 57   }                              │
  ✓ end of s3 · enter → next step     │ › enter to write
──────────────────────────────────────┴──────────────────
 next · comment · ask · skip
```

- **Header** is one line: step pill, `n/total`, progress dots, chapter › title, and a
  `may change` marker when `MayChange` is set. Viewing another step or an extra view
  keeps its "esc to return" line. Chapter intro, step `Note` and step-level hotspots
  leave the header.
- **Step-level hotspots and `Note`** become note rows above the first file, styled and
  folded like code annotations (`o` folds, `enter` opens details).
- **Right column**: the current step's files on top (current file marked, a click or
  `enter` in the list jumps to it, `f` focuses it), a rule, the chat below. The chat's
  "scroll · drag │ to resize" line goes; dragging the border still resizes.
- **No left column.** The plan is a modal (below). `view.hide_plan` goes; the config
  ignores unknown keys, so old files still load.
- **Narrow panes** (no room for the right column): chat at the bottom as today; the file
  list opens as a modal on `f`.
- **Footer**: buttons `next · comment · ask · skip` (`replies N` and `finish` when they
  apply as today) and `? keys`. The "mouse off" hint goes; the status line and cursor
  hints stay.

## Modals

One overlay component, drawn over the code and right column, header line left visible.
It has a bordered box, a title, a single-line hint in the bottom border, `esc` closes.
Used by:

- **Chapter intro.** Opens on entering the first step of a chapter whose intro this
  viewer process has not shown yet (kept in memory, not in state): chapter name, `Intro`,
  the chapter's steps with status. `enter` or `esc` closes. `I` or `:chapter` reopens it for the current step's chapter.
- **Plan.** `p` (and `ctrl+p`) open the whole plan: chapters, steps with status,
  boilerplate and extra views. `j/k` select, `enter` shows the step (same as today's plan
  panel preview), `esc` closes.
- **Keys.** `?` shows about a dozen essential keys; `?` again shows every action by
  group, as today's help.
- **LSP** and **note details** (see below).

## Enter as the main path

`enter` acts on what is under the cursor:

| Under the cursor                                  | `enter`                                  |
| ------------------------------------------------- | ---------------------------------------- |
| code line (or a `v` selection)                    | inline composer under the line/selection |
| own comment                                       | edit it in the inline composer           |
| agent note, step-level note or hotspot            | details modal (today's `i`)              |
| `⋯` hidden lines, `▸` folded block                | open (as today)                          |
| end-of-step row `✓ end of s3 · enter → next step` | next step (today's `>`)                  |

The end-of-step row is the last row of every step. `enter` only means "next" while the
cursor is on that row; nothing sends to the agent from `enter` alone anywhere else.

**Inline composer**: drawn as rows right under the anchored line, inside the code
column. Mode label and hints in its last row: `comment · tab ask · enter send ·
alt+enter new line · esc cancel`. `tab` cycles comment ↔ ask (in raw mode `tab` keeps
cycling severity, as today). In ask mode an empty `enter` asks to explain the line, as
today. Edit (`E`), raw (`ctrl+r`), reply-to-comment and thread replies use the same
composer. Messages without a line (`C`, the side prompt) and skip (`S`) keep composing in
the chat input.

## LSP in a modal

```
s3/7 ●●○○○○○ Transfers › validate amount
┌ references · Reserve (4) ─────────────────────────────────┐
│ /filter▏                                                  │
│ ▶ wallet/reserve.go:41   │  39 func (w *Wallet) Charge(…  │
│   wallet/charge.go:88    │  40   …                        │
│   api/transfer.go:120    │▶ 41   if err := w.Reserve(amt) │
│   api/transfer_test.go:9 │  42     return err             │
└──────────────────────────────────── enter open · esc close┘
```

- Replaces the popup in the bottom 3/5 of the code column; covers code and right column.
- Lists (references, implementations, callers, symbols, workspace symbols): list left,
  preview right, `j/k`/arrows select, `/` filters by substring, `enter` peeks.
- A single result opens the peek directly.
- Peek keeps today's keys (`w/b`, `g…`, `K`, `ctrl+o`, `tab`, `e`); `esc` closes the whole
  stack. The bottom border shows only `enter open · esc close`; `?` inside the modal lists
  the rest.
- Hover (`K`) is a small box next to the cursor line, not full screen.
- Entry points stay `g` (with its hint box), `K`, and the palette.

## Palette

`:` becomes a palette: typing filters actions by substring of name and description;
each row shows the action, its description and its key; `enter` runs the selected one.
Typed commands keep working when the input matches one (`:42`, `:s3`, `:f name`,
`:sym x`, `:set …`, `:msg …`, `:skip …`, `:q`, step ids). New command `:chapter`.

## Keys

No binding is removed or changed. `enter` gains the meanings above; `I` is new (chapter
intro). Existing overrides under `keys:` keep working.

## Testing

Model tests in `internal/view` for: header line content; hotspot/note rows above the
first file; end-of-step row and `enter` → next only on it; inline composer anchoring,
`tab` cycling, edit via `enter`; chapter modal shown once per chapter and reopened by
`I`; plan modal navigation; LSP modal (single result → peek, `/` filter, `esc` closes
the stack); palette filtering and running an action; typed commands unchanged. Then a
live run on a real MR.
