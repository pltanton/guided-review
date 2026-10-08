# Simple viewer Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement `docs/superpowers/specs/2026-10-08-simple-viewer-design.md`: two columns,
one-line header, modals, `enter` as the main path with an inline composer, `:` palette, LSP
in a modal — without removing any key binding.

**Architecture:** All work is in `internal/view` (bubbletea model). A new `modal.go` draws a
bordered box over the body (everything between the header line and the footer); help, plan,
chapter intro, files (narrow panes), LSP and details render through it. The left plan column
goes away; the right column becomes files (+ flow) over chat. A synthetic end-of-step line
and step-level note rows are added in `relist`/`buildRows`. `enter` gets its own action
(`act`) that dispatches on the row under the cursor.

**Tech Stack:** Go, bubbletea, lipgloss, charmbracelet/x/ansi. Tests: `go test ./internal/view/`.

Gate before every commit: `go build ./... && go vet ./... && go test -race ./...` — check the
exit code, never pipe it into `head`.

---

### Task 1: Modal component

**Files:** Create `internal/view/modal.go`, `internal/view/modal_test.go`.

- [ ] Test `TestModalBox`: `modalBox(40, 8, "plan", "esc close", []string{"a","b"})` returns
      8 lines, each of display width 40; first line starts with `┌` and contains `plan`; last
      line starts with `└` and ends with `esc close┘`; body lines start with `│ ` and end `│`;
      body padded to 6 rows, extra body lines cut.
- [ ] Test `TestOverlayBody`: `overlay(base, box, x)` replaces rows `[0, len(box))` of base
      starting at column x, keeping the base text left of x.
- [ ] Implement:

```go
func modalBox(w, h int, title, hint string, body []string) []string
func overlayAt(base, box []string, top, x int)
```

`modalBox` uses `faintTone` for the frame, `boldStyle` for the title, `dimStyle` for the
hint; body lines go through `fit(line, w-4)`.

- [ ] Run tests, gate, commit `view: modal box`.

### Task 2: Help as a two-level modal

**Files:** Modify `keymap.go` (helpLines, handleHelpKey), `layout.go` (View help branch),
`model.go` (`helpAll bool`), tests in `model_test.go`.

- [ ] Test `TestHelpEssentials`: `?` opens help with `helpAll == false`; the rendered view
      contains `essentials` and the keys of `next`, `act`, `ask`, `skip`, `details`, `finish`,
      `command`, `quit`, and not `diff-algorithm`'s description; `?` again sets `helpAll`, view
      contains `next diff algorithm`; any other key closes.
- [ ] Implement `essentialActions = []string{"down","up","next-hunk","prev-hunk","act",
"ask","next","skip","details","plan","finish","command","quit"}`; `helpLines(width)` takes
      `all bool`; the help is drawn with `modalBox` over the body; `?` while help is open toggles
      `helpAll`.
- [ ] Gate, commit `view: ? shows essentials, ? again all keys, in a modal`.

### Task 3: One-line header

**Files:** Modify `layout.go` (`header`, `stepTitle`), tests.

- [ ] Test `TestHeaderOneLine`: a step with `Intro`, `Note`, a line-0 hotspot and
      `MayChange` gives `header()` of 2 lines (title + separator); the title contains
      `s1/2`, the chapter, `›`, the title and `may change`; it contains neither the intro, the
      note nor the hotspot text. Viewing another step still adds the `esc to return` line.
- [ ] Update `TestChapterIntro` (it asserted the intro in the header) to assert it is not.
- [ ] Implement: title = pill + `n/total` + progress dots (`●` reviewed, `○` pending, at
      most 20, else the bar as today) + `chapter › title`, then status/round/filling/seen
      markers as today, `may change` in `delStyle`.
- [ ] Gate, commit `view: one-line header`.

### Task 4: Step-level notes as rows above the first file

**Files:** Modify `rows.go` (`Note.Top`, `buildRows`), `model.go` (`notes`), tests in
`rows_test.go`, `model_test.go`.

- [ ] Test `TestTopNotes` (rows_test): `buildRows` with a `Note{Top: true, Kind: "hotspot",
Text: "q?"}` puts a `RowNote` with `NoteKind "hotspot"`, `Line 0`, `File` = first file,
      before the first `RowFile`.
- [ ] Implement: `notes()` adds `Top` notes for `st.Note` (kind `note`) and hotspots with
      `Line == 0` (kind `hotspot`); `buildRows` skips `Top` notes in the per-file filter and
      prepends them before the first file.
- [ ] `enter`/`i` on a top note (Line 0) opens the details modal with the note's full text
      (no agent request — there is no detail slot for line 0).
- [ ] Gate, commit `view: step note and risk questions as rows above the code`.

### Task 5: `act` on enter and the end-of-step row

**Files:** Modify `rows.go` (`RowEnd`), `model.go` (`relist` appends it), `layout.go`
(render), `keymap.go` (new `act` action on `enter`; `message` keeps `c`), `wrap.go` if
needed, tests.

