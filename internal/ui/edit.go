package ui

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/mc-akash/lazypost/internal/state"
	"github.com/mc-akash/lazypost/internal/workspace"
)

const (
	markDraft = "●"
	markStale = "!"
)

const (
	statusSaved       = "saved"
	statusNoDraft     = "nothing to save"
	statusNotEditable = "select a request in Collections to edit it"
)

func (m Model) draftKey(path string) string { return relPath(m.ws().Root, path) }

func (m Model) editTarget() (*workspace.Request, bool) {
	if m.lastLeft != PanelCollections || m.scratchSelected() {
		return nil, false
	}
	row, ok := m.collections.selected(m.ws().Collections)
	if !ok || row.req == nil {
		return nil, false
	}
	return row.req, true
}

func (m Model) requireEditable() (Model, bool) {
	if _, _, ok := m.editable(); ok {
		return m, true
	}
	sel, selected := m.selection()
	if m.focus == PanelRequest {
		switch {
		case !selected:

			return m.selectScratch(), true
		case sel.entry != nil:

			return m.editHistoryCopy(sel.tmpl)
		}
	}
	m.status = statusNotEditable
	return m, false
}

func (m Model) editable() (req workspace.Request, file *workspace.Request, ok bool) {
	if m.scratchSelected() {
		return cloneRequest(*m.scratch), nil, true
	}
	file, ok = m.editTarget()
	if !ok {
		return workspace.Request{}, nil, false
	}
	return m.effective(file), file, true
}

func (m Model) storeEdit(file *workspace.Request, req workspace.Request) Model {
	if file == nil {
		return m.storeScratch(req)
	}
	return m.storeDraft(file, req)
}

func (m Model) effective(file *workspace.Request) workspace.Request {
	r := *file
	if d, ok := m.drafts[m.draftKey(file.Path)]; ok {
		r = d.Request
		r.Path, r.Parent = file.Path, file.Parent
	}
	return cloneRequest(r)
}

func cloneRequest(r workspace.Request) workspace.Request {
	r.Headers = slices.Clone(r.Headers)
	r.Params = slices.Clone(r.Params)
	r.Body.Form = slices.Clone(r.Body.Form)
	r.Body.Parts = slices.Clone(r.Body.Parts)
	return r
}

func (m Model) editRequest(fn func(*workspace.Request)) Model {
	req, file, ok := m.editable()
	if !ok {
		m.status = statusNotEditable
		return m
	}
	fn(&req)
	return m.storeEdit(file, req)
}

func (m Model) storeDraft(file *workspace.Request, req workspace.Request) Model {
	key := m.draftKey(file.Path)
	d, ok := m.drafts[key]
	if !ok {
		d.BaseModTime = file.ModTime
	}
	req.Path, req.Parent, req.ModTime = "", nil, time.Time{}
	d.Request = req
	drafts := make(map[string]state.Draft, len(m.drafts)+1)
	maps.Copy(drafts, m.drafts)
	drafts[key] = d
	m.drafts = drafts
	return m.persistSession()
}

func fileModTime(path string) (t time.Time) {
	if fi, err := os.Stat(path); err == nil {
		t = fi.ModTime()
	}
	return t
}

func (m Model) dropDrafts(match func(key string) bool) Model {
	drafts := maps.Clone(m.drafts)
	maps.DeleteFunc(drafts, func(k string, _ state.Draft) bool { return match(k) })
	stale := maps.Clone(m.stale)
	maps.DeleteFunc(stale, func(k string, _ bool) bool { return match(k) })
	m.drafts, m.stale = drafts, stale
	return m
}

func (m Model) dropDraft(key string) Model {
	return m.dropDrafts(func(k string) bool { return k == key })
}

func (m Model) treeDecor(r *workspace.Request) (method, mark string) {
	method = r.Method
	if d, ok := m.drafts[m.draftKey(r.Path)]; ok {
		method = d.Request.Method
	}
	return method, m.draftMark(r.Path)
}

