package ui

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/mc-akash/lazypost/internal/history"
	"github.com/mc-akash/lazypost/internal/workspace"
)

const unsavedTitle = "[4] Request · unsaved"

func emptyWS(t *testing.T) *workspace.Workspace {
	t.Helper()
	ws, err := workspace.Load(filepath.Join(t.TempDir(), "empty"))
	if err != nil {
		t.Fatal(err)
	}
	return ws
}

type scratchRec struct {
	mu                  sync.Mutex
	hits                int
	method, path, token string
	body                string
}

func (rec *scratchRec) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rec.mu.Lock()
	rec.hits++
	rec.method, rec.path, rec.token = r.Method, r.URL.Path, r.Header.Get("X-Token")
	body := rec.body
	rec.mu.Unlock()
	w.Header().Set("Content-Type", "text/plain")
	_, _ = w.Write([]byte(body))
}

func (rec *scratchRec) last() (hits int, method, path, token string) {
	rec.mu.Lock()
	defer rec.mu.Unlock()
	return rec.hits, rec.method, rec.path, rec.token
}

func newScratch(t *testing.T, m Model, url string) Model {
	t.Helper()
	m = press(m, "N")
	if m.input() == nil || m.input().title != dialogEditURL {
		t.Fatalf("N should open the URL dialog, input = %+v", m.input())
	}
	return submitInput(t, m, url)
}

