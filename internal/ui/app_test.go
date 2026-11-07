package ui

import (
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/mc-akash/lazypost/internal/config"
	"github.com/mc-akash/lazypost/internal/history"
	"github.com/mc-akash/lazypost/internal/workspace"
)

func TestRenderShowsAllPanels(t *testing.T) {
	out := newTestModel(t, fixtureWS(t)).plainRender()
	for _, title := range []string{"[1] Environments", "[2] Collections", "[3] History", "[4] Request", "[5] Response"} {
		if !strings.Contains(out, title) {
			t.Errorf("Render missing %q", title)
		}
	}
}

func TestRenderFillsScreenExactly(t *testing.T) {
	m := newTestModel(t, fixtureWS(t))
	for _, size := range [][2]int{{120, 40}, {80, 20}, {201, 57}} {
		m = resize(m, size[0], size[1])
		lines := strings.Split(m.plainRender(), "\n")
		if len(lines) != size[1] {
			t.Fatalf("%v: got %d lines, want %d", size, len(lines), size[1])
		}
		for i, l := range lines {
			if w := displayWidth(l); w > size[0] {
				t.Fatalf("%v: line %d is %d cells wide", size, i, w)
			}
		}
	}
}

func TestInitialFocusCollections(t *testing.T) {
	if got := newTestModel(t, fixtureWS(t)).Focus(); got != PanelCollections {
		t.Fatalf("focus = %v, want PanelCollections", got)
	}
}

func TestNumberKeysJump(t *testing.T) {
	m := newTestModel(t, fixtureWS(t))
	if m = press(m, "4"); m.Focus() != PanelRequest {
		t.Fatalf("after 4: focus = %v", m.Focus())
	}
	if m = press(m, "1"); m.Focus() != PanelEnvs {
		t.Fatalf("after 1: focus = %v", m.Focus())
	}
	if m = press(m, "5"); m.Focus() != PanelResponse {
		t.Fatalf("after 5: focus = %v", m.Focus())
	}
}

func TestZeroFocusesLastMainPanel(t *testing.T) {
	m := newTestModel(t, fixtureWS(t))
	if m = press(m, "0"); m.Focus() != PanelRequest {
		t.Fatalf("0: focus = %v, want PanelRequest by default", m.Focus())
	}
	m = press(m, "5", "2", "0")
	if m.Focus() != PanelResponse {
		t.Fatalf("0 after leaving Response: focus = %v, want the last of [4]/[5]", m.Focus())
	}
	m = press(m, "3", "0")
	if m.Focus() != PanelResponse {
		t.Fatalf("0 from History: focus = %v, want PanelResponse", m.Focus())
	}
	if m = press(m, "1", "0"); m.Focus() != PanelVars {
		t.Fatalf("0 from Envs: focus = %v, want the variables table", m.Focus())
	}
	if m = press(m, "esc"); m.Focus() != PanelEnvs {
		t.Fatalf("esc from vars: focus = %v, want PanelEnvs", m.Focus())
	}
}

func TestTabCycles(t *testing.T) {
	m := press(newTestModel(t, fixtureWS(t)), "1")
	m = press(m, "tab")
	if m.Focus() != PanelCollections {
		t.Fatalf("one tab from Envs: focus = %v", m.Focus())
	}
	m = press(m, "tab", "tab", "tab", "tab")
	if m.Focus() != PanelEnvs {
		t.Fatalf("5 tabs from Envs: focus = %v", m.Focus())
	}
	m = press(m, "shift+tab")
	if m.Focus() != PanelResponse {
		t.Fatalf("shift+tab from Envs: focus = %v", m.Focus())
	}
	m = press(m, "shift+tab")
	if m.Focus() != PanelRequest {
		t.Fatalf("shift+tab from Response: focus = %v", m.Focus())
	}
	if m = press(m, "V", "tab"); m.Focus() != PanelEnvs {
		t.Fatalf("tab from the variables table: focus = %v, want PanelEnvs", m.Focus())
	}
}

