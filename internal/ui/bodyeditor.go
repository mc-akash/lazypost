package ui

import (
	"os"
	"strings"
	"unicode"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/mc-akash/lazypost/internal/workspace"
)

const (
	bodyEditorPercent = 80

	bodyEditorMaxLines = 9999
	bodyEditorChrome   = 3

	dialogEditBody    = "Edit body ("
	dialogDiscardBody = "Discard changes"
	msgDiscardBody    = "Discard body changes?"

	statusNotJSON = "not valid JSON: "
)

type bodyEditor struct {
	ta      textarea.Model
	typ     workspace.BodyType
	initial string
	confirm *keyPrompt
}

func newBodyEditor(typ workspace.BodyType, content string, w, h int) *bodyEditor {
	ta := textarea.New()
	ta.SetStyles(bodyEditorStyles())
	ta.MaxHeight = bodyEditorMaxLines
	ta.Focus()
	ed := bodyEditor{ta: ta, typ: typ}.sized(w, h).withText(content)
	ed.initial = ed.ta.Value()
	return &ed
}

func bodyEditorStyles() textarea.Styles {
	s := textarea.DefaultStyles(true)
	state := textarea.StyleState{
		Base:             lipgloss.NewStyle(),
		Text:             lipgloss.NewStyle(),
		LineNumber:       mutedStyle,
		CursorLineNumber: lipgloss.NewStyle().Foreground(focusColor),
		CursorLine:       lipgloss.NewStyle(),
		EndOfBuffer:      mutedStyle,
		Placeholder:      mutedStyle,
		Prompt:           mutedStyle,
		Selection:        lipgloss.NewStyle().Reverse(true),
	}
	s.Focused, s.Blurred = state, state
	return s
}

