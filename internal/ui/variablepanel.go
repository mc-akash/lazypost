package ui

import (
	"maps"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/mc-akash/lazypost/internal/vars"
	"github.com/mc-akash/lazypost/internal/workspace"
)

const (
	varsTitlePrefix = "[4] Variables"
	maskedValue     = "••••••••"
	missingValue    = "(missing)"
	colGap          = "  "
	maxNameCol      = 24
)

var varsHeader = [3]string{"NAME", "VALUE", "FROM"}

type variablePanel struct {
	cursor   int
	revealed map[string]bool
}

func revealKey(env, name string) string { return env + "\x00" + name }

func (m Model) showsVars() bool { return m.focus == PanelEnvs || m.focus == PanelVars }

func (m Model) tableRequest() (*workspace.Request, workspace.Request) {
	if s, ok := m.selection(); ok {
		return s.file, s.tmpl
	}
	if row, ok := m.collections.selected(m.ws().Collections); ok && row.req != nil {
		return row.req, *row.req
	}
	return nil, workspace.Request{}
}

func (m Model) varRows() []vars.Row {
	file, tmpl := m.tableRequest()
	return vars.ForRequest(m.ws(), m.highlightedEnv(), file).Table(vars.RequestRefs(tmpl))
}

func (m Model) selectedVar() (vars.Row, bool) {
	rows := m.varRows()
	if len(rows) == 0 {
		return vars.Row{}, false
	}
	return rows[clampCursor(m.variables.cursor, len(rows))], true
}

func (m Model) isRevealed(name string) bool {
	e := m.highlightedEnv()
	return e != nil && m.variables.revealed[revealKey(e.Name, name)]
}

func (m Model) masked(row vars.Row) bool { return row.Secret && !m.isRevealed(row.Name) }

func (m Model) varsTitle() string {
	if e := m.highlightedEnv(); e != nil {
		return varsTitlePrefix + " · " + e.Name
	}
	return varsTitlePrefix
}

func (Model) varsBindings() []binding {
	return []binding{
		{keys: keys(enter), run: noCmd(Model.editVar), help: "edit value", hint: "edit value"},
		{keys: keys(&km.Paste), run: noCmd(Model.pasteVar), help: "paste clipboard into value", hint: "paste clipboard"},
		{keys: keys(&km.Reveal), run: noCmd(Model.toggleReveal), help: "reveal / hide secret", hint: "reveal"},
		{keys: keys(&km.Add), run: noCmd(Model.addVar), hint: "add"},
		{keys: keys(&km.Delete), run: noCmd(Model.deleteVar), hint: "delete"},
		{keys: keys(&km.ToggleSecret), run: noCmd(Model.toggleSecret), help: "move between vars and secrets", hint: "secret"},
		{keys: keys(&km.Add, &km.Delete), help: "add / delete variable"},
		{keys: keys(esc), run: noCmd(func(m Model) Model {
			back := m.prevFocus
			if back == 0 || back == PanelVars {
				back = PanelEnvs
			}
			return m.setFocus(back)
		}), help: "back to previous panel"},
		{keys: listMoves, run: func(m Model) (Model, tea.Cmd) {
			return m.moveVarCursor(direction(m.lastKey, km.NextItem)), nil
		}},
	}
}

func (m Model) moveVarCursor(delta int) Model {
	n := len(m.varRows())
	m.variables.cursor = step(m.variables.cursor, delta, n)
	return m
}

func (m Model) editVar() Model {
	row, ok := m.selectedVar()
	if !ok {
		return m
	}
	masked := m.masked(row)
	initial := row.Value
	if masked {
		initial = ""
	}
	target := vars.PlanWrite(row)
	in := valueInput(row.Secret)(dialogEditPrefix+row.Name, initial, func(m Model, v string) (Model, tea.Cmd) {
		if masked && v == "" {
			m.status = row.Name + " unchanged"
			return m, nil
		}
		return m.saveVar(row.Name, v, target), nil
	})
	if masked {
		in = in.withHint(hintKeepValue)
	}
	m.overlay = &in
	return m
}

