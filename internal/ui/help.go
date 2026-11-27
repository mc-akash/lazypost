package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/mc-akash/lazypost/internal/config"
)

const (
	helpTitle   = "Keybindings"
	helpKeyCol  = 14
	helpPadding = 2
)

type helpSection struct {
	title    string
	enter    func(Model) Model
	bindings []binding
}

func focusOn(p Panel) func(Model) Model {
	return func(m Model) Model { return m.setFocus(p) }
}

func helpSections() []helpSection {
	m := Model{}
	return []helpSection{
		{"Global", func(m Model) Model { return m }, m.globalBindings()},
		{"Environments", focusOn(PanelEnvs), m.envBindings()},
		{"Variables", Model.jumpToVars, m.varsBindings()},
		{"Collections", focusOn(PanelCollections), m.collectionsBindings()},
		{"Request", focusOn(PanelRequest), m.requestBindings()},
		{"Body editor", nil, bodyEditor{}.bindings()},
		{"Response", focusOn(PanelResponse), m.responseBindings()},
		{"History", focusOn(PanelHistory), m.historyBindings()},
	}
}

type helpEntry struct {
	section helpSection
	b       binding
}

func (e helpEntry) runnable() bool { return e.section.enter != nil }

func helpEntries(query string) []helpEntry {
	q := strings.ToLower(query)
	var out []helpEntry
	for _, s := range helpSections() {
		for _, b := range s.bindings {
			if b.help == "" {
				continue
			}
			text := strings.ToLower(s.title + "\n" + b.keyText() + "\n" + b.help)
			if strings.Contains(text, q) {
				out = append(out, helpEntry{s, b})
			}
		}
	}
	return out
}

type helpRow struct {
	line  string
	entry int
}

func helpRows(entries []helpEntry) []helpRow {
	var rows []helpRow
	for i, e := range entries {
		if i == 0 || entries[i-1].section.title != e.section.title {
			rows = append(rows, helpRow{line: e.section.title, entry: -1})
		}
		k := e.b.keyText()
		rows = append(rows, helpRow{line: k + strings.Repeat(" ", max(1, helpKeyCol-len(k))) + e.b.help, entry: i})
	}
	return rows
}

type helpScreen struct {
	query  string
	cursor int
}

func (hs *helpScreen) key(m Model, msg tea.KeyPressMsg) (Model, tea.Cmd) {
	if !config.IsText(msg.String()) {
		if next, cmd, ok := dispatch(m, msg.String(), hs.bindings()); ok || msg.Text == "" {
			return next, cmd
		}
	}
	next := *hs
	next.query, next.cursor = next.query+msg.Text, 0
	m.overlay = &next
	return m, nil
}

func (hs *helpScreen) bindings() []binding {
	n := len(helpEntries(hs.query))
	set := func(f func(helpScreen) helpScreen) func(Model) (Model, tea.Cmd) {
		return func(m Model) (Model, tea.Cmd) {
			next := f(*hs)
			m.overlay = &next
			return m, nil
		}
	}
	back := binding{keys: keys(esc, &km.Help), run: noCmd(Model.closeOverlay), hint: "close"}
	if hs.query != "" {
		back = binding{keys: keys(esc), run: set(func(helpScreen) helpScreen { return helpScreen{} }), hint: "clear search"}
	}
	return []binding{
		{keys: keys(&fixed.Type), hint: "search"},
		{keys: keys(enter), run: hs.run, hint: "run"},
		back,
		{keys: keys(&km.NextItem, &fixed.NextLine), run: set(func(h helpScreen) helpScreen { h.cursor = step(h.cursor, 1, n); return h })},
		{keys: keys(&km.PrevItem, &fixed.PrevLine), run: set(func(h helpScreen) helpScreen { h.cursor = step(h.cursor, -1, n); return h })},
		{keys: keys(&fixed.Backspace), run: set(func(h helpScreen) helpScreen {
			if r := []rune(h.query); len(r) > 0 {
				h.query, h.cursor = string(r[:len(r)-1]), 0
			}
			return h
		})},
	}
}

func (hs *helpScreen) run(m Model) (Model, tea.Cmd) {
	entries := helpEntries(hs.query)
	if len(entries) == 0 {
		return m, nil
	}
	e := entries[clampCursor(hs.cursor, len(entries))]
	if !e.runnable() {
		return m, nil
	}
	next, cmd := e.section.enter(m.closeOverlay()).handleKey((*e.b.keys[0])[0])
	return next.(Model), cmd
}

func (hs *helpScreen) hints(Model) []binding { return hinted(hs.bindings()) }

func (hs *helpScreen) draw(m Model, screen string) string {
	w, h := m.width, m.height
	all := helpRows(helpEntries(""))
	bw := helpPadding * 2
	for _, r := range all {
		bw = max(bw, boxPad+displayCells(r.line)+helpPadding)
	}
	bw = min(bw+2, w)
	bh := max(min(len(all)+3, h), 4)
	room := bw - 2 - boxPad
	indent := strings.Repeat(" ", helpPadding-boxPad)
	lines := []string{truncate(pickerPrompt+cleanText(hs.query), room)}
	entries := helpEntries(hs.query)
	rows := helpRows(entries)
	if len(rows) == 0 {
		lines = append(lines, mutedStyle.Render(truncate(pickerNoMatch, room)))
	}
	cursor := clampCursor(hs.cursor, len(entries))
	sel := 0
	for i, r := range rows {
		if r.entry == cursor {
			sel = i
		}
	}
	start, end := listWindow(len(rows), sel, bh-3)
	for _, r := range rows[start:end] {
		switch {
		case r.entry == cursor:
			lines = append(lines, selectedRow(indent+r.line, room, true))
		case r.entry < 0:
			lines = append(lines, focusTitle.Render(truncate(indent+r.line, room)))
		case !entries[r.entry].runnable():
			lines = append(lines, mutedStyle.Render(truncate(indent+r.line, room)))
		default:
			lines = append(lines, truncate(indent+r.line, room))
		}
	}
	return centered(screen, box(helpTitle, lines, bw, bh, true), w, h, bw, bh)
}
