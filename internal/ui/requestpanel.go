package ui

import (
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/mc-akash/lazypost/internal/config"
	"github.com/mc-akash/lazypost/internal/httpx"
	"github.com/mc-akash/lazypost/internal/vars"
	"github.com/mc-akash/lazypost/internal/workspace"
)

const (
	reqTitle   = "[4] Request"
	headerRows = 2
	tabGap     = "  "
)

var (
	missingVarStyle = lipgloss.NewStyle().Foreground(lipgloss.Red)
	knownVarStyle   = lipgloss.NewStyle().Foreground(accent)
	activeTabStyle  = focusTitle
)

var reqTabs = []string{"Headers", "Body", "Auth", "Params"}

const (
	reqTabHeaders = iota
	reqTabBody
	reqTabAuth
	reqTabParams
)

type requestPanel struct {
	tab int
	row int
}

func cycleTab(tab, delta, n int) int { return ((tab+delta)%n + n) % n }

func (m Model) requestBindings() []binding {
	format := binding{keys: keys(&km.FormatBody), help: "pretty-print the JSON body (Body tab)"}
	if m.request.tab == reqTabBody {
		format.run, format.hint = noCmd(Model.formatBody), "format"
	}
	tabs := keys(&km.HeadersTab, &km.BodyTab, &km.AuthTab, &km.ParamsTab)
	return []binding{
		{keys: keys(enter), run: Model.enterRow, help: "edit row (add when empty), body editor, auth type", hint: "edit"},
		format,
		{keys: keys(space), run: noCmd(Model.toggleRow), help: "enable / disable header or param row", hint: "toggle"},
		{keys: keys(&km.Add), run: noCmd(Model.addKVRow), hint: "add"},
		{keys: keys(&km.Delete), run: noCmd(Model.deleteRow), hint: "delete"},
		{keys: keys(&km.Method), run: noCmd(Model.openMethodPicker), help: "pick method", hint: "method"},
		{keys: keys(&km.EditURL), run: noCmd(Model.editURL), help: "edit URL", hint: "url"},
		{keys: keys(&km.EditInEditor), run: Model.openEditor, help: "edit request in $EDITOR", hint: "$EDITOR"},
		{keys: keys(&km.Save), run: noCmd(Model.writeDraft), help: "save changes to the request file", hint: "save"},
		{keys: keys(&km.SaveAs), run: noCmd(Model.saveAs), help: "save as a new request (folder/name)", hint: "save as"},
		{keys: keys(&km.CopyCurl), run: Model.copyAsCurl, help: "copy request as cURL (variables resolved)", hint: "copy cURL"},
		{keys: keys(&km.ImportCurl), run: func(m Model) (Model, tea.Cmd) {
			return m.importCurl(true), nil
		}, help: "import cURL into the unsaved request", hint: "import cURL"},
		{keys: keys(&km.Send), run: Model.send, help: "send the selected request", hint: "send"},
		{keys: keys(esc), run: noCmd(Model.handleEsc), help: "back to the side panel"},
		{keys: keys(&km.PrevTab, &km.NextTab), run: func(m Model) (Model, tea.Cmd) {
			m.request = requestPanel{tab: cycleTab(m.request.tab, direction(m.lastKey, km.NextTab), len(reqTabs))}
			return m, nil
		}, help: "previous / next tab"},
		{keys: tabs, run: func(m Model) (Model, tea.Cmd) {
			m.request = requestPanel{tab: slices.IndexFunc(tabs, func(k *config.Keys) bool { return is(m.lastKey, *k) })}
			return m, nil
		}, help: "Headers / Body / Auth / Params tab"},
		{keys: listMoves, run: func(m Model) (Model, tea.Cmd) {
			return m.moveReqRow(direction(m.lastKey, km.NextItem)), nil
		}, help: "select row"},
		{keys: keys(&km.Add, &km.Delete), help: "add (header picker on Headers) / delete row"},
		{keys: keys(&km.BodyType), run: noCmd(Model.cycleBodyType), help: "cycle body type"},
	}
}

func (m Model) requestTitle() string {
	if m.scratchSelected() {
		return reqTitleScratch
	}
	if file, ok := m.editTarget(); ok {
		if mark := m.draftMark(file.Path); mark != "" {
			return reqTitle + " " + mark
		}
	}
	return reqTitle
}

