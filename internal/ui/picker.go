package ui

import (
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/mc-akash/lazypost/internal/config"
	"github.com/mc-akash/lazypost/internal/workspace"
)

const (
	envPickerTitle    = "Switch environment"
	methodPickerTitle = "Set method"
	authPickerTitle   = "Set auth type"
	authInherit       = "inherit"
	pickerPrompt      = "> "
	slashPrompt       = "/ "
	pickerMinWidth    = 30
	pickerNoMatch     = "no match"
)

type pickerMode int

const (
	typeToFilter pickerMode = iota

	slashFilter

	typeInput
)

type listPicker struct {
	title     string
	mode      pickerMode
	items     []string
	current   string
	pickHint  string
	onPick    func(Model, string) Model
	onTyped   func(Model, string) Model
	itemStyle func(string) lipgloss.Style

	filter    string
	filtering bool
	cursor    int
}

func newListPicker(title string, mode pickerMode, items []string, current, pickHint string, onPick func(Model, string) Model) *listPicker {
	return &listPicker{
		title: title, mode: mode, items: items, current: current, pickHint: pickHint, onPick: onPick,
		cursor: max(0, slices.Index(items, current)),
	}
}

func (p *listPicker) hints(Model) []binding { return hinted(p.bindings()) }

func (p listPicker) typing() bool { return p.mode == typeInput || p.filtering }

func (p listPicker) editsFilter() bool { return p.mode != slashFilter || p.filtering }

func (p listPicker) matches() []string {
	f := strings.ToLower(p.filter)
	var prefix, inner []string
	for _, it := range p.items {
		switch l := strings.ToLower(it); {
		case strings.HasPrefix(l, f):
			prefix = append(prefix, it)
		case strings.Contains(l, f):
			inner = append(inner, it)
		}
	}
	return append(prefix, inner...)
}

func (p *listPicker) key(m Model, msg tea.KeyPressMsg) (Model, tea.Cmd) {
	if !p.typing() || !config.IsText(msg.String()) {
		if next, cmd, ok := dispatch(m, msg.String(), p.bindings()); ok {
			return next, cmd
		}
	}
	next := *p
	if p.editsFilter() {
		next = next.typeText(msg.Text)
	}
	m.overlay = &next
	return m, nil
}

func (p listPicker) bindings() []binding {
	n := len(p.matches())
	set := func(f func(listPicker) listPicker) func(Model) (Model, tea.Cmd) {
		return func(m Model) (Model, tea.Cmd) {
			next := f(p)
			m.overlay = &next
			return m, nil
		}
	}
	filtering := func(on bool) func(Model) (Model, tea.Cmd) {
		return set(func(p listPicker) listPicker {
			p.filtering = on
			return p
		})
	}
	var bs []binding
	switch {
	case p.mode == typeInput:
		bs = []binding{{keys: keys(&fixed.Type), hint: "filter"}}
	case p.mode == slashFilter && !p.filtering:
		bs = []binding{{keys: keys(&km.Search), run: filtering(true), hint: "filter"}}
	}
	back := binding{keys: keys(esc), run: noCmd(Model.closeOverlay)}
	if p.filtering {
		back = binding{keys: keys(esc), run: filtering(false), hint: "stop filtering"}
	}
	down, up := keys(&km.NextItem), keys(&km.PrevItem)
	if p.mode == typeInput {
		down, up = keys(&km.NextItem, &fixed.NextLine), keys(&km.PrevItem, &fixed.PrevLine)
	}
	bs = append(bs,
		binding{keys: keys(enter), run: p.pick, hint: p.pickHint},
		back,
		binding{keys: down, run: set(func(p listPicker) listPicker { return p.move(1, n) })},
		binding{keys: up, run: set(func(p listPicker) listPicker { return p.move(-1, n) })},
		binding{keys: keys(&fixed.Backspace), run: set(listPicker.backspace)},
	)
	if p.mode != slashFilter || p.filtering {
		return bs
	}
	return append(bs,
		binding{keys: keys(&km.GotoTop, &km.GotoBottom), run: func(m Model) (Model, tea.Cmd) {
			next := p
			next.cursor = edge(n, is(m.lastKey, km.GotoBottom))
			m.overlay = &next
			return m, nil
		}},
	)
}

