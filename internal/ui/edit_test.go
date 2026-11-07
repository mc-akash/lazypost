package ui

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/mc-akash/lazypost/internal/state"
	"github.com/mc-akash/lazypost/internal/workspace"
)

const (
	getUserKey    = "collections/users/get-user.yaml"
	listOrdersKey = "collections/orders/list-orders.yaml"
)

type headerRec struct {
	mu     sync.Mutex
	accept []string
	paths  []string
}

func (h *headerRec) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	h.accept = append(h.accept, r.Header.Get("Accept"))
	h.paths = append(h.paths, r.URL.Path)
	h.mu.Unlock()
}

func (h *headerRec) last() (accept, path string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.paths) == 0 {
		return "", ""
	}
	return h.accept[len(h.accept)-1], h.paths[len(h.paths)-1]
}

func acceptHeaderWS(t *testing.T) (*workspace.Workspace, *workspace.Request) {
	t.Helper()
	ws := fixtureWS(t)
	req := requestByName(t, folderByName(t, ws, "users"), "get user")
	req.Headers = []workspace.KV{{Key: "Accept", Value: "application/json"}}
	if err := ws.SaveRequest(req); err != nil {
		t.Fatal(err)
	}
	return ws, req
}

func currentTmpl(m Model) workspace.Request {
	tmpl := m.selectedRequest()
	return tmpl
}

func TestToggleHeaderCreatesDraftAndMarker(t *testing.T) {
	ws, req := acceptHeaderWS(t)
	before := readFile(t, req.Path)
	m, _ := sendModel(t, ws, "http://example.invalid")
	m = press(m, "4", "space")
	out := m.plainRender()
	if !strings.Contains(out, "get user ●") || !strings.Contains(out, "[4] Request ●") {
		t.Fatalf("draft marker missing:\n%s", out)
	}
	if !currentTmpl(m).Headers[0].Disabled {
		t.Fatal("space should disable the header in the draft")
	}
	if readFile(t, req.Path) != before || req.Headers[0].Disabled {
		t.Fatal("a draft must not touch the file or the saved request")
	}
	if d := m.drafts[getUserKey]; d.BaseModTime.IsZero() {
		t.Fatalf("draft BaseModTime not set: %+v", d)
	}
}

func TestSendUsesDraft(t *testing.T) {
	rec := &headerRec{}
	srv := serve(t, rec)
	ws, _ := acceptHeaderWS(t)
	m, _ := sendModel(t, ws, srv.URL)
	m = pressRun(m, "s")
	if accept, _ := rec.last(); accept != "application/json" {
		t.Fatalf("saved request: Accept = %q", accept)
	}
	m = pressRun(press(m, "4", "space"), "s")
	if accept, _ := rec.last(); accept != "" {
		t.Fatalf("draft disabled Accept, server saw %q", accept)
	}
	if _, ok := m.drafts[getUserKey]; !ok || len(m.results) != 1 {
		t.Fatalf("drafts %v results %v", m.drafts, m.results)
	}
}

func TestWriteSavesAndClearsDraft(t *testing.T) {
	ws, req := acceptHeaderWS(t)
	m, _ := sendModel(t, ws, "http://example.invalid")
	m = press(m, "4", "space", "w")
	if !strings.Contains(readFile(t, req.Path), "disabled: true") {
		t.Fatalf("file not written:\n%s", readFile(t, req.Path))
	}
	if out := m.plainRender(); strings.Contains(out, "get user ●") || strings.Contains(out, "Request ●") || len(m.drafts) != 0 {
		t.Fatal("w should clear the draft and its marker")
	}
	if m.Status() != "saved" {
		t.Fatalf("status = %q, want saved", m.Status())
	}
	if !req.Headers[0].Disabled {
		t.Fatal("the in-memory request should match the file")
	}
}

func TestUEditsURLAndKeepsDraft(t *testing.T) {
	ws, _ := acceptHeaderWS(t)
	m, _ := sendModel(t, ws, "http://example.invalid")
	m = press(m, "4", "space", "u")
	if m.input() == nil || m.input().title != dialogEditURL {
		t.Fatalf("u in [4] should open the URL dialog, input = %+v", m.input())
	}
	m = press(m, "esc", "2", "u")
	if m.input() != nil || m.prompt() != nil {
		t.Fatal("u in Collections should do nothing")
	}
	if !strings.Contains(m.plainRender(), "get user ●") || len(m.drafts) != 1 {
		t.Fatal("u must not discard the draft")
	}
}

func TestShiftUDoesNothing(t *testing.T) {
	m, _ := sendModel(t, fixtureWS(t), "http://example.invalid")
	m = press(m, "4", "U")
	if m.input() != nil || m.picker() != nil || m.prompt() != nil {
		t.Fatal("U should no longer open anything")
	}
}

