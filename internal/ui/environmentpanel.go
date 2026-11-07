package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/mc-akash/lazypost/internal/workspace"
)

type environmentPanel struct {
	cursor int
}

func newEnvironmentPanel(envs []*workspace.Env, active string) environmentPanel {
	for i, e := range envs {
		if e.Name == active {
			return environmentPanel{cursor: i}
		}
	}
	return environmentPanel{}
}

func (p environmentPanel) move(delta, n int) environmentPanel {
	p.cursor = step(p.cursor, delta, n)
	return p
}

func (p environmentPanel) selected(envs []*workspace.Env) *workspace.Env {
	if c := clampCursor(p.cursor, len(envs)); c < len(envs) {
		return envs[c]
	}
	return nil
}

func (p environmentPanel) render(envs []*workspace.Env, active string, w, h int, focused bool) []string {
	cursor := clampCursor(p.cursor, len(envs))
	start, end := listWindow(len(envs), cursor, h)
	lines := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		name := envs[i].Name
		marker, markerStyle := "  ", plainStyle
		if name == active {
			marker, markerStyle = "● ", activeEnvStyle
		}
		if i == cursor {
			lines = append(lines, selectedRow(marker+name, w, focused))
			continue
		}
		lines = append(lines, markerStyle.Render(marker)+truncate(name, w-2))
	}
	return lines
}

func indexOf(envs []*workspace.Env, name string) int {
	for i, e := range envs {
		if e.Name == name {
			return i
		}
	}
	return -1
}

func (Model) envBindings() []binding {
	return []binding{
		{keys: keys(space), run: noCmd(func(m Model) Model {
			if e := m.highlightedEnv(); e != nil {
				return m.activateEnv(e.Name)
			}
			return m
		}), help: "activate environment", hint: "activate"},
		{keys: keys(enter), run: noCmd(Model.focusVars), help: "variables table", hint: "variables"},
		{keys: keys(&km.New), run: noCmd(func(m Model) Model {
			in := newInput(dialogNewEnv, "", submitNewEnv)
			m.overlay = &in
			return m
		}), hint: "new"},
		{keys: keys(&km.Delete), run: noCmd(func(m Model) Model {
			if e := m.highlightedEnv(); e != nil {
				return m.confirmDelete(dialogDelEnv, e.Name, func(m Model) Model { return m.deleteEnv(e) })
			}
			return m
		}), hint: "delete"},
		{keys: keys(&km.New, &km.Delete), help: "new / delete environment"},
	}
}

func (m Model) highlightedEnv() *workspace.Env {
	return m.environments.selected(m.ws().Envs)
}

func (m Model) activateEnv(name string) Model {
	m.envName = name
	return m.persistSession()
}

func submitNewEnv(m Model, value string) (Model, tea.Cmd) {
	name := strings.TrimSpace(value)
	if name == "" {
		return m, nil
	}
	e, err := m.ws().CreateEnv(name)
	if err != nil {
		m = m.fail(err.Error())
		return m, nil
	}
	m.environments.cursor = indexOf(m.ws().Envs, e.Name)
	m.status = "created " + e.Name
	if m.activeEnv() == nil {
		m = m.activateEnv(e.Name)
	}
	return m, nil
}

func (m Model) deleteEnv(e *workspace.Env) Model {
	if err := m.ws().DeleteEnv(e); err != nil {
		m = m.fail("delete failed: " + err.Error())
		return m
	}
	m.environments = m.environments.move(0, len(m.ws().Envs))
	m.status = "deleted " + e.Name
	if m.envName == e.Name {
		m = m.activateEnv(pickActiveEnv(m.ws().Envs, ""))
	}
	return m
}

func (m Model) focusVars() Model {
	if m.focus != PanelVars {
		m.prevFocus = m.focus
	}
	m.focus = PanelVars
	return m
}

func (m Model) jumpToVars() Model {
	m.environments = newEnvironmentPanel(m.ws().Envs, m.envName)
	return m.focusVars()
}