func TestHLCycleWithinColumn(t *testing.T) {
	for _, keys := range [][2]string{{"h", "l"}, {"left", "right"}} {
		prev, next := keys[0], keys[1]
		m := press(newTestModel(t, fixtureWS(t)), "1")
		for _, want := range []Panel{PanelCollections, PanelHistory, PanelEnvs} {
			if m = press(m, next); m.Focus() != want {
				t.Fatalf("%s on the left: focus = %v, want %v", next, m.Focus(), want)
			}
		}
		for _, want := range []Panel{PanelHistory, PanelCollections, PanelEnvs} {
			if m = press(m, prev); m.Focus() != want {
				t.Fatalf("%s on the left: focus = %v, want %v", prev, m.Focus(), want)
			}
		}
		m = press(m, "4")
		for _, step := range []struct {
			key  string
			want Panel
		}{{next, PanelResponse}, {next, PanelRequest}, {prev, PanelResponse}, {prev, PanelRequest}} {
			if m = press(m, step.key); m.Focus() != step.want {
				t.Fatalf("%s on the right: focus = %v, want %v", step.key, m.Focus(), step.want)
			}
		}
	}
}

func TestHLKeepTheRequestInContext(t *testing.T) {
	m := press(newTestModel(t, fixtureWS(t)), "j", "enter")
	before := m.selectedRequest()
	m = press(m, "l", "l", "h")
	if after := m.selectedRequest(); after.Name != before.Name || m.lastLeft != PanelCollections {
		t.Fatalf("request in context changed: %q -> %q (lastLeft %v)", before.Name, after.Name, m.lastLeft)
	}
}

func TestHLInVarsTableStay(t *testing.T) {
	for _, k := range []string{"h", "l", "left", "right"} {
		if m := press(newTestModel(t, fixtureWS(t)), "5", "V", k); m.Focus() != PanelVars {
			t.Fatalf("%s in the variables table: focus = %v, want it to stay", k, m.Focus())
		}
	}
}

func TestEnterOpensMainView(t *testing.T) {
	ws := fixtureWS(t)
	m := newTestModel(t, ws)
	if m = press(m, "enter"); m.Focus() != PanelCollections || strings.Contains(m.plainRender(), "list orders") {
		t.Fatalf("enter on a folder should collapse it and keep focus: %v", m.Focus())
	}
	m = press(m, "enter", "j", "enter")
	if m.Focus() != PanelRequest {
		t.Fatalf("enter on a request: focus = %v, want PanelRequest", m.Focus())
	}
	if m = press(m, "esc"); m.Focus() != PanelCollections {
		t.Fatalf("esc in Request: focus = %v, want PanelCollections", m.Focus())
	}
	if m = press(m, "3", "enter"); m.Focus() != PanelHistory {
		t.Fatalf("enter on empty History: focus = %v, want PanelHistory", m.Focus())
	}
	m.history = historyPanel{entries: []history.Entry{{ID: "a", Time: time.Now(), Code: 200}}}
	if m = press(m, "enter"); m.Focus() != PanelResponse {
		t.Fatalf("enter on a History entry: focus = %v, want PanelResponse", m.Focus())
	}
	if m = press(m, "esc"); m.Focus() != PanelHistory {
		t.Fatalf("esc in Response: focus = %v, want PanelHistory", m.Focus())
	}
}

func TestEnvEnterOpensVarsSpaceActivates(t *testing.T) {
	m := press(newTestModel(t, fixtureWS(t)), "1", "k")
	if m = press(m, "enter"); m.Focus() != PanelVars || m.activeEnv().Name != "staging" {
		t.Fatalf("enter: focus = %v active = %s, want vars table and staging", m.Focus(), m.activeEnv().Name)
	}
	m = press(m, "esc", "space")
	if m.Focus() != PanelEnvs || m.activeEnv().Name != "dev" {
		t.Fatalf("space: focus = %v active = %s, want Envs and dev", m.Focus(), m.activeEnv().Name)
	}
}

func TestEscInMainViewCancelsSendFirst(t *testing.T) {
	m := press(newTestModel(t, fixtureWS(t)), "3", "2", "5")
	cancelled := false
	m.flight = &flight{seq: 1, key: "k", cancel: func() { cancelled = true }}
	if m = press(m, "esc"); !cancelled || m.flight != nil || m.Focus() != PanelResponse {
		t.Fatalf("esc while sending: cancelled %v focus %v, want cancel and no move", cancelled, m.Focus())
	}
	if m = press(m, "esc"); m.Focus() != PanelCollections {
		t.Fatalf("second esc: focus = %v, want PanelCollections", m.Focus())
	}
}

