package ui

import (
	"errors"
	"path/filepath"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/mc-akash/lazypost/internal/workspace"
)

const (
	statusSaveAsCancel = "save as cancelled"
)

func (m Model) displayPath(r *workspace.Request) string {
	dir := relPath(m.ws().Collections.Path, filepath.Dir(r.Path))
	if dir == "." {
		return r.Name
	}
	return filepath.ToSlash(dir) + "/" + r.Name
}

func (m Model) saveAs() Model {
	if m.scratchSelected() {
		return m.saveScratchAs()
	}
	file, ok := m.editTarget()
	if !ok {
		m.status = statusNotEditable
		return m
	}
	src, prefill := file.Path, m.displayPath(file)
	in := newInput(dialogSaveAs, prefill, func(m Model, v string) (Model, tea.Cmd) {
		return m.submitSaveAs(src, prefill, v), nil
	}).withHint(hintSaveAs)
	m.overlay = &in
	return m
}

func (m Model) submitSaveAs(src, prefill, value string) Model {
	folders, name := splitSaveAs(value)
	file := m.ws().FindRequest(src)
	if file != nil && strings.TrimSpace(value) == prefill {
		folders, name = nil, file.Name
		for _, f := range m.ws().FolderChain(file)[1:] {
			folders = append(folders, f.Name)
		}
	}
	if name == "" || file == nil {
		m.status = statusSaveAsCancel
		return m
	}
	created, note, err := m.saveNewRequest(folders, name, m.effective(file))
	if err != nil {
		m = m.fail(saveFailStatus("save as failed: ", err))
		return m
	}
	m = m.dropDraft(m.draftKey(src))
	m.collections = m.collections.reveal(m.ws().Collections, created.Path)
	m.status = "saved as " + m.displayPath(created) + note
	return m.persistSession()
}

func (m Model) saveNewRequest(folders []string, name string, req workspace.Request) (*workspace.Request, string, error) {
	env := m.activeEnv()
	if env == nil {
		if hasImportSecrets(req, name) {
			return nil, "", errSaveNeedsEnv
		}
		env = &workspace.Env{}
	}
	parent, err := m.ensureFolders(folders)
	if err != nil {
		return nil, "", err
	}
	return m.createRequest(parent, name, req, env)
}

func (m Model) createRequest(parent *workspace.Folder, name string, req workspace.Request, env *workspace.Env) (*workspace.Request, string, error) {
	withRefs, secrets := extractSecrets(req, name, env)
	created, err := m.ws().CreateRequest(parent, name)
	if err != nil {
		return nil, "", err
	}
	withRefs.Name, withRefs.Path, withRefs.Parent = name, created.Path, created.Parent
	note := ""
	err = m.ws().SaveRequest(&withRefs)
	if err == nil {
		note, err = m.commitSecrets(env, secrets)
	}
	if err != nil {
		return nil, "", errors.Join(err, m.ws().DeleteRequest(created))
	}
	*created = withRefs
	return created, note, nil
}

func splitSaveAs(value string) (folders []string, name string) {
	parts := strings.Split(value, "/")
	for _, p := range parts[:len(parts)-1] {
		if p = strings.TrimSpace(p); p != "" {
			folders = append(folders, p)
		}
	}
	return folders, strings.TrimSpace(parts[len(parts)-1])
}

func (m Model) ensureFolders(segs []string) (*workspace.Folder, error) {
	f := m.ws().Collections
	for _, seg := range segs {
		i := slices.IndexFunc(f.Folders, func(sub *workspace.Folder) bool {
			return sub.Name == seg || sub.Name == workspace.Slug(seg)
		})
		if i >= 0 {
			f = f.Folders[i]
			continue
		}
		sub, err := m.ws().CreateFolder(f, seg)
		if err != nil {
			return nil, err
		}
		f = sub
	}
	return f, nil
}
