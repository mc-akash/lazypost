package ui

import (
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
)

type inputModel struct {
	ti       textinput.Model
	title    string
	hint     string
	onSubmit func(Model, string) (Model, tea.Cmd)
}

func newInput(title, initial string, onSubmit func(Model, string) (Model, tea.Cmd)) inputModel {
	ti := textinput.New()
	ti.Prompt = ""
	ti.SetValue(initial)
	ti.CursorEnd()
	ti.Focus()
	return inputModel{ti: ti, title: title, onSubmit: onSubmit}
}

func newSecretInput(title, initial string, onSubmit func(Model, string) (Model, tea.Cmd)) inputModel {
	in := newInput(title, initial, onSubmit)
	in.ti.EchoMode = textinput.EchoPassword
	return in
}

func newBottomInput(prompt, initial string, onSubmit func(Model, string) (Model, tea.Cmd)) inputModel {
	in := newInput("", initial, onSubmit)
	in.ti.Prompt = cleanText(prompt)
	return in
}

func (in inputModel) withHint(hint string) inputModel {
	in.hint = hint
	return in
}

func (in inputModel) isDialog() bool { return in.title != "" }

func (in *inputModel) key(m Model, msg tea.KeyPressMsg) (Model, tea.Cmd) {
	if next, cmd, ok := dispatch(m, msg.String(), in.bindings()); ok {
		return next, cmd
	}
	return in.feed(m, msg)
}

func (in inputModel) bindings() []binding {
	return []binding{
		{keys: keys(enter), run: func(m Model) (Model, tea.Cmd) {
			m.overlay = nil
			return in.onSubmit(m, in.ti.Value())
		}, hint: "confirm"},
		{keys: keys(esc), run: noCmd(Model.closeOverlay), hint: "cancel"},
	}
}

func (in *inputModel) paste(m Model, msg tea.PasteMsg) (Model, tea.Cmd) {
	return in.feed(m, msg)
}

func (in *inputModel) feed(m Model, msg tea.Msg) (Model, tea.Cmd) {
	next := *in
	var cmd tea.Cmd
	next.ti, cmd = next.ti.Update(msg)
	m.overlay = &next
	return m, cmd
}

func (in *inputModel) draw(m Model, screen string) string {
	if !in.isDialog() {
		lines := strings.Split(screen, "\n")
		lines[len(lines)-footerLines] = " " + in.view(m.width-1)
		return strings.Join(lines, "\n")
	}
	return dialog(screen, m.width, m.height, in.title, in.dialogLines)
}

func (in *inputModel) hints(m Model) []binding {
	if !in.isDialog() {
		return m.focusedHints()
	}
	return hinted(in.bindings())
}

func (in inputModel) view(w int) string {
	ti := in.ti
	ti.SetWidth(max(1, w-displayCells(ti.Prompt)-1))
	return ti.View()
}

const dialogError = "Error"

type keyPrompt struct {
	title    string
	message  string
	bindings []binding
}

func (p *keyPrompt) key(m Model, msg tea.KeyPressMsg) (Model, tea.Cmd) {
	m.overlay = nil
	next, cmd, _ := dispatch(m, msg.String(), p.bindings)
	return next, cmd
}

func (p *keyPrompt) draw(m Model, screen string) string {
	return dialog(screen, m.width, m.height, p.title, p.dialogLines)
}

func (p *keyPrompt) hints(Model) []binding { return hinted(p.bindings) }

func (m Model) fail(msg string) Model {
	m.status = hideQuoted(msg)
	return m.showError(msg)
}

func hideQuoted(msg string) string { return quotedValue.ReplaceAllString(msg, "`…`") }

func (m Model) showError(msg string) Model {
	msg = hideQuoted(msg)
	p := &keyPrompt{title: dialogError, message: msg, bindings: []binding{
		{keys: keys(&fixed.Any), run: func(m Model) (Model, tea.Cmd) { return m, nil }, hint: "close"},
	}}
	switch ov := m.overlay.(type) {
	case nil:
		m.overlay = p
	case *keyPrompt:
		if ov.title == dialogError {
			p.message = ov.message + "\n" + msg
			m.overlay = p
		}
	case *bodyEditor:
		ed := *ov
		ed.confirm = p
		m.overlay = &ed
	}
	return m
}

func confirm(title, msg, noStatus string, onYes func(Model) (Model, tea.Cmd)) *keyPrompt {
	no := func(m Model) (Model, tea.Cmd) {
		if noStatus != "" {
			m.status = noStatus
		}
		return m, nil
	}
	return &keyPrompt{title: title, message: msg, bindings: []binding{
		{keys: keys(&fixed.Yes), run: onYes, hint: "yes"},
		{keys: keys(&fixed.No, esc), run: no, hint: "no"},
		{keys: keys(&fixed.Any), run: no},
	}}
}

func (m Model) confirmDelete(title, name string, del func(Model) Model) Model {
	m.overlay = confirm(title, `Delete "`+name+`"?`, "delete cancelled", func(m Model) (Model, tea.Cmd) {
		return del(m), nil
	})
	return m
}
