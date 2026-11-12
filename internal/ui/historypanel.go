package ui

import (
	"fmt"
	"slices"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/mc-akash/lazypost/internal/history"
)

type historyPanel struct {
	entries []history.Entry
	cursor  int
}

func (m Model) historyMax() int {
	if m.deps.History == nil {
		return history.DefaultMax
	}
	return m.deps.History.Max()
}

func (p historyPanel) add(e history.Entry, selectNew bool, limit int) historyPanel {
	entries := make([]history.Entry, 0, min(len(p.entries)+1, limit))
	entries = append(entries, e)
	entries = append(entries, p.entries[:min(len(p.entries), limit-1)]...)
	cursor := 0
	if !selectNew && len(p.entries) > 0 {
		cursor = clampCursor(p.cursor+1, len(entries))
	}
	return historyPanel{entries: entries, cursor: cursor}
}

func (p historyPanel) move(delta int) historyPanel {
	p.cursor = step(p.cursor, delta, len(p.entries))
	return p
}

func (p historyPanel) selected() (history.Entry, bool) {
	if c := clampCursor(p.cursor, len(p.entries)); c < len(p.entries) {
		return p.entries[c], true
	}
	return history.Entry{}, false
}

func (p historyPanel) render(now time.Time, w, h int, focused bool) []string {
	cursor := clampCursor(p.cursor, len(p.entries))
	start, end := listWindow(len(p.entries), cursor, h)
	lines := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		e := p.entries[i]
		code, method := statusCode(e), methodLabel(e.Template.Method)
		rest := fmt.Sprintf(" %s  %s", entryName(e), age(now.Sub(e.Time)))
		if i == cursor {
			lines = append(lines, selectedRow(code+" "+method+rest, w, focused))
			continue
		}
		lead := len(code) + 1 + len(method)
		if w <= lead {
			lines = append(lines, truncate(code+" "+method, w))
			continue
		}
		lines = append(lines, statusStyle(e).Render(code)+" "+methodStyle(e.Template.Method).Render(method)+truncate(rest, w-lead))
	}
	return lines
}

func statusCode(e history.Entry) string {
	if e.Err != "" || e.Code == 0 {
		return "ERR"
	}
	return fmt.Sprintf("%3d", e.Code)
}

func entryName(e history.Entry) string {
	if e.Template.Name != "" {
		return e.Template.Name
	}
	return e.Sent.URL
}

func age(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", max(0, int(d.Seconds())))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}

func (Model) historyBindings() []binding {
	return []binding{
		{keys: keys(enter), run: noCmd(func(m Model) Model {
			if _, ok := m.history.selected(); ok {
				m.onScratch = false
				return m.setFocus(PanelResponse)
			}
			return m
		}), help: "open response", hint: "response"},
		{keys: keys(&km.Send), run: Model.send, help: "resend with current variables", hint: "resend"},
	}
}

type historyLoadedMsg struct {
	entries []history.Entry
	err     error
}

func loadHistoryCmd(s *history.Store, root string) tea.Cmd {
	return func() tea.Msg {
		historyMu.Lock()
		entries, err := s.Load()
		historyMu.Unlock()
		own := slices.DeleteFunc(entries, func(e history.Entry) bool { return e.Root != root })
		return historyLoadedMsg{entries: own, err: err}
	}
}

func (m Model) handleHistoryLoaded(msg historyLoadedMsg) Model {
	if msg.err != nil {
		m = m.fail("history: " + msg.err.Error())
	}
	m.history = m.history.merge(msg.entries, m.historyMax())
	return m
}

func (p historyPanel) merge(loaded []history.Entry, limit int) historyPanel {
	seen := make(map[string]bool, len(p.entries))
	for _, e := range p.entries {
		if e.ID != "" {
			seen[e.ID] = true
		}
	}
	entries := slices.Clone(p.entries)
	for _, e := range loaded {
		if len(entries) >= limit {
			break
		}
		if e.ID == "" || !seen[e.ID] {
			entries = append(entries, e)
		}
	}
	return historyPanel{entries: entries, cursor: p.cursor}
}
