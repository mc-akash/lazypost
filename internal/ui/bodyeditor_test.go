package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/mc-akash/lazypost/internal/workspace"
)

const editorHint = "save: ctrl+s | format: ctrl+f | $EDITOR: ctrl+e | cancel: esc"

func scratchBodyModel(t *testing.T) Model {
	t.Helper()
	m := newTestModel(t, emptyWS(t))
	return press(typeText(press(m, "N"), "http://x.test"), "enter", "B")
}

func edText(t *testing.T, m Model) string {
	t.Helper()
	if m.bodyEd() == nil {
		t.Fatal("body editor is not open")
	}
	return m.bodyEd().ta.Value()
}

func TestBodyEditorOpensPrefilled(t *testing.T) {
	m := editBodyTo(t, press(requestModel(t), "B"), `{"a":1}`)
	m = press(m, "enter")
	if got := edText(t, m); got != `{"a":1}` {
		t.Fatalf("editor text = %q", got)
	}
	out := m.plainRender()
	if !strings.Contains(out, "Edit body (json)") {
		t.Fatalf("title missing:\n%s", out)
	}
	if !strings.Contains(out, "save: ctrl+s") {
		t.Fatalf("footer missing:\n%s", out)
	}
	if hint := lastLine(out); !strings.Contains(hint, editorHint) {
		t.Fatalf("hint %q", hint)
	}
}

func TestBodyEditorSize(t *testing.T) {
	for _, sz := range [][2]int{{120, 40}, {80, 20}} {
		w, h := sz[0], sz[1]
		m := press(resize(scratchBodyModel(t), w, h), "enter")
		bw, bh := w*80/100, h*80/100
		lines := strings.Split(m.plainRender(), "\n")
		title := "Edit body (none)"
		top, bottom := (h-bh)/2, (h-bh)/2+bh-1
		if want := "┌" + title + strings.Repeat("─", bw-2-len(title)) + "┐"; !strings.Contains(lines[top], want) {
			t.Fatalf("%v: line %d is not the dialog top:\n%s", sz, top, m.plainRender())
		}
		if want := "└" + strings.Repeat("─", bw-2) + "┘"; !strings.Contains(lines[bottom], want) {
			t.Fatalf("%v: line %d is not the dialog bottom:\n%s", sz, bottom, m.plainRender())
		}
	}
}

func TestBodyEditorGetsEveryKey(t *testing.T) {
	m := press(scratchBodyModel(t), "enter")
	text := `{"q":"s1?E4N0hjk[]HBAP"}`
	m = typeText(m, text)
	m = press(m, "tab", "shift+tab")
	if got := edText(t, m); got != text {
		t.Fatalf("editor text = %q, want %q", got, text)
	}
	if m.focus != PanelRequest || m.help() != nil || m.picker() != nil || m.input() != nil || m.request.tab != reqTabBody {
		t.Fatalf("a key leaked: focus %v help %v picker %v input %v tab %d", m.focus, m.help() != nil, m.picker(), m.input(), m.request.tab)
	}
	if m.flight != nil {
		t.Fatal("s sent the request")
	}
}

func TestBodyEditorCtrlCQuits(t *testing.T) {
	m := press(scratchBodyModel(t), "enter")
	_, cmd := m.Update(keyMsg("ctrl+c"))
	if cmd == nil {
		t.Fatal("ctrl+c should quit")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("ctrl+c should quit")
	}
}

func TestBodyEditorSaveInfersType(t *testing.T) {
	for _, tc := range []struct{ text, typ string }{{`{"a":1}`, "json"}, {"hello", "raw"}} {
		m := press(scratchBodyModel(t), "enter")
		m = press(typeText(m, tc.text), "ctrl+s")
		if m.bodyEd() != nil {
			t.Fatalf("%q: ctrl+s should close the editor", tc.text)
		}
		if b := m.scratch.Body; b.Content != tc.text || string(b.Type) != tc.typ {
			t.Fatalf("%q: body = %+v, want type %s", tc.text, b, tc.typ)
		}
	}
}

func TestBodyEditorSaveKeepsChosenType(t *testing.T) {
	m := press(scratchBodyModel(t), "t", "t", "t", "t", "enter")
	m = press(typeText(m, `{"a":1}`), "ctrl+s")
	if b := m.scratch.Body; b.Content != `{"a":1}` || b.Type != "raw" {
		t.Fatalf("body = %+v, want raw kept", b)
	}
}

