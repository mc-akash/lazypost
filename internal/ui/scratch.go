package ui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/mc-akash/lazypost/internal/workspace"
)

const (
	scratchName     = "unsaved"
	scratchKey      = "scratch\x00"
	reqTitleScratch = reqTitle + " · unsaved"

	dialogReplaceScratch = "Replace unsaved request"
	msgReplaceScratch    = "Overwrite it with a copy of this history entry?"
	msgImportScratch     = "Overwrite it with the imported cURL request?"
	statusScratchKept    = "kept the unsaved request"

	statusScratchImported = "imported into unsaved request"
	statusScratchNoEnv    = statusScratchImported + ": no environment for secrets"
)

func (m Model) scratchSelected() bool {
	if m.scratch == nil {
		return false
	}
	if m.onScratch {
		return true
	}
	_, row := m.rowSelection()
	return !row
}

func (m Model) openScratch() Model {
	created := m.scratch == nil
	m = m.selectScratch()
	m = m.setFocus(PanelRequest)
	if created {
		m = m.editURL()
	}
	return m
}

func (m Model) selectScratch() Model {
	if m.scratch == nil {
		m = m.storeScratch(workspace.Request{Method: "GET"})
	}
	m.onScratch = true
	return m
}

func (m Model) editHistoryCopy(tmpl workspace.Request) (Model, bool) {
	if m.scratch == nil || sameContent(*m.scratch, tmpl) {
		m = m.storeScratch(tmpl)
		m.onScratch = true
		return m, true
	}
	key := m.lastKey
	m.overlay = confirm(dialogReplaceScratch, msgReplaceScratch, statusScratchKept, func(m Model) (Model, tea.Cmd) {
		m = m.storeScratch(tmpl)
		m.onScratch = true
		next, cmd := m.handleKey(key)
		return next.(Model), cmd
	})
	return m, false
}

func sameContent(a, b workspace.Request) bool {
	a, b = cloneRequest(a), cloneRequest(b)
	a.Name, a.Path, a.Parent = "", "", nil
	b.Name, b.Path, b.Parent = "", "", nil
	return sameRequest(a, b)
}

func (m Model) storeScratch(req workspace.Request) Model {
	r := cloneRequest(req)
	r.Name, r.Path, r.Parent = scratchName, "", nil
	m.scratch = &r
	return m.persistSession()
}

func (m Model) dropScratch() Model {
	m.scratch, m.onScratch = nil, false
	return m
}

func (m Model) restoreScratch(s *workspace.Request) Model {
	if s == nil {
		return m
	}
	r := cloneRequest(*s)
	r.Name, r.Path, r.Parent = scratchName, "", nil
	m.scratch = &r
	return m
}

func (m Model) saveScratchAs() Model {
	in := newInput(dialogSaveAs, "", func(m Model, v string) (Model, tea.Cmd) {
		return m.submitScratchSaveAs(v), nil
	}).withHint(hintSaveAs)
	m.overlay = &in
	return m
}

func (m Model) submitScratchSaveAs(value string) Model {
	folders, name := splitSaveAs(value)
	if name == "" || m.scratch == nil || m.ws().Collections == nil {
		m.status = statusSaveAsCancel
		return m
	}
	created, note, err := m.saveNewRequest(folders, name, *m.scratch)
	if err != nil {
		m = m.fail(saveFailStatus("save as failed: ", err))
		return m
	}
	m = m.dropScratch()
	m.lastLeft = PanelCollections
	m.collections = m.collections.reveal(m.ws().Collections, created.Path)
	m.status = "saved as " + m.displayPath(created) + note
	return m.persistSession()
}

func (m Model) importIntoScratch(imported workspace.Request) Model {
	if m.scratch == nil {
		return m.replaceScratch(imported)
	}
	m.overlay = confirm(dialogReplaceScratch, msgImportScratch, statusScratchKept, func(m Model) (Model, tea.Cmd) {
		return m.replaceScratch(imported), nil
	})
	return m
}

func (m Model) replaceScratch(imported workspace.Request) Model {
	name := imported.Name
	if name == "" {
		name = scratchName
	}
	status := statusScratchImported
	if env := m.activeEnv(); env != nil {
		withRefs, note, err := m.moveImportSecrets(imported, name, env)
		if err != nil {
			m = m.fail("save failed: " + err.Error())
			return m
		}
		imported, status = withRefs, status+note
	} else if hasImportSecrets(imported, name) {
		status = statusScratchNoEnv
	}
	m.onScratch = true
	m = m.setFocus(PanelRequest)
	m.status = status
	return m.storeScratch(imported)
}
