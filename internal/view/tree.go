package view

import (
	"fmt"
	"path"

	"github.com/charmbracelet/x/ansi"
	"github.com/pltanton/guided-review/internal/state"
)

const sideContext = 3

type treeItem struct {
	file string
	tier state.Tier
}

func tierTitle(t state.Tier) string {
	return string(t)
}

// treeItems lists the tree in reading order: core files, then a row per folded tier with its
// files under it while the tier is open.
func (m *model) treeItems() []treeItem {
	if m.review == nil {
		return nil
	}
	var out []treeItem
	byTier := map[state.Tier][]treeItem{}
	for _, f := range m.review.FileOrder() {
		if f.Tier == state.TierCore {
			out = append(out, treeItem{file: f.Path, tier: f.Tier})
		} else {
			byTier[f.Tier] = append(byTier[f.Tier], treeItem{file: f.Path, tier: f.Tier})
		}
	}
	for _, t := range state.Tiers[1:] {
		if len(byTier[t]) == 0 {
			continue
		}
		out = append(out, treeItem{tier: t})
		if m.tierOpen[t] {
			out = append(out, byTier[t]...)
		}
	}
	return out
}

func (m *model) toggleTier(t state.Tier) {
	if m.tierOpen == nil {
		m.tierOpen = map[state.Tier]bool{}
	}
	m.tierOpen[t] = !m.tierOpen[t]
	if m.src != nil {
		m.rebuild(false)
	}
	m.seek(func(l line) bool { return l.Kind == RowFile && (l.FoldKey == string(t) || l.Tier == t) })
	m.clamp()
}

func (m *model) openTreeItem(it treeItem) {
	if it.file == "" {
		m.toggleTier(it.tier)
		return
	}
	m.jumpToFile(it.file)
	m.focusFiles = false
}

func (m *model) focusFilesPanel() {
	items := m.treeItems()
	if len(items) == 0 {
		return
	}
	m.focusFiles = true
	cur := m.current().File
	for i, it := range items {
		if it.file == cur {
			m.fileCursor = i
		}
	}
}

func (m *model) fileEntries(w int, row rowFunc, plain, selected func(string) string) ([]sideEntry, int) {
	items := m.treeItems()
	if m.focusFiles {
		m.fileCursor = max(0, min(m.fileCursor, len(items)-1))
	}
	current := m.current().File
	flags := map[string]int{}
	for _, h := range m.review.Hotspots {
		flags[h.File]++
	}
	_, unseen := m.changeCounts()
	var out []sideEntry
	at, prevDir := 0, ""
	for i, it := range items {
		var left, right string
		switch it.file {
		case "":
			glyph := "▸"
			if m.tierOpen[it.tier] {
				glyph = "▾"
			}
			n := 0
			for _, f := range m.review.Files {
				if f.Tier == it.tier {
					n++
				}
			}
			left, right = dimStyle.Render(glyph+" "+tierTitle(it.tier)), dimStyle.Render(fmt.Sprint(n))
			prevDir = ""
		default:
			dir, base := path.Dir(it.file), path.Base(it.file)
			indent := ""
			if dir != "." {
				if dir != prevDir {
					out = append(out, sideEntry{text: plain(row(dimStyle.Render(ansi.TruncateLeft(dir+"/", max(ansi.StringWidth(dir)-w+6, 0), "…")), "")), files: true})
				}
				indent = "  "
			}
			prevDir = dir
			f := m.review.File(it.file)
			style := textTone.fg()
			if f != nil && !f.Read && f.Tier == state.TierCore {
				style = dimStyle
				right = hotStyle.Render(m.spin())
				if m.agentWaiting || m.agentIdle {
					right = dimStyle.Render("○")
				}
			}
			if f != nil && f.Order > 0 {
				right = dimStyle.Render(fmt.Sprintf("%d", f.Order))
			}
			if n := flags[it.file]; n > 0 {
				right = hotStyle.Render(fmt.Sprintf("⚑%d", n)) + " " + right
			}
			if unseen[it.file] > 0 {
				right = accentTone.fg().Render("●") + " " + right
			}
			left = style.Render(indent + base)
			if m.focusFiles && i == m.fileCursor || !m.focusFiles && it.file == current {
				left = boldStyle.Render(indent + base)
			}
		}
		text := plain(row(left, right))
		if m.focusFiles && i == m.fileCursor || !m.focusFiles && it.file != "" && it.file == current {
			text = selected(row(left, right))
			at = len(out)
		}
		out = append(out, sideEntry{text: text, file: it.file, files: true})
	}
	return out, at
}

func (m *model) panelHint() string {
	k := m.keys()
	if m.focusFiles {
		return fmt.Sprintf("%s/%s move · %s open · %s back",
			k.key("down"), k.key("up"), k.key("open"), k.key("back"))
	}
	return ""
}

func follow(top, at, h, n int) int {
	ctx := max(min(sideContext, (h-1)/2), 0)
	top = max(min(top, at-ctx), at+ctx-h+1)
	return max(0, min(top, n-h))
}

func scrollWindow(top *int, last *string, key string, force bool, at, h, n int) int {
	if force || *last != key {
		*top, *last = follow(*top, at, h, n), key
	}
	*top = max(0, min(*top, n-h))
	return *top
}

func (m *model) sideWheel(x, y, d int) bool {
	cw := m.chatWidth()
	if cw == 0 || x < m.width-cw || m.review == nil {
		return false
	}
	side := m.sideFiles(m.height-len(m.bottomLines()), cw)
	if y < 0 || y >= len(side) || !side[y].files {
		return false
	}
	m.fileTop += d
	if m.focusFiles {
		m.fileCursor = max(0, min(m.fileCursor+d, len(m.treeItems())-1))
	}
	return true
}
