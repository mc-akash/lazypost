package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/mc-akash/lazypost/internal/curl"
	"github.com/mc-akash/lazypost/internal/workspace"
)

const (
	statusCopiedCurl = "copied as cURL"
)

func (m Model) copyAsCurl() (Model, tea.Cmd) {
	sel, ok := m.selection()
	if !ok {
		return m, nil
	}
	resolved, _, _ := m.resolve(sel)
	text := curl.Export(resolved)
	m.status = statusCopiedCurl
	if err := m.deps.Clip.Write(text); err != nil {
		m.status = statusCopiedCurl + " (OSC 52 only): " + err.Error()
	}
	return m, tea.SetClipboard(text)
}

func (m Model) importCurl(toScratch bool) Model {
	initial := ""
	if text, err := m.deps.Clip.Read(); err == nil && strings.HasPrefix(strings.TrimSpace(text), "curl") {
		initial = strings.TrimSpace(text)
	}
	ed := newBodyEditor("", initial, m.width, m.height)
	ed.title, ed.ta.Placeholder = dialogPasteCurl, hintCurl
	ed.onSave = func(m Model, value string) (Model, tea.Cmd) { return m.submitCurl(value, toScratch) }
	m.overlay = ed
	return m
}

func (m Model) submitCurl(value string, toScratch bool) (Model, tea.Cmd) {
	if strings.TrimSpace(value) == "" {
		m.overlay = nil
		return m, nil
	}
	imported, err := curl.Import(value)
	if err != nil {
		m = m.fail("curl: " + err.Error())
		return m, nil
	}
	m.overlay = nil
	if toScratch {
		return m.importIntoScratch(imported), nil
	}
	in := newInput(dialogRequestName, imported.Name, func(m Model, name string) (Model, tea.Cmd) {
		return m.createImported(strings.TrimSpace(name), imported), nil
	})
	m.overlay = &in
	return m, nil
}

func statusImportDraft() string {
	return "imported as draft: no environment for secrets (" + km.Save[0] + " writes)"
}

func (m Model) createImported(name string, imported workspace.Request) Model {
	parent := m.newItemParent()
	if name == "" || parent == nil {
		return m
	}
	env := m.activeEnv()
	if env == nil {
		if hasImportSecrets(imported, name) {
			return m.createImportedDraft(parent, name, imported)
		}
		env = &workspace.Env{}
	}
	r, note, err := m.createRequest(parent, name, imported, env)
	if err != nil {
		return m.fail(saveFailStatus("create failed: ", err))
	}
	m.status = "created " + r.Name + note
	m.collections = m.collections.reveal(m.ws().Collections, r.Path)
	m.onScratch = false
	return m
}

func hasImportSecrets(imported workspace.Request, name string) bool {
	_, secrets := extractSecrets(imported, name, &workspace.Env{})
	return len(secrets) > 0
}

func (m Model) moveImportSecrets(imported workspace.Request, name string, env *workspace.Env) (workspace.Request, string, error) {
	withRefs, secrets := extractSecrets(imported, name, env)
	note, err := m.commitSecrets(env, secrets)
	if err != nil {
		return imported, "", err
	}
	return withRefs, note, nil
}

func (m Model) commitSecrets(env *workspace.Env, secrets []importSecret) (string, error) {
	if len(secrets) == 0 {
		return "", nil
	}
	err := m.commitEnv(env, func(_, ns map[string]string) {
		for _, s := range secrets {
			ns[s.name] = s.value
		}
	})
	if err != nil {
		return "", err
	}
	return secretsNote(len(secrets), env.Name), nil
}

func (m Model) createImportedDraft(parent *workspace.Folder, name string, imported workspace.Request) Model {
	r, err := m.ws().CreateRequest(parent, name)
	if err != nil {
		m = m.fail("create failed: " + err.Error())
		return m
	}
	draft := *r
	setImported(&draft, imported)
	m.collections = m.collections.reveal(m.ws().Collections, r.Path)
	m.onScratch = false
	m.status = statusImportDraft()
	return m.storeDraft(r, draft)
}

func setImported(r *workspace.Request, imported workspace.Request) {
	r.Method, r.URL = imported.Method, imported.URL
	r.Params, r.Headers = imported.Params, imported.Headers
	r.Auth, r.Body = imported.Auth, imported.Body
}