func TestMethodPickAndURLEdit(t *testing.T) {
	m, _ := sendModel(t, fixtureWS(t), "http://example.invalid")
	m = press(m, "4", "m", "j", "enter")
	if got := currentTmpl(m).Method; got != "POST" {
		t.Fatalf("method = %q, want POST", got)
	}
	m = press(m, "m", "k", "enter")
	if got := currentTmpl(m).Method; got != "GET" {
		t.Fatalf("method should be back to GET, got %q", got)
	}
	m = press(m, "u")
	if m.input() == nil || m.input().ti.Value() != "{{base_url}}/users/1" {
		t.Fatal("u should open an input holding the URL")
	}
	m = press(typeText(press(m, "ctrl+u"), "{{base_url}}/users/2"), "enter")
	if got := currentTmpl(m).URL; got != "{{base_url}}/users/2" {
		t.Fatalf("URL = %q", got)
	}
}

func TestRowEditing(t *testing.T) {
	ws, _ := acceptHeaderWS(t)
	m, _ := sendModel(t, ws, "http://example.invalid")
	m = press(m, "4", "enter")
	if m.input() == nil || m.input().ti.Value() != "Accept: application/json" {
		t.Fatal("enter should edit the row as key: value")
	}
	m = press(typeText(press(m, "ctrl+u"), "Accept:text/plain"), "enter")
	m = addHeader(t, m, "X-Id: 7")
	want := []workspace.KV{{Key: "Accept", Value: "text/plain"}, {Key: "X-Id", Value: "7"}}
	if got := currentTmpl(m).Headers; len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("headers = %+v, want %+v", got, want)
	}
	m = press(typeText(press(m, "enter", "ctrl+u"), ": nothing"), "enter")
	if !strings.Contains(m.Status(), "empty key") || len(currentTmpl(m).Headers) != 2 {
		t.Fatalf("empty key should cancel with a status, got %q", m.Status())
	}
	m = press(m, "j", "d")
	if got := currentTmpl(m).Headers; len(got) != 1 || got[0].Key != "Accept" {
		t.Fatalf("d should delete the selected row, headers = %+v", got)
	}
	m = press(m, "]", "]", "]", "a")
	m = press(typeText(m, "page: 2"), "enter")
	if got := currentTmpl(m).Params; len(got) != 1 || got[0] != (workspace.KV{Key: "page", Value: "2"}) {
		t.Fatalf("params = %+v", got)
	}
}

func TestBodyTypeCycleAndAuthPicker(t *testing.T) {
	m, _ := sendModel(t, fixtureWS(t), "http://example.invalid")
	m = press(m, "4", "]", "t")
	if got := currentTmpl(m).Body.Type; got != "json" {
		t.Fatalf("body type = %q, want json", got)
	}
	m = press(m, "t", "t", "t", "t")
	if got := currentTmpl(m).Body.Type; got != "none" {
		t.Fatalf("body type should wrap to none, got %q", got)
	}
	m = press(typeText(press(m, "]", "enter"), "bearer"), "enter")
	if got := currentTmpl(m).Auth.Type; got != "bearer" {
		t.Fatalf("auth type = %q, want bearer", got)
	}
	m = press(typeText(press(m, "j", "enter"), "{{token}}"), "enter")
	if got := currentTmpl(m).Auth.Token; got != "{{token}}" {
		t.Fatalf("token = %q", got)
	}
	m = press(typeText(press(m, "k", "enter"), "inherit"), "enter")
	if got := currentTmpl(m).Auth.Type; got != "" {
		t.Fatalf("auth type should be back to inherit, got %q", got)
	}
}

func TestQuitPersistsDraftsWithoutPrompt(t *testing.T) {
	for _, key := range []string{"q", "ctrl+c"} {
		t.Run(key, func(t *testing.T) {
			ws, _ := acceptHeaderWS(t)
			m, _ := sendModel(t, ws, "http://example.invalid")
			m = press(m, "4", "space")
			next, cmd := m.Update(keyMsg(key))
			if cmd == nil {
				t.Fatal("no command")
			}
			if _, ok := cmd().(tea.QuitMsg); !ok {
				t.Fatal("quit must not prompt")
			}
			if next.(Model).prompt() != nil {
				t.Fatal("quit opened a prompt")
			}
			sess, err := m.deps.StateStore.Load(ws.Root)
			if err != nil {
				t.Fatal(err)
			}
			d, ok := sess.Drafts[getUserKey]
			if !ok || !d.Request.Headers[0].Disabled {
				t.Fatalf("draft not persisted: %+v", sess.Drafts)
			}
			if sess.LastRequest != getUserKey || sess.LastEnv != "staging" {
				t.Fatalf("session = %+v", sess)
			}
		})
	}
}

