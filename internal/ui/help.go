package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

const (
	helpTitle   = "Keybindings"
	helpKeyCol  = 14
	helpPadding = 2
)

type helpSection struct {
	title    string
	bindings []binding
}

func helpSections() []helpSection {
	m := Model{}
	return []helpSection{
		{"Global", m.globalBindings()},
		{"Environments", m.envBindings()},
		{"Variables", m.varsBindings()},
		{"Collections", m.collectionsBindings()},
		{"Request", m.requestBindings()},
		{"Body editor", bodyEditor{}.bindings()},
		{"Response", m.responseBindings()},
		{"History", m.historyBindings()},
	}
}

func helpLines(spaced bool) []string {
	var lines []string
	for i, s := range helpSections() {
		if i > 0 && spaced {
			lines = append(lines, "")
		}
		lines = append(lines, strings.Repeat(" ", helpPadding-boxPad)+focusTitle.Render(s.title))
		for _, b := range s.bindings {
			if b.help == "" {
				continue
			}
			k := b.keyText()
			key := k + strings.Repeat(" ", max(1, helpKeyCol-len(k)))
			lines = append(lines, strings.Repeat(" ", helpPadding-boxPad)+key+b.help)
		}
	}
	return lines
}

const helpMore = "↓ more"

func helpBody(h int) []string {
	if body := helpLines(true); len(body)+2 <= h {
		return body
	}
	return helpLines(false)
}

func helpMaxScroll(h int) int {
	return max(0, len(helpBody(h))-(h-2))
}

type helpScreen struct{ scroll int }

func (hs *helpScreen) key(m Model, msg tea.KeyPressMsg) (Model, tea.Cmd) {
	next, cmd, _ := dispatch(m, msg.String(), hs.bindings())
	return next, cmd
}

func (hs *helpScreen) bindings() []binding {
	scroll := func(to func(Model) int) func(Model) (Model, tea.Cmd) {
		return func(m Model) (Model, tea.Cmd) {
			m.overlay = &helpScreen{scroll: to(m)}
			return m, nil
		}
	}
	return []binding{
		{keys: keys(&km.Help, esc), run: noCmd(Model.closeOverlay), hint: "close"},
		{keys: keys(&km.Quit), run: Model.quit},
		{keys: keys(&km.NextItem), run: scroll(func(m Model) int { return min(hs.scroll+1, helpMaxScroll(m.height)) })},
		{keys: keys(&km.PrevItem), run: scroll(func(Model) int { return max(hs.scroll-1, 0) })},
		{keys: keys(&km.GotoTop), run: scroll(func(Model) int { return 0 })},
		{keys: keys(&km.GotoBottom), run: scroll(func(m Model) int { return helpMaxScroll(m.height) })},
	}
}

func (hs *helpScreen) hints(Model) []binding { return hinted(hs.bindings()) }

func (hs *helpScreen) draw(m Model, screen string) string {
	w, h := m.width, m.height
	body := helpBody(h)
	bw := helpPadding * 2
	for _, l := range body {
		bw = max(bw, boxPad+displayCells(l)+helpPadding)
	}
	bw = min(bw+2, w)
	rows := max(0, h-2)
	scroll := max(0, min(hs.scroll, len(body)-rows))
	visible := body[scroll:min(len(body), scroll+rows)]
	bh := len(visible) + 2
	fitted := make([]string, len(visible))
	for i, l := range visible {
		if room := bw - 2 - boxPad; displayCells(l) > room {
			l = lipgloss.NewStyle().MaxWidth(room).Render(l)
		}
		fitted[i] = l
	}
	if scroll+rows < len(body) && rows > 0 {
		fitted[rows-1] = strings.Repeat(" ", helpPadding-boxPad) + mutedStyle.Render(helpMore)
	}
	return centered(screen, box(helpTitle, fitted, bw, bh, true), w, h, bw, bh)
}