func TestTreeNavigation(t *testing.T) {
	m := newTestModel(t, fixtureWS(t))

	if got := m.selectedRowLabel(); got != "orders" {
		t.Fatalf("initial selection = %q, want orders", got)
	}
	m = press(m, "j", "j", "j", "j")
	if got := m.selectedRowLabel(); got != "get user" {
		t.Fatalf("selection = %q, want get user", got)
	}
	m = press(m, "j")
	if got := m.selectedRowLabel(); got != "get user" {
		t.Fatalf("selection past end = %q, want get user", got)
	}
	m = press(m, "k", "k")
	if got := m.selectedRowLabel(); got != "users" {
		t.Fatalf("selection = %q, want users", got)
	}
	if !strings.Contains(m.plainRender(), "get user") {
		t.Fatal("expanded users should show get user")
	}
	m = press(m, "enter")
	out := m.plainRender()
	if strings.Contains(out, "get user") || strings.Contains(out, "create user") {
		t.Fatal("collapsed users still shows its requests")
	}
	if !strings.Contains(out, "▸ users") {
		t.Fatal("collapsed folder should render with ▸")
	}
	if m = press(m, "enter"); !strings.Contains(m.plainRender(), "get user") {
		t.Fatal("re-expanded users should show get user")
	}
}

func TestEnvNavigation(t *testing.T) {
	m := press(newTestModel(t, fixtureWS(t)), "1")
	if got := m.selectedRowLabel(); got != "staging" {
		t.Fatalf("env cursor starts on %q, want active env staging", got)
	}
	if m = press(m, "k"); m.selectedRowLabel() != "dev" {
		t.Fatalf("k: env cursor on %q, want dev", m.selectedRowLabel())
	}
}

func TestActiveEnvFromSession(t *testing.T) {
	m := newTestModel(t, fixtureWS(t))
	if e := m.activeEnv(); e == nil || e.Name != "staging" {
		t.Fatalf("ActiveEnv = %v, want staging", e)
	}
	if !strings.Contains(m.plainRender(), "● staging") {
		t.Fatal("active env should be marked with ●")
	}

	d := testDeps(t, fixtureWS(t))
	d.Session.LastEnv = "gone"
	if e := newModel(d).activeEnv(); e == nil || e.Name != "dev" {
		t.Fatalf("unknown LastEnv: ActiveEnv = %v, want first env dev", e)
	}

	empty, err := workspace.Load(filepath.Join(t.TempDir(), "empty"))
	if err != nil {
		t.Fatal(err)
	}
	if e := newModel(testDeps(t, empty)).activeEnv(); e != nil {
		t.Fatalf("no envs: ActiveEnv = %v, want nil", e)
	}
}

func TestHistoryListsEntries(t *testing.T) {
	ws := fixtureWS(t)
	d := testDeps(t, ws)
	old := history.Entry{Root: ws.Root, Time: time.Now().Add(-5 * time.Minute), Code: 401, Template: workspace.Request{Name: "get user", Method: "GET"}}
	recent := history.Entry{Root: ws.Root, Time: time.Now().Add(-2 * time.Minute), Code: 200, Template: workspace.Request{Name: "get user", Method: "GET"}}
	for _, e := range []history.Entry{old, recent} {
		if err := d.History.Append(e); err != nil {
			t.Fatal(err)
		}
	}
	m := resize(newModel(d), 120, 40)
	out := drain(m, m.Init()).plainRender()
	i200, i401 := strings.Index(out, "200 GET  get user  2m"), strings.Index(out, "401 GET  get user  5m")
	if i200 < 0 || i401 < 0 {
		t.Fatalf("history rows missing:\n%s", out)
	}
	if i200 > i401 {
		t.Fatal("history should list newest first")
	}
}

func TestHelpOverlay(t *testing.T) {
	m := newTestModel(t, fixtureWS(t))
	if strings.Contains(m.plainRender(), "send the selected request") {
		t.Fatal("help visible before ?")
	}
	m = press(m, "?")
	if out := m.plainRender(); !strings.Contains(out, "send the selected request") {
		t.Fatal("help overlay should list send")
	}
	sections := helpSections()
	last := sections[len(sections)-1].bindings
	for range helpMaxScroll(40) + 1 {
		lines := strings.Split(m.plainRender(), "\n")
		if len(lines) != 40 {
			t.Fatalf("help overlay render has %d lines", len(lines))
		}
		for i, l := range lines {
			if w := displayWidth(l); w > 120 {
				t.Fatalf("line %d is %d cells wide", i, w)
			}
		}
		m = press(m, "j")
	}
	if out := m.plainRender(); !strings.Contains(out, last[len(last)-1].help) {
		t.Fatalf("scrolled to the end, help should show the last section:\n%s", out)
	}
	if m = press(m, "?"); strings.Contains(m.plainRender(), "send the selected request") {
		t.Fatal("second ? should close help")
	}
	if m = press(m, "?", "esc"); strings.Contains(m.plainRender(), "send the selected request") {
		t.Fatal("esc should close help")
	}
}