func TestRestoreSelectsLastRequestAndEnv(t *testing.T) {
	rec := &headerRec{}
	srv := serve(t, rec)
	ws := fixtureWS(t)
	envByName(t, ws, "dev").Vars["base_url"] = srv.URL
	d := testDeps(t, ws)
	d.Session = state.Session{LastEnv: "dev", LastRequest: listOrdersKey, Drafts: map[string]state.Draft{}}
	m := resize(newModel(d), 120, 40)
	if got := m.selectedRowLabel(); got != "list orders" {
		t.Fatalf("selected %q, want list orders", got)
	}
	pressRun(m, "s")
	if _, path := rec.last(); path != "/orders" {
		t.Fatalf("server saw path %q, want /orders", path)
	}
}

func TestRestoreRevealsCollapsedFolder(t *testing.T) {
	ws := fixtureWS(t)
	d := testDeps(t, ws)
	d.Session = state.Session{LastRequest: getUserKey, Drafts: map[string]state.Draft{}}
	m := newModel(d)
	if got := m.selectedRowLabel(); got != "get user" {
		t.Fatalf("selected %q, want get user", got)
	}
	users := folderByName(t, ws, "users")
	m.collections.collapsed = map[string]bool{users.Path: true}
	m.collections = m.collections.reveal(ws.Collections, filepath.Join(ws.Root, getUserKey))
	if got := m.selectedRowLabel(); got != "get user" {
		t.Fatalf("reveal selected %q, want get user", got)
	}
}

func draftOf(t *testing.T, ws *workspace.Workspace, key string, base time.Time) state.Draft {
	t.Helper()
	r := ws.FindRequest(filepath.Join(ws.Root, key))
	if r == nil {
		t.Fatalf("no request %s", key)
	}
	req := *r
	req.Method = "POST"
	return state.Draft{Request: req, BaseModTime: base}
}

func TestChangedOnDiskMarker(t *testing.T) {
	ws := fixtureWS(t)
	fi, err := os.Stat(filepath.Join(ws.Root, listOrdersKey))
	if err != nil {
		t.Fatal(err)
	}
	d := testDeps(t, ws)
	d.Session.Drafts = map[string]state.Draft{
		listOrdersKey: draftOf(t, ws, listOrdersKey, fi.ModTime().Add(-time.Hour)),
		getUserKey:    draftOf(t, ws, getUserKey, modTime(t, filepath.Join(ws.Root, getUserKey))),
	}
	m := resize(newModel(d), 120, 40)
	out := m.plainRender()
	if !strings.Contains(out, "list orders !") || !strings.Contains(out, "get user ●") {
		t.Fatalf("markers wrong:\n%s", out)
	}
	if want := "changed on disk: list orders (w overwrites)"; !strings.Contains(m.Status(), want) {
		t.Fatalf("status = %q, want %q", m.Status(), want)
	}
	if currentTmpl(press(m, "j")).Method != "POST" {
		t.Fatal("restored draft should be in effect")
	}
}

func modTime(t *testing.T, path string) time.Time {
	t.Helper()
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return fi.ModTime()
}

func TestOrphanDraftDropped(t *testing.T) {
	ws := fixtureWS(t)
	d := testDeps(t, ws)
	d.Session.Drafts = map[string]state.Draft{"collections/gone.yaml": {Request: workspace.Request{Name: "gone"}}}
	m := newModel(d)
	if !strings.Contains(m.Status(), "dropped 1 orphan draft(s)") {
		t.Fatalf("status = %q", m.Status())
	}
	if len(m.drafts) != 0 {
		t.Fatalf("drafts = %v", m.drafts)
	}
}

func TestNewRequestAndFolder(t *testing.T) {
	ws := fixtureWS(t)
	m := press(newTestModel(t, ws), "j", "j", "n")
	if !strings.Contains(m.plainRender(), "┌"+dialogNewRequest) {
		t.Fatal("n should prompt for a name")
	}
	m = press(typeText(m, "Ping"), "enter")
	if _, err := os.Stat(filepath.Join(ws.Root, "collections/users/ping.yaml")); err != nil {
		t.Fatalf("request not created: %v", err)
	}
	if got := m.selectedRowLabel(); got != "Ping" {
		t.Fatalf("selected %q, want Ping", got)
	}
	m = press(typeText(press(m, "n"), "admin/"), "enter")
	if fi, err := os.Stat(filepath.Join(ws.Root, "collections/users/admin")); err != nil || !fi.IsDir() {
		t.Fatalf("folder not created: %v", err)
	}
	if got := m.selectedRowLabel(); got != "admin" {
		t.Fatalf("selected %q, want admin", got)
	}
	m = press(m, "n", "enter")
	if m.input() != nil || len(folderByName(t, ws, "users").Requests) != 3 {
		t.Fatal("an empty name should create nothing")
	}
}