- [ ] Test `TestEndRow`: the last line is `RowEnd`, rendered with `end of s1` and `next
step`; `G` then `enter` emits `KindNext`; `enter` on a code line does not emit.
- [ ] Test `TestEnterDispatch`: enter on a code line starts composing a message anchored
      there; on own comment (Ref>0) starts an edit; on an agent note opens the details modal; on
      `⋯` reveals lines; in a viewed earlier step the end row says `esc to return` and enter
      returns.
- [ ] Update `TestEventsFromKeys` and others that used `enter` for message only where the
      cursor sits on something else now.
- [ ] Gate, commit `view: enter acts on the row under the cursor; end-of-step row`.

### Task 6: Inline composer

**Files:** Modify `input.go` (`composeAt`), `layout.go` (View inserts composer rows,
`sidePrompt`, `bottomLines`, `inputInSideChat`), `wrap.go` (`lineHeight` counts composer
rows), tests.

- [ ] Test `TestInlineComposer`: after enter on line i, the rendered view has the input bar
      row right after line i's row and the hint `tab ask`; `tab` switches to ask and the hint
      says `ask`; `tab` again back to comment; raw mode `tab` still cycles severity. `C` and `S`
      still compose in the chat prompt.
- [ ] Implement: `m.composeAt` = index of the last anchored line (selection end or cursor)
      when composing a message/ask/edit/raw with a file anchor, else -1. View renders
      `inputLines` + `composeStatus` (with `tab ask`/`tab comment` hint) under that line inside
      the code column; `lineHeight(composeAt)` adds those rows so scrolling keeps it visible.
- [ ] Gate, commit `view: compose under the line`.

### Task 7: Two columns — files over chat on the right

**Files:** Modify `layout.go` (`planWidth` removed, `mainWidth`, `sideChatLines`,
`sidebar` → `sideFiles`), `mouse.go` (clicks/wheel on the right column, no plan resize),
`plan.go` (`sideWheel`), `keymap.go` (`files` focuses the right column), tests
(`plan_test.go`, `model_test.go`, `wrap_test.go`).

- [ ] Test `TestRightColumn`: with width 160 the view has no `PLAN` label, has `FILES` and
      the chat in the right column; the current file is marked; clicking a file row jumps to it;
      `f` then `j` then `enter` jumps to the second file. Narrow width (no right column): `f`
      opens a files modal.
- [ ] Implement; the flow block (`FLOW`) stays under the files, the files+flow block is
      capped at half the column height. Drop the chat's "scroll · drag" line.
- [ ] Gate, commit `view: two columns, step files above the chat`.

### Task 8: Plan modal

**Files:** Modify `plan.go` (`focusPlanPanel` opens the modal), `layout.go` (render plan
entries in a modal), `keymap.go` (`plan` = `p` and `steps` = `ctrl+p` both open it),
tests.

- [ ] Test `TestPlanModal`: `p` opens it, contains every chapter and step; `j` and `enter`
      on a step shows that step and closes; `enter` on a chapter folds it; `esc` closes.
- [ ] Gate, commit `view: plan as a modal`.

### Task 9: Chapter intro modal

**Files:** Modify `model.go` (`introShown map[string]bool`, open on step change), `keymap.go`
(`chapter` action on `I`), `command.go` (`:chapter`), tests.

- [ ] Test `TestChapterModal`: showing the first step of a chapter with an intro opens the
      modal (title = chapter, body has the intro and the chapter's steps); `enter` closes;
      showing a second step of the same chapter does not reopen; `I` reopens; `:chapter`
      reopens; a chapter without intro never opens.
- [ ] Gate, commit `view: chapter intro in a modal when the chapter starts`.

### Task 10: LSP and details in the modal

**Files:** Modify `lspview.go` (`popupLines` hint, `/` filter), `layout.go` (draw popup via
`modalBox` over body and right column; hover as a small box near the cursor), `mouse.go`,
tests.

- [ ] Test `TestLSPModal`: a references result opens a modal whose bottom border has `enter
open · esc close`; `/` + `chg` + enter filters items to those whose path/text contain it;
      `esc` with a peek stacked on the list closes everything; hover renders a box of at most
      half the body height.
- [ ] Gate, commit `view: LSP results in a full modal, filter with /`.

### Task 11: Palette

**Files:** Modify `command.go` (`paletteItems`, enter runs the selection when the input is
not a known command), `layout.go` (list above the `:` prompt), tests.

- [ ] Test `TestPalette`: `:` + `diff` lists `diff-algorithm` with its key `d`; enter runs
      it (algo changes); `:42` still jumps; `:s2` still shows s2; up/down move the selection.
- [ ] Gate, commit `view: : is a palette of actions`.

### Task 12: Footer, docs, live check

**Files:** `layout.go` (footer label `comment`, no `mouse off`, `? keys`), `docs/guide.md`,
`README.md` if it lists keys, skill text if it names viewer keys.

- [ ] Test: footer contains `comment` and `? keys`, not `mouse off`.
- [ ] Update docs; gate; commit `docs: simple viewer`.
- [ ] `go install ./cmd/gr`, respawn open `gr view` panes, look at a real review.