func TestQuit(t *testing.T) {
	m := newTestModel(t, fixtureWS(t))
	_, cmd := m.Update(keyMsg("q"))
	if cmd == nil {
		t.Fatal("q returned no command")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("q command did not produce tea.QuitMsg")
	}
}

func TestTinyTerminal(t *testing.T) {
	m := newTestModel(t, fixtureWS(t))
	if out := resize(m, 60, 15).plainRender(); !strings.Contains(out, "terminal too small") {
		t.Fatalf("60×15 render = %q", out)
	}
	if out := resize(m, 79, 40).plainRender(); !strings.Contains(out, "terminal too small (need 80×20)") {
		t.Fatalf("79×40 render = %q", out)
	}
	_ = resize(m, 0, 0).plainRender()
	_ = press(resize(m, 0, 0), "?").plainRender()
}

func TestLoadErrorsShownInStatus(t *testing.T) {
	ws := fixtureWS(t)
	bad := filepath.Join(ws.Root, "collections", "users", "broken.yaml")
	ws.LoadErrors = []workspace.FileError{{Path: bad, Err: errors.New("yaml: bad")}}
	m := newTestModel(t, ws)
	if !strings.Contains(m.Status(), "1 file(s) failed to load") {
		t.Fatalf("status = %q", m.Status())
	}
	if !strings.Contains(m.Status(), "users/broken.yaml: yaml: bad") {
		t.Fatalf("status should name the first failing file and its error: %q", m.Status())
	}
	if !strings.Contains(m.plainRender(), "1 file(s) failed to load") {
		t.Fatal("status line not rendered")
	}
}

func TestEmptyStateHint(t *testing.T) {
	ws, err := workspace.Load(filepath.Join(t.TempDir(), "fresh"))
	if err != nil {
		t.Fatal(err)
	}
	d := testDeps(t, ws)
	m := resize(newModel(d), 120, 40)
	if !strings.Contains(m.Status(), "press n") {
		t.Fatalf("status = %q", m.Status())
	}
	_ = press(m, "j", "enter", "k", "1", "j", "3", "j").plainRender()

	rebind(t, func(k *config.Keymap) { k.New = config.Keys{"x"} })
	if m := newModel(testDeps(t, ws)); !strings.Contains(m.Status(), "press x") {
		t.Fatalf("rebound status = %q", m.Status())
	}
}

func TestHintLineFollowsFocus(t *testing.T) {
	m := newTestModel(t, fixtureWS(t))
	if !strings.Contains(lastLine(m.plainRender()), "send: s") {
		t.Fatalf("collections hint line = %q", lastLine(m.plainRender()))
	}
	m = press(m, "3")
	if !strings.Contains(lastLine(m.plainRender()), "resend: s") {
		t.Fatalf("history hint line = %q", lastLine(m.plainRender()))
	}
}

func lastLine(s string) string {
	lines := strings.Split(s, "\n")
	return lines[len(lines)-1]
}

func TestHelpOverlayFitsMinimumTerminal(t *testing.T) {
	m := press(resize(newTestModel(t, fixtureWS(t)), 80, 20), "?")
	for range helpMaxScroll(20) + 2 {
		lines := strings.Split(m.plainRender(), "\n")
		if len(lines) != 20 {
			t.Fatalf("help overlay render has %d lines", len(lines))
		}
		for i, l := range lines {
			if w := displayWidth(l); w > 80 {
				t.Fatalf("line %d is %d cells wide", i, w)
			}
		}
		m = press(m, "j")
	}
}