func (p listPicker) pick(m Model) (Model, tea.Cmd) {
	items := p.matches()
	if len(items) > 0 {
		m.overlay = nil
		return p.onPick(m, items[clampCursor(p.cursor, len(items))]), nil
	}
	if p.mode != typeInput {
		m.overlay = nil
		return m, nil
	}
	if typed := strings.TrimSpace(p.filter); typed != "" && p.onTyped != nil {
		m.overlay = &p
		return p.onTyped(m, typed), nil
	}
	return m, nil
}

func (p listPicker) move(delta, n int) listPicker {
	p.cursor = step(p.cursor, delta, n)
	return p
}

func (p listPicker) backspace() listPicker {
	if r := []rune(p.filter); len(r) > 0 && p.editsFilter() {
		p.filter, p.cursor = string(r[:len(r)-1]), 0
	}
	return p
}

func (p listPicker) typeText(text string) listPicker {
	if text != "" {
		p.filter, p.cursor = p.filter+text, 0
	}
	return p
}

func (p listPicker) filterLine() string {
	switch {
	case p.mode == typeToFilter, p.mode == typeInput:
		return pickerPrompt + cleanText(p.filter)
	case p.filtering || p.filter != "":
		return slashPrompt + cleanText(p.filter)
	}
	return ""
}

func (p listPicker) lines(w, h int) []string {
	items := p.matches()
	lines := []string{truncate(p.filterLine(), w)}
	if len(items) == 0 {
		return append(lines, mutedStyle.Render(truncate(pickerNoMatch, w)))
	}
	cursor := clampCursor(p.cursor, len(items))
	start, end := listWindow(len(items), cursor, h-1)
	for i := start; i < end; i++ {
		it := items[i]
		marker, markerStyle := "  ", plainStyle
		if it == p.current {
			marker, markerStyle = "● ", activeEnvStyle
		}
		if i == cursor {
			lines = append(lines, selectedRow(marker+it, w, true))
			continue
		}
		text := truncate(it, w-2)
		if p.itemStyle != nil {
			text = p.itemStyle(it).Render(text)
		}
		lines = append(lines, markerStyle.Render(marker)+text)
	}
	return lines
}

func (p *listPicker) draw(m Model, screen string) string {
	w, h := m.width, m.height
	bw := pickerMinWidth
	for _, it := range p.items {
		bw = max(bw, displayCells(cleanText(it))+len(pickerPrompt)+2+boxPad+2)
	}
	bw = min(bw, w)
	bh := min(len(p.matches())+3, h)
	bh = max(bh, 4)
	return centered(screen, box(p.title, p.lines(bw-2-boxPad, bh-2), bw, bh, true), w, h, bw, bh)
}

func (m Model) openEnvPicker() Model {
	names := make([]string, len(m.ws().Envs))
	for i, e := range m.ws().Envs {
		names[i] = e.Name
	}
	m.overlay = newListPicker(envPickerTitle, slashFilter, names, m.envName, "activate", func(m Model, name string) Model {
		m.environments.cursor = indexOf(m.ws().Envs, name)
		return m.activateEnv(name)
	})
	return m
}

func (m Model) openMethodPicker() Model {
	m, ok := m.requireEditable()
	if !ok {
		return m
	}
	tmpl := m.selectedRequest()
	p := newListPicker(methodPickerTitle, typeToFilter, httpMethods, strings.ToUpper(tmpl.Method), "set method", func(m Model, method string) Model {
		if tmpl := m.selectedRequest(); strings.EqualFold(tmpl.Method, method) {
			return m
		}
		return m.editRequest(func(r *workspace.Request) { r.Method = method })
	})
	p.itemStyle = methodStyle
	m.overlay = p
	return m
}

func authTypeLabel(t workspace.AuthType) string {
	if t == "" {
		return authInherit
	}
	return string(t)
}

func (m Model) openAuthPicker() Model {
	tmpl := m.selectedRequest()
	items := make([]string, len(authTypes))
	for i, t := range authTypes {
		items[i] = authTypeLabel(t)
	}
	m.overlay = newListPicker(authPickerTitle, typeToFilter, items, authTypeLabel(tmpl.Auth.Type), "set auth type", func(m Model, label string) Model {
		typ := workspace.AuthType(label)
		if label == authInherit {
			typ = ""
		}
		if tmpl := m.selectedRequest(); tmpl.Auth.Type == typ {
			return m
		}
		return m.editRequest(func(r *workspace.Request) { r.Auth.Type = typ })
	})
	return m
}