func requestFiles(t *testing.T, ws *workspace.Workspace) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(ws.Collections.Path, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			out = append(out, p)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestScratchNewInEmptyWorkspace(t *testing.T) {
	m := newTestModel(t, emptyWS(t))
	m = press(m, "N")
	if m.scratch == nil {
		t.Fatal("N should create the unsaved request")
	}
	if m.input() == nil || m.input().title != dialogEditURL || m.input().ti.Value() != "" {
		t.Fatalf("N should open an empty URL dialog, input = %+v", m.input())
	}
	if m.Focus() != PanelRequest {
		t.Fatalf("focus = %v, want [4]", m.Focus())
	}
	m = submitInput(t, m, "https://api.example/ping")
	if s := m.scratch; s.Method != "GET" || s.URL != "https://api.example/ping" {
		t.Fatalf("scratch = %+v", s)
	}
	out := m.plainRender()
	for _, want := range []string{unsavedTitle, "https://api.example/ping"} {
		if !strings.Contains(out, want) {
			t.Fatalf("render lacks %q:\n%s", want, out)
		}
	}
	if !m.requireEditableOK() {
		t.Fatal("the unsaved request should be editable")
	}
}

func (m Model) requireEditableOK() bool {
	_, ok := m.requireEditable()
	return ok
}

func TestScratchNAgainKeepsEdits(t *testing.T) {
	m := newScratch(t, newTestModel(t, emptyWS(t)), "http://x.example/a")
	m = press(m, "m", "j", "enter")
	m = press(m, "1", "N")
	if m.input() != nil {
		t.Fatal("N on an existing unsaved request must not open the URL dialog")
	}
	if m.Focus() != PanelRequest {
		t.Fatalf("focus = %v, want [4]", m.Focus())
	}
	if s := m.scratch; s.Method != "POST" || s.URL != "http://x.example/a" {
		t.Fatalf("scratch reset: %+v", s)
	}
}

func TestScratchEditAndSendWithEnv(t *testing.T) {
	rec := &scratchRec{body: "pong"}
	srv := serve(t, rec)
	m, histPath := sendModel(t, fixtureWS(t), srv.URL)
	m = newScratch(t, m, "{{base_url}}/things/{{api_ver}}")
	m = press(m, "m", "j", "enter")
	m = addHeader(t, m, "X-Token: {{token}}")
	m = pressRun(m, "s")

	hits, method, path, token := rec.last()
	if hits != 1 || method != "POST" || path != "/things/v1" || token != "staging-secret-token" {
		t.Fatalf("server got %d hit(s): %s %s token %q", hits, method, path, token)
	}
	out := m.plainRender()
	for _, want := range []string{unsavedTitle, "200 OK", "pong", "200 POST unsaved"} {
		if !strings.Contains(out, want) {
			t.Fatalf("render lacks %q:\n%s", want, out)
		}
	}
	if raw := readFile(t, histPath); strings.Contains(raw, "staging-secret-token") {
		t.Fatalf("history leaks the token:\n%s", raw)
	}
	entries, err := history.Open(histPath).Load()
	if err != nil || len(entries) != 1 {
		t.Fatalf("entries = %d, err = %v", len(entries), err)
	}
	e := entries[0]
	if e.Template.Name != scratchName || e.RequestPath != "" || e.Template.URL != "{{base_url}}/things/{{api_ver}}" || e.Env != "staging" {
		t.Fatalf("entry = %+v", e)
	}
}

func TestScratchSendWithoutEnv(t *testing.T) {
	rec := &scratchRec{body: "ok"}
	srv := serve(t, rec)
	m := newTestModel(t, fixtureWS(t))
	m.envName = ""
	m = newScratch(t, m, srv.URL+"/v/{{api_ver}}")
	m = addHeader(t, m, "X-Token: {{token}}")
	m = pressRun(m, "s")

	hits, _, path, token := rec.last()
	if hits != 1 || path != "/v/v1" || token != "{{token}}" {
		t.Fatalf("server got %d hit(s): %s token %q", hits, path, token)
	}
	if !strings.Contains(m.Status(), "missing: token") {
		t.Fatalf("status = %q", m.Status())
	}
	if !strings.Contains(m.plainRender(), "200 OK") {
		t.Fatal("response not shown in [5]")
	}
}

func TestScratchSendInEmptyWorkspace(t *testing.T) {
	rec := &scratchRec{body: "hello"}
	srv := serve(t, rec)
	m := newScratch(t, newTestModel(t, emptyWS(t)), srv.URL+"/empty")
	m = pressRun(m, "s")
	if hits, _, path, _ := rec.last(); hits != 1 || path != "/empty" {
		t.Fatalf("server got %d hit(s) on %s", hits, path)
	}
	if out := m.plainRender(); !strings.Contains(out, "hello") || !strings.Contains(out, "200 GET  unsaved") {
		t.Fatalf("render:\n%s", out)
	}
}

func TestScratchHistoryResend(t *testing.T) {
	rec := &scratchRec{body: "ok"}
	srv := serve(t, rec)
	m, histPath := sendModel(t, fixtureWS(t), srv.URL)
	m = newScratch(t, m, "{{base_url}}/once")
	m = pressRun(m, "s")
	m = m.dropScratch()
	if m.scratch != nil {
		t.Fatal("scratch not discarded")
	}
	m = press(m, "3", "k")
	if e, ok := m.history.selected(); !ok || e.Template.Name != scratchName {
		t.Fatalf("history selection = %+v", e)
	}
	envByName(t, m.ws(), "staging").Vars["base_url"] = srv.URL + "/fresh"
	m = pressRun(m, "s")

	if hits, _, path, _ := rec.last(); hits != 2 || path != "/fresh/once" {
		t.Fatalf("resend: %d hit(s), path %s", hits, path)
	}
	entries, err := history.Open(histPath).Load()
	if err != nil || len(entries) != 2 {
		t.Fatalf("entries = %d, err = %v", len(entries), err)
	}
	for _, e := range entries {
		if e.Template.Name != scratchName || e.RequestPath != "" {
			t.Fatalf("entry = %+v", e)
		}
	}
	if _, ok := m.results[""]; ok {
		t.Fatal("a result stored under an empty request path")
	}
	if out := m.plainRender(); !strings.Contains(out, "200 OK") {
		t.Fatalf("resent entry's response not shown:\n%s", out)
	}
}

func TestScratchSaveAs(t *testing.T) {
	for _, key := range []string{"w", "W"} {
		t.Run(key, func(t *testing.T) {
			ws := fixtureWS(t)
			m := newTestModel(t, ws)
			m = press(m, "3")
			m = newScratch(t, m, "{{base_url}}/charges")
			m = addHeader(t, m, "Authorization: Bearer {{token}}")

			m = press(m, key)
			if m.input() == nil || m.input().title != dialogSaveAs || m.input().ti.Value() != "" {
				t.Fatalf("%s should open an empty Save as dialog, input = %+v", key, m.input())
			}
			m = submitInput(t, m, "Billing/v2/charge it")

			want := filepath.Join(ws.Root, "collections", "billing", "v2", "charge-it.yaml")
			file := readFile(t, want)
			for _, s := range []string{"name: charge it", "url: '{{base_url}}/charges'", "Bearer {{token}}"} {
				if !strings.Contains(file, s) {
					t.Fatalf("file lacks %q:\n%s", s, file)
				}
			}
			if strings.Contains(file, "staging-secret-token") {
				t.Fatalf("file holds the resolved secret:\n%s", file)
			}
			if m.scratch != nil || m.deps.Session.Scratch != nil {
				t.Fatal("unsaved request should be gone after saving")
			}
			if sel := selectedReq(t, m); sel.Path != want || sel.Name != "charge it" {
				t.Fatalf("selected %q (%s)", sel.Name, sel.Path)
			}
			if s, _ := m.selection(); s.file == nil || s.file.Path != want {
				t.Fatalf("[4] shows %+v, want the new request", file)
			}
			if m.requestTitle() != reqTitle {
				t.Fatalf("title = %q", m.requestTitle())
			}
			if m.Status() != "saved as billing/v2/charge it" {
				t.Fatalf("status = %q", m.Status())
			}
		})
	}
}

func TestScratchSaveAsCancel(t *testing.T) {
	ws := fixtureWS(t)
	before := requestFiles(t, ws)
	m := newScratch(t, newTestModel(t, ws), "http://x.example/keep")
	m = press(m, "w", "esc")
	if m.scratch == nil || m.scratch.URL != "http://x.example/keep" || m.requestTitle() != unsavedTitle {
		t.Fatalf("esc changed the scratch: %+v", m.scratch)
	}
	m = submitInput(t, press(m, "W"), "  ")
	if m.Status() != statusSaveAsCancel || m.scratch == nil {
		t.Fatalf("status %q, scratch %+v", m.Status(), m.scratch)
	}
	if after := requestFiles(t, ws); !slices.Equal(before, after) {
		t.Fatalf("files changed: %v -> %v", before, after)
	}
}

func TestScratchUEditsURL(t *testing.T) {
	m := newScratch(t, newTestModel(t, fixtureWS(t)), "http://x.example/a")
	m = press(m, "u")
	if m.prompt() != nil {
		t.Fatalf("u must not ask to discard, prompt = %+v", m.prompt())
	}
	if m.input() == nil || m.input().title != dialogEditURL || m.input().ti.Value() != "http://x.example/a" {
		t.Fatalf("u should open the URL dialog, input = %+v", m.input())
	}
	m = submitInput(t, m, "http://x.example/b")
	if m.scratch == nil || m.scratch.URL != "http://x.example/b" {
		t.Fatalf("scratch = %+v", m.scratch)
	}
}

func TestScratchPersistsAcrossRestart(t *testing.T) {
	ws := fixtureWS(t)
	d := testDeps(t, ws)
	m := newScratch(t, resize(newModel(d), 120, 40), "http://x.example/persist")
	_ = press(m, "m", "j", "enter")

	sess, err := d.StateStore.Load(ws.Root)
	if err != nil {
		t.Fatal(err)
	}
	if s := sess.Scratch; s == nil || s.URL != "http://x.example/persist" || s.Method != "POST" {
		t.Fatalf("session scratch = %+v", s)
	}
	d.Session = sess
	m = press(resize(newModel(d), 120, 40), "N")
	if m.input() != nil {
		t.Fatal("a restored unsaved request must not reopen the URL dialog")
	}
	if s := m.scratch; s.URL != "http://x.example/persist" || s.Method != "POST" || m.requestTitle() != unsavedTitle {
		t.Fatalf("restored scratch = %+v, title %q", s, m.requestTitle())
	}
}

func TestScratchImportCurlInRequestPanel(t *testing.T) {
	ws := fixtureWS(t)
	before := requestFiles(t, ws)
	m := newTestModel(t, ws)
	m = press(typeText(press(m, "4", "i"), secretCurl), "enter")
	if m.input() != nil {
		t.Fatalf("import into [4] must not ask for a name, input = %+v", m.input())
	}
	s := m.scratch
	if s == nil || s.URL != "https://a.io/x" || m.requestTitle() != unsavedTitle || m.Focus() != PanelRequest {
		t.Fatalf("scratch = %+v, title %q", s, m.requestTitle())
	}
	held := fmt.Sprintf("%+v", *s)
	for _, sec := range importedSecrets {
		if strings.Contains(held, sec) {
			t.Fatalf("scratch holds secret %q: %s", sec, held)
		}
	}
	if !slices.Contains(s.Headers, workspace.KV{Key: "Authorization", Value: "Bearer {{get_x_authorization}}"}) {
		t.Fatalf("headers = %+v", s.Headers)
	}
	if got := envByName(t, ws, "staging").Secrets["get_x_authorization"]; got != "tok-abc123" {
		t.Fatalf("staging secret = %q", got)
	}
	if m.Status() != "imported into unsaved request (3 secrets → staging)" {
		t.Fatalf("status = %q", m.Status())
	}
	if after := requestFiles(t, ws); !slices.Equal(before, after) {
		t.Fatalf("import into [4] created files: %v", after)
	}
}

func TestScratchImportCurlWithoutEnv(t *testing.T) {
	ws := fixtureWS(t)
	m := newTestModel(t, ws)
	m.envName = ""
	m = press(typeText(press(m, "4", "i"), secretCurl), "enter")
	if s := m.scratch; s == nil || s.Auth.Password != "pw-xyz789" {
		t.Fatalf("without an env the import should stay unsaved as is: %+v", s)
	}
	if m.Status() != "imported into unsaved request: no environment for secrets" {
		t.Fatalf("status = %q", m.Status())
	}
	if _, ok := envByName(t, ws, "staging").Secrets["get_x_authorization"]; ok {
		t.Fatal("secrets moved without an active env")
	}
}

func TestScratchImportCurlWithoutCollections(t *testing.T) {
	ws := emptyWS(t)
	m := newTestModel(t, ws)
	m = press(typeText(press(m, "i"), `curl https://a.io/z -H 'Accept: text/plain'`), "enter")
	if m.input() != nil {
		t.Fatalf("no name dialog expected, input = %+v", m.input())
	}
	if s := m.scratch; s == nil || s.URL != "https://a.io/z" || m.Focus() != PanelRequest {
		t.Fatalf("scratch = %+v, focus %v", s, m.Focus())
	}
	if m.Status() != "imported into unsaved request" {
		t.Fatalf("status = %q", m.Status())
	}
	if files := requestFiles(t, ws); len(files) != 0 {
		t.Fatalf("files created: %v", files)
	}
}

func TestScratchSelectionSwitching(t *testing.T) {
	m, _ := sendModel(t, fixtureWS(t), "http://staging.example")
	m = newScratch(t, m, "http://scratch.example/x")
	m = press(m, "m", "j", "enter")

	if m = press(m, "2"); m.requestTitle() != unsavedTitle {
		t.Fatal("focusing Collections alone should keep the unsaved request")
	}
	m = press(m, "k")
	if tmpl := m.selectedRequest(); tmpl.Name != "create user" || m.requestTitle() != reqTitle {
		t.Fatalf("after k: %q, title %q", tmpl.Name, m.requestTitle())
	}
	if out := m.plainRender(); strings.Contains(out, "scratch.example") {
		t.Fatalf("[4] still shows the unsaved request:\n%s", out)
	}

	m = press(m, "N")
	if tmpl := m.selectedRequest(); m.input() != nil || m.Focus() != PanelRequest || tmpl.URL != "http://scratch.example/x" || tmpl.Method != "POST" {
		t.Fatalf("N back: %+v, input %v", tmpl, m.input())
	}

	m = press(m, "2", "enter")
	if tmpl := m.selectedRequest(); tmpl.Name != "create user" || m.Focus() != PanelRequest {
		t.Fatalf("enter: %q", tmpl.Name)
	}

	entries := []history.Entry{
		{ID: "h1", Root: m.ws().Root, Time: time.Now(), Code: 200, Template: workspace.Request{Name: "a", Method: "GET", URL: "http://h.test/a"}},
		{ID: "h2", Root: m.ws().Root, Time: time.Now(), Code: 200, Template: workspace.Request{Name: "b", Method: "GET", URL: "http://h.test/b"}},
	}
	next, _ := m.Update(historyLoadedMsg{entries: entries})
	m = press(next.(Model), "N", "3", "j")
	if m.requestTitle() == unsavedTitle {
		t.Fatal("moving the History cursor should switch away")
	}
	if m.scratch == nil || m.scratch.URL != "http://scratch.example/x" {
		t.Fatal("switching away must keep the unsaved request")
	}
}

func TestScratchEditorRoundTrip(t *testing.T) {
	m := newScratch(t, newTestModel(t, emptyWS(t)), "http://x.example/a")
	path, err := m.requestTempFile()
	if err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(path); err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("temp file %s: %v %v", path, fi, err)
	}
	if !strings.Contains(readFile(t, path), "url: http://x.example/a") {
		t.Fatalf("temp YAML:\n%s", readFile(t, path))
	}
	if err := os.WriteFile(path, []byte("method: PUT\nurl: http://x.example/b\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	m = drain(m, func() tea.Msg { return editorDoneMsg{kind: editRequest, path: path} })
	if s := m.scratch; s.Method != "PUT" || s.URL != "http://x.example/b" || s.Name != scratchName {
		t.Fatalf("scratch = %+v", s)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("temp file should be removed after reading")
	}
}

func TestScratchCopyAsCurl(t *testing.T) {
	m := newScratch(t, newTestModel(t, fixtureWS(t)), "{{base_url}}/c")
	m = press(m, "c")
	if got := clipText(m); !strings.Contains(got, "http://staging.example/c") {
		t.Fatalf("clipboard = %q", got)
	}
}

func TestScratchKeyListed(t *testing.T) {
	m := newTestModel(t, fixtureWS(t))
	for _, p := range []Panel{PanelEnvs, PanelCollections, PanelHistory, PanelRequest} {
		if !slices.ContainsFunc(Model{focus: p}.focusedHints(), func(b binding) bool { return b.keys[0] == &km.NewUnsaved }) {
			t.Errorf("panel %v hints lack %s", p, km.NewUnsaved[0])
		}
	}
	if !slices.ContainsFunc(Model{focus: PanelRequest}.focusedHints(), func(b binding) bool { return b.keys[0] == &km.ImportCurl }) {
		t.Error("[4] hints lack i")
	}
	if out := press(m, "?").plainRender(); !strings.Contains(out, "new unsaved request") {
		t.Fatalf("help lacks N:\n%s", out)
	}
}