func TestDeleteRequestAsksOnce(t *testing.T) {
	ws, req := acceptHeaderWS(t)
	m, _ := sendModel(t, ws, "http://example.invalid")
	m = press(m, "4", "space", "2", "d")
	if !strings.Contains(m.plainRender(), `Delete "get user"?`) {
		t.Fatal("delete confirmation missing")
	}
	m = press(m, "n")
	if _, err := os.Stat(req.Path); err != nil {
		t.Fatal("n should keep the request")
	}
	m = press(m, "d", "y")
	if _, err := os.Stat(req.Path); !os.IsNotExist(err) {
		t.Fatal("y should delete the request")
	}
	if len(m.drafts) != 0 {
		t.Fatal("the deleted request's draft should go too")
	}
	orders := folderByName(t, ws, "orders")
	m = press(m, "k", "k", "k", "d", "y")
	if _, err := os.Stat(orders.Path); !os.IsNotExist(err) {
		t.Fatal("deleting a folder should remove it recursively")
	}
}

func TestEditorCommandResolution(t *testing.T) {
	cases := []struct {
		visual, editor string
		want           []string
	}{
		{"code -w", "nano", []string{"code", "-w", "/tmp/f"}},
		{"", "nano", []string{"nano", "/tmp/f"}},
		{"", "", []string{"vi", "/tmp/f"}},
	}
	for _, c := range cases {
		t.Setenv("VISUAL", c.visual)
		t.Setenv("EDITOR", c.editor)
		cmd, err := editorCommand("/tmp/f")
		if err != nil {
			t.Fatal(err)
		}
		if strings.Join(cmd.Args, "|") != strings.Join(c.want, "|") {
			t.Errorf("VISUAL=%q EDITOR=%q → %q, want %q", c.visual, c.editor, cmd.Args, c.want)
		}
	}
	t.Setenv("VISUAL", `"unterminated`)
	if _, err := editorCommand("/tmp/f"); err == nil {
		t.Error("an unparsable editor should be an error")
	}
}

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "edit")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestApplyEditedRequestInvalidYAMLKeepsDraft(t *testing.T) {
	m, _ := sendModel(t, fixtureWS(t), "http://example.invalid")
	m = press(m, "4", "m", "j", "enter")
	next := m.applyEditedRequest(writeTemp(t, "method: [unclosed"))
	if !strings.HasPrefix(next.Status(), "invalid YAML: ") || currentTmpl(next).Method != "POST" {
		t.Fatalf("status %q, method %q", next.Status(), currentTmpl(next).Method)
	}
	next = m.applyEditedRequest(writeTemp(t, "name: get user\nmethod: PUT\nurl: http://x/y\n"))
	if tmpl := currentTmpl(next); tmpl.Method != "PUT" || tmpl.URL != "http://x/y" {
		t.Fatalf("draft = %+v", tmpl)
	}
}

func TestSetBodyContent(t *testing.T) {
	m, _ := sendModel(t, fixtureWS(t), "http://example.invalid")
	m = m.setBodyContent(`{"a":1}`)
	if got := currentTmpl(m).Body.Content; got != `{"a":1}` {
		t.Fatalf("body = %q", got)
	}
	if !strings.Contains(m.plainRender(), "get user ●") {
		t.Fatal("body edit should create a draft")
	}
}

func TestEditorRoundTrip(t *testing.T) {
	m, _ := sendModel(t, fixtureWS(t), "http://example.invalid")
	path, err := m.requestTempFile()
	if err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(path); err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("temp file %s: %v %v", path, fi, err)
	}
	if !strings.Contains(readFile(t, path), "url: '{{base_url}}/users/1'") {
		t.Fatalf("temp YAML:\n%s", readFile(t, path))
	}
	unchanged := drain(m, func() tea.Msg { return editorDoneMsg{kind: editRequest, path: path} })
	if len(unchanged.drafts) != 0 {
		t.Fatal("saving the editor unchanged should not create a draft")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("temp file should be removed after reading")
	}
}

func TestDeleteDropsStoredResponses(t *testing.T) {
	srv := serve(t, &recorder{contentType: "text/plain", body: []byte("hello")})
	for _, folder := range []bool{false, true} {
		m, _ := sendModel(t, fixtureWS(t), srv.URL)
		m = pressRun(m, "s")
		if len(m.results) != 1 {
			t.Fatalf("setup: results = %v", m.results)
		}
		if folder {
			m = press(m, "k", "k")
			if m.selectedRowLabel() != "users" {
				t.Fatalf("setup: selected %q", m.selectedRowLabel())
			}
		}
		m = press(m, "d", "y")
		if len(m.results) != 0 {
			t.Fatalf("folder=%v: results left after delete: %v", folder, m.results)
		}
	}
}