func (m Model) draftMark(path string) string {
	key := m.draftKey(path)
	if _, ok := m.drafts[key]; !ok {
		return ""
	}
	if m.stale[key] {
		return markStale
	}
	return markDraft
}

func (m Model) writeDraft() Model {
	if m.scratchSelected() {
		return m.saveScratchAs()
	}
	file, ok := m.editTarget()
	if !ok {
		m.status = statusNotEditable
		return m
	}
	key := m.draftKey(file.Path)
	d, ok := m.drafts[key]
	if !ok {
		m.status = statusNoDraft
		return m
	}
	if !m.stale[key] && !fileModTime(file.Path).Equal(d.BaseModTime) {
		stale := make(map[string]bool, len(m.stale)+1)
		maps.Copy(stale, m.stale)
		stale[key] = true
		m.stale = stale
		m.status = changedOnDiskStatus([]string{file.Name})
		return m
	}
	req, note, err := m.routeSaveSecrets(m.effective(file), file.Name)
	if err == nil {
		err = m.ws().SaveRequest(&req)
	}
	if err != nil {
		m = m.fail(saveFailStatus("save failed: ", err))
		return m
	}
	*file = req
	m = m.dropDraft(key)
	m.status = statusSaved + note
	return m.persistSession()
}

func (m Model) session() state.Session {
	last := m.deps.Session.LastRequest
	if row, ok := m.collections.selected(m.ws().Collections); ok && row.req != nil {
		last = m.draftKey(row.req.Path)
	}
	drafts := m.drafts
	if drafts == nil {
		drafts = map[string]state.Draft{}
	}
	return state.Session{LastEnv: m.envName, LastRequest: last, Drafts: drafts, Scratch: m.scratch}
}

func (m Model) saveSession() (Model, error) {
	m.deps.Session = m.session()
	return m, m.deps.StateStore.Save(m.ws().Root, m.deps.Session)
}

func (m Model) persistSession() Model {
	m, err := m.saveSession()
	if err != nil {
		m = m.fail("session: " + err.Error())
	}
	return m
}

func (m Model) quit() (Model, tea.Cmd) {
	m, _ = m.saveSession()
	return m, tea.Quit
}

func (m Model) restoreSession(s state.Session) (Model, []string) {
	m, dropped, changed := m.restoreDrafts(s.Drafts)
	m = m.restoreScratch(s.Scratch)
	if s.LastRequest != "" {
		m.collections = m.collections.reveal(m.ws().Collections, filepath.Join(m.ws().Root, s.LastRequest))
	}
	var msgs []string
	if dropped != "" {
		msgs = append(msgs, dropped)
	}
	if len(changed) > 0 {
		sort.Strings(changed)
		msgs = append(msgs, changedOnDiskStatus(changed))
	}
	return m, msgs
}

func changedOnDiskStatus(names []string) string {
	return fmt.Sprintf("changed on disk: %s (%s overwrites)", strings.Join(names, ", "), km.Save[0])
}

func (m Model) restoreDrafts(src map[string]state.Draft) (Model, string, []string) {
	root := m.ws().Root
	drafts := map[string]state.Draft{}
	stale := map[string]bool{}
	var changed []string
	dropped := 0
	for _, key := range slices.Sorted(maps.Keys(src)) {
		d := src[key]
		path := filepath.Join(root, key)
		file := m.ws().FindRequest(path)
		if file == nil {
			if m.failedToLoad(path) {
				drafts[key] = d
			} else {
				dropped++
			}
			continue
		}
		drafts[key] = d
		if !fileModTime(file.Path).Equal(d.BaseModTime) {
			stale[key] = true
			changed = append(changed, file.Name)
		}
	}
	m.drafts, m.stale = drafts, stale
	var orphans string
	if dropped > 0 {
		orphans = fmt.Sprintf("dropped %d orphan draft(s)", dropped)
	}
	return m, orphans, changed
}

func (m Model) failedToLoad(path string) bool {
	return slices.ContainsFunc(m.ws().LoadErrors, func(e workspace.FileError) bool {
		return e.Path == path || strings.HasPrefix(path, e.Path+string(filepath.Separator))
	})
}
