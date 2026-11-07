package ui

import (
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/mc-akash/lazypost/internal/history"
	"github.com/mc-akash/lazypost/internal/state"
	"github.com/mc-akash/lazypost/internal/workspace"
)

const maxDrainDepth = 10

type fakeClip struct{ text string }

func (f *fakeClip) Read() (string, error) { return f.text, nil }
func (f *fakeClip) Write(s string) error  { f.text = s; return nil }

func testDeps(t *testing.T, ws *workspace.Workspace) modelDeps {
	t.Helper()
	dir := t.TempDir()
	return modelDeps{
		WS:         ws,
		Session:    state.Session{LastEnv: "staging", Drafts: map[string]state.Draft{}},
		StateStore: state.Open(filepath.Join(dir, "state.json")),
		History:    history.Open(filepath.Join(dir, "history.jsonl")),
		Clip:       &fakeClip{},
		Client:     &http.Client{Timeout: 5 * time.Second},
	}
}

func newTestModel(t *testing.T, ws *workspace.Workspace) Model {
	t.Helper()
	return resize(newModel(testDeps(t, ws)), 120, 40)
}

func resize(m Model, w, h int) Model {
	next, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	return next.(Model)
}

func keyMsg(name string) tea.KeyPressMsg {
	var mod tea.KeyMod
	parts := strings.Split(name, "+")
	base := parts[len(parts)-1]
	if name == "+" {
		base, parts = "+", []string{"+"}
	}
	for _, p := range parts[:len(parts)-1] {
		switch p {
		case "ctrl":
			mod |= tea.ModCtrl
		case "alt":
			mod |= tea.ModAlt
		case "shift":
			mod |= tea.ModShift
		}
	}
	named := map[string]rune{
		"enter": tea.KeyEnter, "esc": tea.KeyEscape, "tab": tea.KeyTab,
		"space": tea.KeySpace, "backspace": tea.KeyBackspace, "delete": tea.KeyDelete,
		"up": tea.KeyUp, "down": tea.KeyDown, "left": tea.KeyLeft, "right": tea.KeyRight,
		"home": tea.KeyHome, "end": tea.KeyEnd, "pgup": tea.KeyPgUp, "pgdown": tea.KeyPgDown,
	}
	if code, ok := named[base]; ok {
		return tea.KeyPressMsg{Code: code, Mod: mod}
	}
	r := []rune(base)[0]
	k := tea.KeyPressMsg{Code: r, Mod: mod}
	if mod&(tea.ModCtrl|tea.ModAlt) == 0 {
		k.Text = base
	}
	return k
}

func press(m Model, keys ...string) Model {
	for _, k := range keys {
		next, _ := m.Update(keyMsg(k))
		m = next.(Model)
	}
	return m
}

func drain(m Model, cmd tea.Cmd) Model {
	return drainDepth(m, cmd, 0)
}

func drainDepth(m Model, cmd tea.Cmd, depth int) Model {
	if cmd == nil || depth > maxDrainDepth {
		return m
	}
	msg := cmd()
	switch msg.(type) {
	case nil, spinner.TickMsg, tea.QuitMsg:
		return m
	}
	if cmds, ok := cmdList(msg); ok {
		for _, c := range cmds {
			m = drainDepth(m, c, depth+1)
		}
		return m
	}
	next, c := m.Update(msg)
	return drainDepth(next.(Model), c, depth+1)
}

func cmdList(msg tea.Msg) ([]tea.Cmd, bool) {
	rv := reflect.ValueOf(msg)
	if rv.Kind() != reflect.Slice || rv.Type().Elem() != reflect.TypeOf(tea.Cmd(nil)) {
		return nil, false
	}
	cmds := make([]tea.Cmd, rv.Len())
	for i := range cmds {
		cmds[i] = rv.Index(i).Interface().(tea.Cmd)
	}
	return cmds, true
}

func fixtureWS(t *testing.T) *workspace.Workspace {
	t.Helper()
	root := filepath.Join(t.TempDir(), "ws")
	ws, err := workspace.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	dev := mustEnv(t, ws, "dev")
	dev.Vars["base_url"] = "http://dev.example"
	staging := mustEnv(t, ws, "staging")
	staging.Vars["base_url"] = "http://staging.example"
	staging.Secrets["token"] = "staging-secret-token"
	for _, e := range []*workspace.Env{dev, staging} {
		if err := ws.SaveEnv(e); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "lazypost.yaml"), []byte("globals:\n  api_ver: v1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	users := mustFolder(t, ws, ws.Collections, "users")
	mustRequest(t, ws, users, "get user", "GET", "{{base_url}}/users/1")
	mustRequest(t, ws, users, "create user", "POST", "{{base_url}}/users")
	orders := mustFolder(t, ws, ws.Collections, "orders")
	mustRequest(t, ws, orders, "list orders", "GET", "{{base_url}}/orders")

	loaded, err := workspace.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	return loaded
}

func mustEnv(t *testing.T, ws *workspace.Workspace, name string) *workspace.Env {
	t.Helper()
	e, err := ws.CreateEnv(name)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func mustFolder(t *testing.T, ws *workspace.Workspace, parent *workspace.Folder, name string) *workspace.Folder {
	t.Helper()
	f, err := ws.CreateFolder(parent, name)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func mustRequest(t *testing.T, ws *workspace.Workspace, parent *workspace.Folder, name, method, url string) {
	t.Helper()
	r, err := ws.CreateRequest(parent, name)
	if err != nil {
		t.Fatal(err)
	}
	r.Method, r.URL = method, url
	if err := ws.SaveRequest(r); err != nil {
		t.Fatal(err)
	}
}

var ansiSeq = regexp.MustCompile(`\x1b\[[0-9;:?]*[A-Za-z]|\x1b\][^\x07\x1b]*(\x07|\x1b\\)`)

func (m Model) plainRender() string {
	return ansiSeq.ReplaceAllString(m.Render(), "")
}

func displayWidth(s string) int { return lipgloss.Width(s) }

func (m Model) input() *inputModel {
	in, _ := m.overlay.(*inputModel)
	return in
}

func (m Model) prompt() *keyPrompt {
	if ed := m.bodyEd(); ed != nil {
		return ed.confirm
	}
	p, _ := m.overlay.(*keyPrompt)
	return p
}

func (m Model) picker() *listPicker {
	p, _ := m.overlay.(*listPicker)
	return p
}

func (m Model) help() *helpScreen {
	hs, _ := m.overlay.(*helpScreen)
	return hs
}

func (m Model) selectedRowLabel() string {
	switch m.focus {
	case PanelEnvs:
		if e := m.environments.selected(m.ws().Envs); e != nil {
			return e.Name
		}
	case PanelCollections:
		if row, ok := m.collections.selected(m.ws().Collections); ok {
			return row.label()
		}
	case PanelHistory:
		if e, ok := m.history.selected(); ok {
			return e.Template.Name
		}
	}
	return ""
}