func TestBodyEditorSavesDraftOfSavedRequest(t *testing.T) {
	m := press(requestModel(t), "B")
	file, _ := m.editTarget()
	before := readFile(t, file.Path)
	m = press(typeText(press(m, "enter"), `{"x":1}`), "ctrl+s")
	d, ok := m.drafts[getUserKey]
	if !ok || d.Request.Body.Content != `{"x":1}` || d.Request.Body.Type != "json" {
		t.Fatalf("draft = %+v (ok %v)", d.Request.Body, ok)
	}
	if readFile(t, file.Path) != before {
		t.Fatal("the file changed")
	}
	if !strings.Contains(m.plainRender(), "[4] Request ●") {
		t.Fatal("draft marker missing")
	}
}

func TestBodyEditorEscWithoutChangesCloses(t *testing.T) {
	m := press(requestModel(t), "B", "enter", "esc")
	if m.bodyEd() != nil || m.prompt() != nil || len(m.drafts) != 0 {
		t.Fatalf("esc: editor %v prompt %v drafts %v", m.bodyEd(), m.prompt(), m.drafts)
	}

	m = press(typeText(press(m, "enter"), "x"), "backspace", "esc")
	if m.bodyEd() != nil || m.prompt() != nil {
		t.Fatalf("esc after undoing: editor %v prompt %v", m.bodyEd(), m.prompt())
	}
}

func TestBodyEditorEscWithChangesAsks(t *testing.T) {
	m := press(typeText(press(scratchBodyModel(t), "enter"), "x"), "esc")
	if m.prompt() == nil || m.bodyEd() == nil {
		t.Fatalf("esc with changes should ask: prompt %v editor %v", m.prompt(), m.bodyEd())
	}
	if out := m.plainRender(); !strings.Contains(out, "Discard body changes?") {
		t.Fatalf("question missing:\n%s", out)
	}
	if hint := lastLine(m.plainRender()); !strings.Contains(hint, "yes: y") {
		t.Fatalf("hint %q", hint)
	}
	m = press(m, "n")
	if m.prompt() != nil || m.bodyEd() == nil {
		t.Fatalf("n should go back to editing: prompt %v editor %v", m.prompt(), m.bodyEd())
	}
	m = typeText(m, "y")
	if got := edText(t, m); got != "xy" {
		t.Fatalf("editor text = %q, want xy", got)
	}
	m = press(m, "esc", "y")
	if m.prompt() != nil || m.bodyEd() != nil {
		t.Fatalf("y should discard: prompt %v editor %v", m.prompt(), m.bodyEd())
	}
	if m.scratch.Body.Content != "" {
		t.Fatalf("discarded text was saved: %+v", m.scratch.Body)
	}
}

func TestBodyEditorFormat(t *testing.T) {
	m := press(scratchBodyModel(t), "enter")
	m = press(typeText(m, `{"id": {{user_id}},"auth":"Bearer {{token}}"}`), "ctrl+f")
	want := "{\n  \"id\": {{user_id}},\n  \"auth\": \"Bearer {{token}}\"\n}"
	if got := edText(t, m); got != want {
		t.Fatalf("editor text:\n%s\nwant:\n%s", got, want)
	}
	m = press(m, "ctrl+s")
	if m.scratch.Body.Content != want {
		t.Fatalf("saved %q", m.scratch.Body.Content)
	}
}

func TestBodyEditorFormatInvalidJSON(t *testing.T) {
	m := press(scratchBodyModel(t), "enter")
	m = press(typeText(m, `{"a":`), "ctrl+f")
	if !strings.HasPrefix(m.Status(), "not valid JSON: ") {
		t.Fatalf("status = %q", m.Status())
	}
	if got := edText(t, m); got != `{"a":` {
		t.Fatalf("text changed to %q", got)
	}
}

func TestBodyEditorPasteAndControlChars(t *testing.T) {
	m := press(scratchBodyModel(t), "enter")
	next, _ := m.Update(tea.PasteMsg{Content: "a\r\nb\x1b[31mc\x9b\x07d"})
	m = next.(Model)
	if got := edText(t, m); got != "a\nb[31mcd" {
		t.Fatalf("editor text = %q", got)
	}
}

func TestBodyEditorSanitizesPrefill(t *testing.T) {
	m := scratchBodyModel(t)
	m = m.editRequest(func(r *workspace.Request) {
		r.Body.Type, r.Body.Content = "raw", "x\x1b]0;evil\x07y\r\nz\x9b1m"
	})
	m = press(m, "enter")
	got := edText(t, m)
	if got != "x]0;evily\nz1m" {
		t.Fatalf("editor text = %q", got)
	}
	if strings.Contains(m.Render(), "\x1b]0;evil") {
		t.Fatal("the body's escape sequence reached the screen")
	}

	m = press(m, "esc")
	if m.prompt() != nil || m.bodyEd() != nil || m.scratch.Body.Content != "x\x1b]0;evil\x07y\r\nz\x9b1m" {
		t.Fatalf("esc on an unchanged editor: prompt %v body %q", m.prompt(), m.scratch.Body.Content)
	}
}