func editorText(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return strings.Map(func(r rune) rune {
		if r != '\n' && r != '\t' && unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
}

func (ed bodyEditor) withText(s string) bodyEditor {
	ed.ta.SetValue(editorText(s))
	ed.ta.MoveToBegin()
	return ed
}

func (ed bodyEditor) changed() bool { return ed.ta.Value() != ed.initial }

func bodyEditorBox(w, h int) (bw, bh int) {
	return min(w, w*bodyEditorPercent/100), min(h, h*bodyEditorPercent/100)
}

func (ed bodyEditor) sized(w, h int) bodyEditor {
	bw, bh := bodyEditorBox(w, h)
	ed.ta.SetWidth(max(1, bw-2-boxPad))
	ed.ta.SetHeight(max(1, bh-bodyEditorChrome))
	return ed
}

func (ed bodyEditor) tempFile() (string, error) {
	return writeTempFile(tempBodyPrefix+bodyFileExt(ed.typ), []byte(ed.ta.Value()))
}

func (ed bodyEditor) lines(w, h int) []string {
	view := strings.Split(ed.ta.View(), "\n")
	clip := lipgloss.NewStyle().MaxWidth(w)
	out := make([]string, 0, h)
	for i := 0; i < h-1; i++ {
		l := ""
		if i < len(view) {
			l = view[i]
		}
		if displayCells(l) > w {
			l = clip.Render(l)
		}
		out = append(out, l)
	}
	return append(out, dialogFooter(ed.bindings(), w))
}

func (ed *bodyEditor) draw(m Model, screen string) string {
	w, h := m.width, m.height
	bw, bh := bodyEditorBox(w, h)
	title := dialogEditBody + bodyTypeName(ed.typ) + ")"
	screen = centered(screen, box(title, ed.lines(bw-2-boxPad, bh-2), bw, bh, true), w, h, bw, bh)
	if ed.confirm != nil {
		screen = ed.confirm.draw(m, screen)
	}
	return screen
}

func (ed *bodyEditor) hints(m Model) []binding {
	if ed.confirm != nil {
		return ed.confirm.hints(m)
	}
	return hinted(ed.bindings())
}

func formBody(t workspace.BodyType) bool { return t == "form" || t == "multipart" }

func (m Model) openBodyEditor() Model {
	if tmpl := m.selectedRequest(); formBody(tmpl.Body.Type) {
		m.status = statusFormBody()
		return m
	}
	m, ok := m.requireEditable()
	if !ok {
		return m
	}
	req, _, ok := m.editable()
	if !ok {
		return m
	}
	if strings.Count(editorText(req.Body.Content), "\n") >= bodyEditorMaxLines {
		m.status = statusBodyTooLong()
		return m
	}
	m.overlay = newBodyEditor(req.Body.Type, req.Body.Content, m.width, m.height)
	return m
}

func (ed *bodyEditor) key(m Model, msg tea.KeyPressMsg) (Model, tea.Cmd) {
	if p := ed.confirm; p != nil {
		next := *ed
		next.confirm = nil
		m.overlay = &next
		n, cmd, _ := dispatch(m, msg.String(), p.bindings)
		return n, cmd
	}
	if next, cmd, ok := dispatch(m, msg.String(), ed.bindings()); ok {
		return next, cmd
	}
	return m.feedBodyEditor(*ed, msg)
}

func (ed bodyEditor) bindings() []binding {
	return []binding{
		{keys: keys(&km.BodySave), run: func(m Model) (Model, tea.Cmd) {
			return m.saveBodyEditor(ed), nil
		}, help: "save the body", hint: "save"},
		{keys: keys(&km.BodyFormat), run: func(m Model) (Model, tea.Cmd) {
			return m.formatBodyEditor(ed), nil
		}, help: "pretty-print JSON", hint: "format"},
		{keys: keys(&km.BodyExternal), run: func(m Model) (Model, tea.Cmd) {
			return m.externalBodyEditor(ed)
		}, help: "continue in $EDITOR (still unsaved)", hint: "$EDITOR"},
		{keys: keys(esc), run: func(m Model) (Model, tea.Cmd) {
			return m.cancelBodyEditor(ed), nil
		}, help: "close (asks when changed)", hint: "cancel"},
	}
}

func (ed *bodyEditor) paste(m Model, msg tea.PasteMsg) (Model, tea.Cmd) {
	if ed.confirm != nil {
		return m, nil
	}
	msg.Content = editorText(msg.Content)
	return m.feedBodyEditor(*ed, msg)
}

func (m Model) feedBodyEditor(ed bodyEditor, msg tea.Msg) (Model, tea.Cmd) {
	var cmd tea.Cmd
	ed.ta, cmd = ed.ta.Update(msg)
	m.overlay = &ed
	return m, cmd
}

func (m Model) saveBodyEditor(ed bodyEditor) Model {
	m.overlay = nil
	if !ed.changed() {
		return m
	}
	return m.setBodyContent(ed.ta.Value())
}

func (m Model) cancelBodyEditor(ed bodyEditor) Model {
	if !ed.changed() {
		m.overlay = nil
		return m
	}
	ed.confirm = confirm(dialogDiscardBody, msgDiscardBody, "", func(m Model) (Model, tea.Cmd) {
		m.overlay = nil
		return m, nil
	})
	m.overlay = &ed
	return m
}

func (m Model) formatBodyEditor(ed bodyEditor) Model {
	out, err := prettyJSON(ed.ta.Value())
	if err != nil {
		m = m.fail(statusNotJSON + err.Error())
		return m
	}
	if out != ed.ta.Value() {
		ed = ed.withText(out)
		m.overlay = &ed
	}
	return m
}

func (m Model) externalBodyEditor(ed bodyEditor) (Model, tea.Cmd) {
	path, err := ed.tempFile()
	if err != nil {
		m = m.fail("editor: " + err.Error())
		return m, nil
	}
	return m.execEditor(editBodyText, path)
}

func (m Model) applyEditorText(path string) Model {
	ed := m.bodyEd()
	if ed == nil {
		return m
	}
	data, err := os.ReadFile(path)
	if err != nil {
		m = m.fail("editor: " + err.Error())
		return m
	}
	text := strings.TrimSuffix(string(data), "\n")
	if text != ed.ta.Value() {
		next := ed.withText(text)
		m.overlay = &next
	}
	return m
}

func (m Model) formatBody() Model {
	if tmpl := m.selectedRequest(); formBody(tmpl.Body.Type) {
		m.status = statusFormBody()
		return m
	}
	m, ok := m.requireEditable()
	if !ok {
		return m
	}
	req, _, ok := m.editable()
	if !ok {
		return m
	}
	out, err := prettyJSON(req.Body.Content)
	if err != nil {
		m = m.fail(statusNotJSON + err.Error())
		return m
	}
	if out == req.Body.Content {
		return m
	}
	return m.editRequest(func(r *workspace.Request) { r.Body.Content = out })
}

func statusBodyTooLong() string {
	return "body too long for the in-app editor: use " + km.EditInEditor[0] + " ($EDITOR)"
}