func (m Model) requestLines(w int) []string {
	sel, ok := m.selection()
	if !ok {
		return []string{mutedStyle.Render(truncate("no request selected · "+km.EditURL[0]+": set a URL to start an unsaved one", w))}
	}
	r := m.resolverFor(sel)
	hl := varHighlighter{w: w, known: func(name string) bool {
		_, _, ok := r.Lookup(name)
		return ok
	}}
	req, auth, inherited := sel.tmpl, sel.tmpl.Auth, false
	if sel.entry != nil {
		req, auth = sel.entry.Sent, sel.entry.Sent.Auth
	} else if req.Auth.Type == "" {
		var chain []*workspace.Folder
		if sel.file != nil {
			chain = m.ws().FolderChain(sel.file)
		}
		auth = httpx.EffectiveAuth(&req, chain)
		inherited = auth.Type != "none"
	}

	lines := []string{requestLine(req.Method, req.URL, hl), tabBar(reqTabs, m.request.tab, w)}
	var rows []string
	switch m.request.tab {
	case reqTabHeaders:
		rows = kvTexts(req.Headers, ": ")
		lines = append(lines, hl.kvs(req.Headers, ": ", "no headers · "+km.Confirm[0]+": add")...)
	case reqTabBody:
		lines = append(lines, hl.dim("Type: "+bodyTypeName(req.Body.Type)))
		lines = append(lines, hl.body(req.Body)...)
	case reqTabAuth:
		rows = authTexts(auth, inherited)
		lines = append(lines, hl.lines(rows)...)
	case reqTabParams:
		rows = kvTexts(req.Params, "=")
		lines = append(lines, hl.kvs(req.Params, "=", "no params · "+km.Confirm[0]+": add")...)
	}
	if sel.entry != nil || len(rows) == 0 {
		return lines
	}
	cur := m.reqRow()
	if m.focus == PanelRequest && cur < len(rows) {
		lines[headerRows+cur] = selectedRow(cleanText(rows[cur]), w, true)
	}
	h := computeGeometry(m.width, m.height, len(m.ws().Envs)).reqH - 2
	start, end := listWindow(len(rows), min(cur, len(rows)-1), h-headerRows)
	return append(lines[:headerRows], lines[headerRows+start:headerRows+end]...)
}

func requestLine(method, url string, hl varHighlighter) string {
	label := strings.ToUpper(cleanText(method)) + " "
	if hl.w <= len(label) {
		return truncate(label, hl.w)
	}
	rest := hl
	rest.w -= len(label)
	return methodStyle(method).Render(label) + rest.line(url)
}

func tabBar(names []string, active, w int) string {
	plain := strings.Join(names, tabGap)
	if displayCells(plain) > w {
		return truncate(plain, w)
	}
	parts := make([]string, len(names))
	for i, n := range names {
		if i == active {
			parts[i] = activeTabStyle.Render(n)
		} else {
			parts[i] = mutedStyle.Render(n)
		}
	}
	return strings.Join(parts, tabGap)
}

type varHighlighter struct {
	w     int
	known func(name string) bool
}

func (h varHighlighter) line(s string) string {
	t := truncate(cleanText(s), h.w)
	var b strings.Builder
	last := 0
	for _, loc := range vars.RefPattern.FindAllStringSubmatchIndex(t, -1) {
		b.WriteString(t[last:loc[0]])
		style := knownVarStyle
		if !h.known(t[loc[2]:loc[3]]) {
			style = missingVarStyle
		}
		b.WriteString(style.Render(t[loc[0]:loc[1]]))
		last = loc[1]
	}
	b.WriteString(t[last:])
	return b.String()
}

func (h varHighlighter) dim(s string) string {
	return mutedStyle.Render(truncate(cleanText(s), h.w))
}

func (h varHighlighter) row(s string, disabled bool) string {
	if disabled {
		return h.dim(s)
	}
	return h.line(s)
}

func kvTexts(kvs []workspace.KV, sep string) []string {
	out := make([]string, len(kvs))
	for i, kv := range kvs {
		out[i] = checkbox(kv.Disabled) + kv.Key + sep + kv.Value
	}
	return out
}

func checkbox(disabled bool) string {
	if disabled {
		return "[ ] "
	}
	return "[x] "
}

func (h varHighlighter) kvs(kvs []workspace.KV, sep, empty string) []string {
	if len(kvs) == 0 {
		return []string{h.dim(empty)}
	}
	out := make([]string, len(kvs))
	for i, t := range kvTexts(kvs, sep) {
		out[i] = h.row(t, kvs[i].Disabled)
	}
	return out
}

func (h varHighlighter) lines(texts []string) []string {
	out := make([]string, len(texts))
	for i, t := range texts {
		out[i] = h.line(t)
	}
	return out
}

func (h varHighlighter) body(b workspace.Body) []string {
	switch b.Type {
	case "form":
		return h.kvs(b.Form, "=", "no fields")
	case "multipart":
		if len(b.Parts) == 0 {
			return []string{h.dim("no parts")}
		}
		out := make([]string, len(b.Parts))
		for i, p := range b.Parts {
			v := p.Value
			if p.File {
				v = "@" + v
			}
			out[i] = h.row(checkbox(p.Disabled)+p.Key+"="+v, p.Disabled)
		}
		return out
	case "json", "raw":
		if b.Content == "" {
			return []string{h.dim("empty · " + km.Confirm[0] + ": edit")}
		}
		var out []string
		for _, l := range strings.Split(b.Content, "\n") {
			out = append(out, h.line(l))
		}
		return out
	}
	return []string{h.dim("no body · " + km.Confirm[0] + ": write it, " + km.BodyType[0] + ": type")}
}

func bodyTypeName(t workspace.BodyType) string {
	if t == "" {
		return "none"
	}
	return string(t)
}

func authTexts(a workspace.Auth, inherited bool) []string {
	typ := string(a.Type)
	if typ == "" {
		typ = "none"
	}
	if inherited {
		typ += " (inherited)"
	}
	out := []string{"Type: " + typ}
	for _, f := range authFields(a.Type) {
		out = append(out, f.name+": "+f.get(a))
	}
	return out
}