func (m Model) pasteVar() Model {
	row, ok := m.selectedVar()
	if !ok {
		return m
	}
	if m.highlightedEnv() == nil {
		m.status = statusNoEnv()
		return m
	}
	target := vars.PlanWrite(row)
	save := func(m Model, text string) Model {
		v := strings.TrimSpace(text)
		if v == "" {
			m.status = "nothing to paste"
			return m
		}
		return m.saveVar(row.Name, v, target)
	}
	text, err := m.deps.Clip.Read()
	if err != nil {
		in := valueInput(row.Secret)(dialogPasteValue, "", func(m Model, v string) (Model, tea.Cmd) {
			return save(m, v), nil
		})
		m.overlay = &in
		return m
	}
	return save(m, text)
}

func (m Model) toggleReveal() Model {
	row, ok := m.selectedVar()
	e := m.highlightedEnv()
	if !ok || e == nil {
		return m
	}
	k := revealKey(e.Name, row.Name)
	revealed := maps.Clone(m.variables.revealed)
	if revealed == nil {
		revealed = map[string]bool{}
	}
	if revealed[k] {
		delete(revealed, k)
	} else {
		revealed[k] = true
	}
	m.variables.revealed = revealed
	return m
}

func (m Model) addVar() Model {
	if m.highlightedEnv() == nil {
		m.status = statusNoEnv()
		return m
	}
	in := newInput(dialogAddVar, "", submitAddName).withHint(hintVarName)
	m.overlay = &in
	return m
}

func submitAddName(m Model, value string) (Model, tea.Cmd) {
	name := strings.TrimSpace(value)
	if !validVarName(name) {
		if name != "" {
			m.status = "invalid variable name " + `"` + name + `"`
		}
		return m, nil
	}

	in := valueInput(vars.LooksSecret(name))(dialogVarValue, "", func(m Model, v string) (Model, tea.Cmd) {
		return m.askSecret(name, v), nil
	}).withHint(name)
	m.overlay = &in
	return m, nil
}

func valueInput(secret bool) func(string, string, func(Model, string) (Model, tea.Cmd)) inputModel {
	if secret {
		return newSecretInput
	}
	return newInput
}

func validVarName(name string) bool {
	return name != "" && !strings.HasPrefix(name, "$") && !strings.ContainsAny(name, "{} \t\r\n")
}

func (m Model) askSecret(name, value string) Model {
	def := vars.LooksSecret(name)
	label := "no"
	if def {
		label = "yes"
	}
	save := func(secret bool) func(Model) (Model, tea.Cmd) {
		return func(m Model) (Model, tea.Cmd) { return m.saveVar(name, value, writeTarget(name, secret)), nil }
	}
	p := &keyPrompt{title: dialogSecret, message: `Is "` + name + `" a secret?`}
	p.bindings = []binding{
		{keys: keys(enter), run: save(def), hint: label},
		{keys: keys(&fixed.Yes), run: save(true), hint: "yes"},
		{keys: keys(&fixed.No), run: save(false), hint: "no"},
		{keys: keys(esc), run: func(m Model) (Model, tea.Cmd) {
			m.status = "add cancelled"
			return m, nil
		}},
		{keys: keys(&fixed.Any), run: func(m Model) (Model, tea.Cmd) {
			m.overlay = p
			return m, nil
		}},
	}
	m.overlay = p
	return m
}

func writeTarget(name string, secret bool) vars.WriteTarget {
	src := vars.SrcEnv
	if secret {
		src = vars.SrcSecret
	}
	return vars.PlanWrite(vars.Row{Name: name, Source: src})
}

func (m Model) ownRow(row vars.Row, verb string) (Model, bool) {
	switch {
	case row.Missing:
		m.status = row.Name + " is not defined"
		return m, false
	case row.Source != vars.SrcEnv && row.Source != vars.SrcSecret:
		m.status = "inherited from " + row.Source.String() + " — not " + verb
		return m, false
	}
	return m, true
}

