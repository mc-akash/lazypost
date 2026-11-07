package ui

import "github.com/mc-akash/lazypost/internal/workspace"

const statusReloaded = "reloaded"

func (m Model) reload() Model {
	old := m.ws()
	ws, err := workspace.Load(old.Root)
	if err != nil {
		m = m.fail("reload failed: " + err.Error())
		return m
	}
	selPath := ""
	if row, ok := m.collections.selected(old.Collections); ok {
		selPath = row.path()
	}
	highlighted := m.envName
	if e := m.highlightedEnv(); e != nil {
		highlighted = e.Name
	}

	m.deps.WS = ws
	m.envName = pickActiveEnv(ws.Envs, m.envName)
	if indexOf(ws.Envs, highlighted) < 0 {
		highlighted = m.envName
	}
	m.environments = newEnvironmentPanel(ws.Envs, highlighted)
	m.collections = m.collections.reselect(ws.Collections, selPath)
	m, orphans, _ := m.restoreDrafts(m.drafts)

	status := statusReloaded
	if len(ws.LoadErrors) > 0 {
		status += " — " + loadErrorsStatus(ws)
	}
	m.status = joinStatus([]string{status, orphans})
	return m.persistSession()
}

func (p collectionPanel) reselect(root *workspace.Folder, path string) collectionPanel {
	p = p.reveal(root, path)
	if row, ok := p.selected(root); !ok || row.path() != path {
		p.cursor = 0
	}
	return p
}