func TestHelpOverlayScrolls(t *testing.T) {
	m := press(resize(newTestModel(t, fixtureWS(t)), 80, 20), "?")
	out := m.plainRender()
	if !strings.Contains(out, "↓ more") {
		t.Fatalf("80×20 help should show ↓ more:\n%s", out)
	}
	if strings.Contains(out, "copy body") {
		t.Fatal("the end of the Response section should start below the fold")
	}
	seen := map[string]bool{}
	for range helpMaxScroll(20) {
		m = press(m, "j")
		out = m.plainRender()
		for _, b := range allBindings() {
			if strings.Contains(out, b.help) {
				seen[b.help] = true
			}
		}
	}
	if !strings.Contains(out, "copy body") || strings.Contains(out, "↓ more") {
		t.Fatalf("scrolled to the end, help should show the Response section without ↓ more:\n%s", out)
	}
	for _, b := range allBindings() {
		if !seen[b.help] {
			t.Errorf("scrolling never revealed %q", b.help)
		}
	}
	if m = press(m, "k"); !strings.Contains(m.plainRender(), "↓ more") {
		t.Fatal("k should scroll back up")
	}
	if m.Focus() != PanelCollections {
		t.Fatal("j/k in help must not move panel focus")
	}
}

func TestHelpGroupedByPanel(t *testing.T) {
	out := ansiSeq.ReplaceAllString(strings.Join(helpLines(false), "\n"), "")
	prev := -1
	for _, title := range []string{"Global", "Collections", "Request", "Response", "History"} {
		i := strings.Index(out, " "+title+"\n")
		if i < 0 || i < prev {
			t.Fatalf("help section %q missing or out of order:\n%s", title, out)
		}
		prev = i
	}
}

func TestHintLinePerPanel(t *testing.T) {
	m := newTestModel(t, fixtureWS(t))
	if m = press(m, "5"); !strings.Contains(lastLine(m.plainRender()), "search: /") {
		t.Fatalf("response hint line = %q", lastLine(m.plainRender()))
	}
	m = press(m, "2")
	hint := lastLine(m.plainRender())
	if strings.Contains(hint, "search: /") || !strings.Contains(hint, "expand/open: enter") {
		t.Fatalf("collections hint line = %q", hint)
	}
	if m = press(m, "4"); !strings.Contains(lastLine(m.plainRender()), "url: u") {
		t.Fatalf("request hint line = %q", lastLine(m.plainRender()))
	}
	works := map[string][]string{
		"$EDITOR: e": {"2", "4"}, "new: n": {"1", "2"}, "delete: d": {"1", "2", "4"}, "save: w": {"2", "4"},
	}
	for hint, panels := range works {
		for _, p := range []string{"1", "2", "3", "4", "5"} {
			listed := strings.Contains(lastLine(press(m, p).plainRender()), hint)
			if want := slices.Contains(panels, p); listed != want {
				t.Errorf("panel %s: hint %q listed = %v, want %v", p, hint, listed, want)
			}
		}
	}
}

func allBindings() []binding {
	var out []binding
	for _, s := range helpSections() {
		out = append(out, s.bindings...)
	}
	return out
}

func TestHintLines(t *testing.T) {
	base := resize(newTestModel(t, fixtureWS(t)), 400, 30)
	for _, tc := range []struct {
		keys []string
		want string
	}{
		{[]string{"1"}, "activate: space | variables: enter | new: n | delete: d | new unsaved request: N | keybinds: ?"},
		{[]string{"2"}, "send: s | expand/open: enter | new: n | delete: d | $EDITOR: e | save: w | save as: W | copy cURL: c | import cURL: i | new unsaved request: N | keybinds: ?"},
		{[]string{"3"}, "response: enter | resend: s | new unsaved request: N | keybinds: ?"},
		{[]string{"2", "j", "enter"}, "edit: enter | toggle: space | add: a | delete: d | method: m | url: u | $EDITOR: e | save: w | save as: W | copy cURL: c | import cURL: i | send: s | new unsaved request: N | keybinds: ?"},
		{[]string{"5"}, "search: / | next/prev: n/N | copy: y | send: s | keybinds: ?"},
		{[]string{"1", "enter"}, "edit value: enter | paste clipboard: p | reveal: r | add: a | delete: d | secret: S | keybinds: ?"},
		{[]string{"2", "j", "enter", "m"}, "set method: enter"},
		{[]string{"2", "j", "enter", "A", "enter"}, "set auth type: enter"},
		{[]string{"E"}, "filter: / | activate: enter"},
	} {
		if got := strings.TrimSpace(lastLine(press(base, tc.keys...).plainRender())); got != tc.want {
			t.Errorf("%v: hint line\n got %q\nwant %q", tc.keys, got, tc.want)
		}
	}
}