func (m Model) deleteVar() Model {
	row, ok := m.selectedVar()
	if !ok {
		return m
	}
	if m, ok = m.ownRow(row, "deleted"); !ok {
		return m
	}
	return m.confirmDelete(dialogDelVar, row.Name, func(m Model) Model {
		env := m.highlightedEnv()
		err := m.commitEnv(env, func(nv, ns map[string]string) {
			delete(nv, row.Name)
			delete(ns, row.Name)
		})
		if err != nil {
			m = m.fail("save failed: " + err.Error())
			return m
		}
		m.status = "deleted " + row.Name
		return m
	})
}

func (m Model) toggleSecret() Model {
	row, ok := m.selectedVar()
	if !ok {
		return m
	}
	if m, ok = m.ownRow(row, "moved"); !ok {
		return m
	}
	return m.saveVar(row.Name, row.Value, writeTarget(row.Name, row.Source == vars.SrcEnv))
}

func (m Model) saveVar(name, value string, target vars.WriteTarget) Model {
	env := m.highlightedEnv()
	if env == nil {
		m.status = statusNoEnv()
		return m
	}
	err := m.commitEnv(env, func(nv, ns map[string]string) {
		into, from := nv, ns
		if target == vars.ToEnvSecret {
			into, from = ns, nv
		}
		into[name] = value
		delete(from, name)
	})
	if err != nil {
		m = m.fail("save failed: " + err.Error())
		return m
	}
	m.status = "saved " + name
	if target == vars.ToEnvSecret {
		m.status += " (secret)"
	}
	return m
}

func (m Model) commitEnv(env *workspace.Env, edit func(vars, secrets map[string]string)) error {
	nv, ns := cloneVars(env.Vars), cloneVars(env.Secrets)
	edit(nv, ns)
	if err := m.ws().SaveEnv(&workspace.Env{Name: env.Name, Vars: nv, Secrets: ns}); err != nil {
		return err
	}
	env.Vars, env.Secrets = nv, ns
	return nil
}

func cloneVars(src map[string]string) map[string]string {
	out := make(map[string]string, len(src)+1)
	maps.Copy(out, src)
	return out
}

func (m Model) varsLines(w, h int) []string {
	rows := m.varRows()
	nameW, fromW := len(varsHeader[0]), len(varsHeader[2])
	for _, r := range rows {
		nameW = max(nameW, displayCells(cleanText(r.Name)))
		if !r.Missing {
			fromW = max(fromW, len(r.Source.String()))
		}
	}
	nameW = min(nameW, maxNameCol)
	valueW := max(1, w-nameW-fromW-2*len(colGap))
	cols := [3]int{nameW, valueW, fromW}

	lines := []string{mutedStyle.Render(truncate(tableRow(varsHeader, cols), w))}
	if len(rows) == 0 {
		return append(lines, mutedStyle.Render(truncate("no variables", w)))
	}
	cursor := clampCursor(m.variables.cursor, len(rows))
	start, end := listWindow(len(rows), cursor, h-1)
	for i := start; i < end; i++ {
		lines = append(lines, m.varLine(rows[i], cols, w, i == cursor))
	}
	return lines
}

func (m Model) varLine(r vars.Row, cols [3]int, w int, selected bool) string {
	value, from := cleanText(r.Value), r.Source.String()
	switch {
	case r.Missing:
		value, from = missingValue, ""
	case m.masked(r):
		value = maskedValue
	}
	cells := [3]string{cleanText(r.Name), value, from}
	if selected {
		return selectedRow(tableRow(cells, cols), w, m.focus == PanelVars)
	}
	if !r.Missing || cols[1] < len(missingValue) {
		return truncate(tableRow(cells, cols), w)
	}
	name := padCell(cells[0], cols[0]) + colGap
	return name + missingVarStyle.Render(missingValue) + strings.Repeat(" ", max(0, cols[1]-len(missingValue)))
}

func tableRow(cells [3]string, cols [3]int) string {
	return padCell(cells[0], cols[0]) + colGap + padCell(cells[1], cols[1]) + colGap + truncate(cells[2], cols[2])
}

func padCell(s string, w int) string {
	t := truncate(s, w)
	return t + strings.Repeat(" ", max(0, w-displayCells(t)))
}

func statusNoEnv() string {
	return "no environment — press " + km.New[0] + " in [1] to create one"
}
