package ui

import (
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/mc-akash/lazypost/internal/workspace"
)

const statusEmptyKey = "edit cancelled: empty key"

var (
	httpMethods   = []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"}
	bodyTypeCycle = []workspace.BodyType{"none", "json", "form", "multipart", "raw"}
	authTypes     = []workspace.AuthType{"", "none", "bearer", "basic", "apikey"}
)

func nextIn[T comparable](cycle []T, cur T) T {
	return cycle[(slices.Index(cycle, cur)+1)%len(cycle)]
}

type authField struct {
	name string
	get  func(workspace.Auth) string
	set  func(*workspace.Auth, string)
}

var authFieldsByType = map[workspace.AuthType][]authField{
	"bearer": {
		{"Token", func(a workspace.Auth) string { return a.Token }, func(a *workspace.Auth, v string) { a.Token = v }},
	},
	"basic": {
		{"Username", func(a workspace.Auth) string { return a.Username }, func(a *workspace.Auth, v string) { a.Username = v }},
		{"Password", func(a workspace.Auth) string { return a.Password }, func(a *workspace.Auth, v string) { a.Password = v }},
	},
	"apikey": {
		{"Key", func(a workspace.Auth) string { return a.Key }, func(a *workspace.Auth, v string) { a.Key = v }},
		{"Value", func(a workspace.Auth) string { return a.Value }, func(a *workspace.Auth, v string) { a.Value = v }},
		{"In", func(a workspace.Auth) string {
			if a.In == "" {
				return "header"
			}
			return a.In
		}, func(a *workspace.Auth, v string) { a.In = v }},
	},
}

func authFields(t workspace.AuthType) []authField { return authFieldsByType[t] }

func kvField(r *workspace.Request, tab int) *[]workspace.KV {
	switch tab {
	case reqTabHeaders:
		return &r.Headers
	case reqTabParams:
		return &r.Params
	}
	return nil
}

func (m Model) reqRowCount() int {
	tmpl := m.selectedRequest()
	if kvs := kvField(&tmpl, m.request.tab); kvs != nil {
		return len(*kvs)
	}
	if m.request.tab == reqTabAuth {
		return 1 + len(authFields(tmpl.Auth.Type))
	}
	return 0
}

func (m Model) reqRow() int { return clampCursor(m.request.row, m.reqRowCount()) }

func (m Model) moveReqRow(delta int) Model {
	m.request.row = step(m.request.row, delta, m.reqRowCount())
	return m
}

func (m Model) editKVRows(fn func(kvs *[]workspace.KV, row int)) Model {
	if kvField(&workspace.Request{}, m.request.tab) == nil || m.reqRowCount() == 0 {
		return m
	}
	m, ok := m.requireEditable()
	if !ok {
		return m
	}
	tab, row := m.request.tab, m.reqRow()
	m = m.editRequest(func(r *workspace.Request) { fn(kvField(r, tab), row) })
	m.request.row = m.reqRow()
	return m
}

func (m Model) toggleRow() Model {
	return m.editKVRows(func(kvs *[]workspace.KV, row int) { (*kvs)[row].Disabled = !(*kvs)[row].Disabled })
}

func (m Model) deleteRow() Model {
	return m.editKVRows(func(kvs *[]workspace.KV, row int) { *kvs = slices.Delete(*kvs, row, row+1) })
}

func (m Model) editKVRow(add bool) Model {
	tmpl := m.selectedRequest()
	kvs := kvField(&tmpl, m.request.tab)
	if kvs == nil {
		return m
	}
	m, ok := m.requireEditable()
	if !ok {
		return m
	}
	tab, row := m.request.tab, m.reqRow()
	initial := ""
	if !add {
		if row >= len(*kvs) {
			return m
		}
		initial = (*kvs)[row].Key + ": " + (*kvs)[row].Value
	}
	in := newInput(kvDialogTitle(tab, add), initial, func(m Model, v string) (Model, tea.Cmd) {
		kv, ok := parseKV(v)
		if !ok {
			m.status = statusEmptyKey
			return m, nil
		}
		if !add && v == initial {
			return m, nil
		}
		m = m.editRequest(func(r *workspace.Request) {
			p := kvField(r, tab)
			if add || row >= len(*p) {
				*p = append(*p, kv)
				return
			}
			kv.Disabled = (*p)[row].Disabled
			(*p)[row] = kv
		})
		if add {
			m.request.row = m.reqRowCount() - 1
		}
		return m, nil
	}).withHint(hintKV)
	m.overlay = &in
	return m
}

func kvDialogTitle(tab int, add bool) string {
	switch {
	case tab == reqTabParams && add:
		return dialogAddParam
	case tab == reqTabParams:
		return dialogEditParam
	case add:
		return dialogAddHeader
	}
	return dialogEditHeader
}

func parseKV(s string) (workspace.KV, bool) {
	k, v, found := strings.Cut(s, ": ")
	if !found {
		k, v, _ = strings.Cut(s, ":")
	}
	k = strings.TrimSpace(k)
	if k == "" {
		return workspace.KV{}, false
	}
	return workspace.KV{Key: k, Value: strings.TrimSpace(v)}, true
}

func (m Model) addKVRow() Model {
	if m.request.tab == reqTabHeaders {
		return m.openHeaderPicker()
	}
	return m.editKVRow(true)
}

func (m Model) enterRow() (Model, tea.Cmd) {
	switch m.request.tab {
	case reqTabHeaders, reqTabParams:
		if m.reqRowCount() == 0 {
			return m.addKVRow(), nil
		}
		return m.editKVRow(false), nil
	case reqTabBody:
		return m.openBodyEditor(), nil
	case reqTabAuth:
		return m.editAuthRow(), nil
	}
	return m, nil
}

func (m Model) editAuthRow() Model {
	m, ok := m.requireEditable()
	if !ok {
		return m
	}
	row := m.reqRow()
	if row == 0 {
		return m.openAuthPicker()
	}
	tmpl := m.selectedRequest()
	f := authFields(tmpl.Auth.Type)[row-1]
	initial := f.get(tmpl.Auth)
	in := newInput(dialogEditPrefix+strings.ToLower(f.name), initial, func(m Model, v string) (Model, tea.Cmd) {
		if v == initial {
			return m, nil
		}
		return m.editRequest(func(r *workspace.Request) { f.set(&r.Auth, strings.TrimSpace(v)) }), nil
	})
	m.overlay = &in
	return m
}

func (m Model) cycleBodyType() Model {
	if m.request.tab != reqTabBody {
		return m
	}
	m, ok := m.requireEditable()
	if !ok {
		return m
	}
	return m.editRequest(func(r *workspace.Request) {
		cur := r.Body.Type
		if cur == "" {
			cur = "none"
		}
		r.Body.Type = nextIn(bodyTypeCycle, cur)
	})
}

func (m Model) editURL() Model {
	m, ok := m.requireEditable()
	if !ok {
		return m
	}
	tmpl := m.selectedRequest()
	in := newInput(dialogEditURL, tmpl.URL, func(m Model, v string) (Model, tea.Cmd) {
		v = strings.TrimSpace(v)
		if v == tmpl.URL {
			return m, nil
		}
		return m.editRequest(func(r *workspace.Request) { r.URL = v }), nil
	})
	m.overlay = &in
	return m
}

func statusFormBody() string {
	return "edit form fields with " + km.EditInEditor[0] + " (full request in $EDITOR)"
}