func TestBodyEditorNotForFormBodies(t *testing.T) {
	m := press(scratchBodyModel(t), "t", "t", "enter")
	if m.bodyEd() != nil || m.Status() != statusFormBody() {
		t.Fatalf("form body: editor %v status %q", m.bodyEd(), m.Status())
	}
}

func TestBodyEditorTooLong(t *testing.T) {
	m := scratchBodyModel(t)
	long := strings.Repeat("x\n", bodyEditorMaxLines) + "x"
	m = m.editRequest(func(r *workspace.Request) { r.Body.Type, r.Body.Content = "raw", long })
	m = press(m, "enter")
	if m.bodyEd() != nil || m.Status() != statusBodyTooLong() {
		t.Fatalf("long body: editor %v status %q", m.bodyEd(), m.Status())
	}
}

func TestBodyEditorExternalRoundTrip(t *testing.T) {
	m := press(scratchBodyModel(t), "t", "enter")
	m = typeText(m, `{"a":1}`)
	path, err := m.bodyEd().tempFile()
	if err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(path); err != nil || fi.Mode().Perm() != 0o600 || filepath.Ext(path) != ".json" {
		t.Fatalf("temp file %s: %v %v", path, fi, err)
	}
	if got := readFile(t, path); got != `{"a":1}` {
		t.Fatalf("temp file holds %q", got)
	}
	if err := os.WriteFile(path, []byte("{\"b\":2}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	m = m.handleEditorDone(editorDoneMsg{kind: editBodyText, path: path})
	if got := edText(t, m); got != `{"b":2}` {
		t.Fatalf("editor text = %q", got)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("temp file should be removed")
	}
	if m.scratch.Body.Content != "" {
		t.Fatal("ctrl+e must not save the body")
	}
	m = press(m, "ctrl+s")
	if m.scratch.Body.Content != `{"b":2}` {
		t.Fatalf("saved %q", m.scratch.Body.Content)
	}
}

func TestBodyEditorCtrlEStartsEditor(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	t.Setenv("VISUAL", "true")
	m := press(scratchBodyModel(t), "enter")
	next, cmd := m.Update(keyMsg("ctrl+e"))
	if cmd == nil || next.(Model).bodyEd() == nil {
		t.Fatal("ctrl+e should run $EDITOR and keep the editor open")
	}

	t.Setenv("VISUAL", `"unterminated`)
	m = press(typeText(m, "abc"), "ctrl+e")
	if !strings.HasPrefix(m.Status(), "editor: ") || edText(t, m) != "abc" {
		t.Fatalf("status %q text %q", m.Status(), edText(t, m))
	}
}

func TestFormatKeyOnBodyTab(t *testing.T) {
	m := editBodyTo(t, press(requestModel(t), "B"), `{"a":1,"b":[{{n}}]}`)
	m = press(m, "f")
	want := "{\n  \"a\": 1,\n  \"b\": [\n    {{n}}\n  ]\n}"
	if got := currentTmpl(m).Body.Content; got != want {
		t.Fatalf("body:\n%s\nwant:\n%s", got, want)
	}
	if d, ok := m.drafts[getUserKey]; !ok || d.Request.Body.Content != want {
		t.Fatal("f should edit the draft")
	}
	if hint := lastLine(m.plainRender()); !strings.Contains(hint, "format: f") {
		t.Fatalf("Body tab hint %q", hint)
	}
	if hint := lastLine(press(m, "H").plainRender()); strings.Contains(hint, "format: f") {
		t.Fatalf("Headers tab hint %q", hint)
	}
}

func TestFormatKeyOnScratch(t *testing.T) {
	m := scratchBodyModel(t)
	m = press(typeText(press(m, "enter"), `[1,2]`), "ctrl+s", "f")
	if got := m.scratch.Body.Content; got != "[\n  1,\n  2\n]" {
		t.Fatalf("body = %q", got)
	}
}

func TestFormatKeyInvalidJSON(t *testing.T) {
	m := scratchBodyModel(t)
	m = press(typeText(press(m, "enter"), "plain"), "ctrl+s", "f")
	if !strings.HasPrefix(m.Status(), "not valid JSON: ") || m.scratch.Body.Content != "plain" {
		t.Fatalf("status %q body %q", m.Status(), m.scratch.Body.Content)
	}
}

func TestFormatKeyOtherTabs(t *testing.T) {
	m := press(requestModel(t), "H", "f")
	if len(m.drafts) != 0 || strings.Contains(m.Status(), "JSON") {
		t.Fatalf("f on Headers: drafts %v status %q", m.drafts, m.Status())
	}
}
