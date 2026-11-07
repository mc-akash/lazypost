package ui

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/mc-akash/lazypost/internal/history"
	"github.com/mc-akash/lazypost/internal/workspace"
)

type recorder struct {
	mu          sync.Mutex
	auth        []string
	hits        int
	contentType string
	body        []byte
}

func (rec *recorder) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rec.mu.Lock()
	rec.hits++
	rec.auth = append(rec.auth, r.Header.Get("Authorization"))
	ct, body := rec.contentType, rec.body
	rec.mu.Unlock()
	if ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	_, _ = w.Write(body)
}

func (rec *recorder) lastAuth() string {
	rec.mu.Lock()
	defer rec.mu.Unlock()
	if len(rec.auth) == 0 {
		return ""
	}
	return rec.auth[len(rec.auth)-1]
}

func (rec *recorder) hitCount() int {
	rec.mu.Lock()
	defer rec.mu.Unlock()
	return rec.hits
}

func serve(t *testing.T, rec http.Handler) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(rec)
	t.Cleanup(srv.Close)
	return srv
}

func sendModel(t *testing.T, ws *workspace.Workspace, baseURL string) (Model, string) {
	t.Helper()
	envByName(t, ws, "staging").Vars["base_url"] = baseURL
	d := testDeps(t, ws)
	histPath := filepath.Join(t.TempDir(), "history.jsonl")
	d.History = history.Open(histPath)
	m := resize(newModel(d), 120, 40)

	return press(m, "j", "j", "j", "j"), histPath
}

func envByName(t *testing.T, ws *workspace.Workspace, name string) *workspace.Env {
	t.Helper()
	for _, e := range ws.Envs {
		if e.Name == name {
			return e
		}
	}
	t.Fatalf("no env %q", name)
	return nil
}

func folderByName(t *testing.T, ws *workspace.Workspace, name string) *workspace.Folder {
	t.Helper()
	for _, f := range ws.Collections.Folders {
		if f.Name == name {
			return f
		}
	}
	t.Fatalf("no folder %q", name)
	return nil
}

func requestByName(t *testing.T, f *workspace.Folder, name string) *workspace.Request {
	t.Helper()
	for _, r := range f.Requests {
		if r.Name == name {
			return r
		}
	}
	t.Fatalf("no request %q", name)
	return nil
}

func bearerFolder(t *testing.T, ws *workspace.Workspace) {
	t.Helper()
	folderByName(t, ws, "users").Auth = workspace.Auth{Type: "bearer", Token: "{{token}}"}
}

func sendMsg(cmd tea.Cmd) tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if _, ok := msg.(sendResultMsg); ok {
		return msg
	}
	cmds, _ := cmdList(msg)
	for _, c := range cmds {
		if m := sendMsg(c); m != nil {
			return m
		}
	}
	return nil
}

func pressRun(m Model, key string) Model {
	next, cmd := m.Update(keyMsg(key))
	return drain(next.(Model), cmd)
}

func TestSendShowsStatusInResponseTitle(t *testing.T) {
	srv := serve(t, &recorder{contentType: "application/json", body: []byte(`{"id":1}`)})
	m, _ := sendModel(t, fixtureWS(t), srv.URL)
	m = pressRun(m, "s")
	out := m.plainRender()
	if !regexp.MustCompile(`200 OK · \d+ms · 8B`).MatchString(out) {
		t.Fatalf("response title missing:\n%s", out)
	}
	if !strings.Contains(out, `"id": 1`) {
		t.Fatalf("pretty JSON body missing:\n%s", out)
	}
}

func TestSendResolvesActiveEnv(t *testing.T) {
	rec := &recorder{}
	srv := serve(t, rec)
	ws := fixtureWS(t)
	bearerFolder(t, ws)
	m, _ := sendModel(t, ws, srv.URL)
	pressRun(m, "s")
	if got := rec.lastAuth(); got != "Bearer staging-secret-token" {
		t.Fatalf("Authorization = %q", got)
	}
}

func TestSendAppendsRedactedHistory(t *testing.T) {
	srv := serve(t, &recorder{body: []byte("ok")})
	ws := fixtureWS(t)
	bearerFolder(t, ws)
	m, histPath := sendModel(t, ws, srv.URL)
	m = pressRun(m, "s")
	raw, err := os.ReadFile(histPath)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte("staging-secret-token")) {
		t.Fatalf("history leaks the token:\n%s", raw)
	}
	if !bytes.Contains(raw, []byte("••••")) {
		t.Fatalf("history has no redaction mask:\n%s", raw)
	}
	entries, err := history.Open(histPath).Load()
	if err != nil || len(entries) != 1 {
		t.Fatalf("entries = %d, err = %v", len(entries), err)
	}
	e := entries[0]
	if e.Code != 200 || e.Env != "staging" || e.Template.URL != "{{base_url}}/users/1" || e.Sent.URL != srv.URL+"/users/1" {
		t.Fatalf("entry = %+v", e)
	}
	if !strings.Contains(m.plainRender(), "200 GET  get user") {
		t.Fatal("history panel should list the new entry")
	}
}

func TestSendWithMissingVarWarns(t *testing.T) {
	rec := &recorder{}
	srv := serve(t, rec)
	ws := fixtureWS(t)
	req := requestByName(t, folderByName(t, ws, "users"), "get user")
	req.Headers = append(req.Headers, workspace.KV{Key: "X-Trace", Value: "{{trace_id}}"})
	m, _ := sendModel(t, ws, srv.URL)
	m = pressRun(m, "s")
	if !strings.Contains(m.Status(), "missing: trace_id") {
		t.Fatalf("status = %q", m.Status())
	}
	if rec.hitCount() != 1 {
		t.Fatalf("server hits = %d, want 1", rec.hitCount())
	}
}

func TestEscCancelsInFlight(t *testing.T) {
	arrived := make(chan struct{}, 1)
	srv := serve(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		arrived <- struct{}{}
		<-r.Context().Done()
	}))
	m, histPath := sendModel(t, fixtureWS(t), srv.URL)
	next, cmd := m.Update(keyMsg("s"))
	m = next.(Model)
	if !strings.Contains(m.plainRender(), "⠋ sending… (esc to cancel)") {
		t.Fatalf("no sending title with spinner:\n%s", m.plainRender())
	}
	if !strings.Contains(lastLine(m.plainRender()), "cancel: esc") {
		t.Fatalf("hint line should offer esc while sending: %q", lastLine(m.plainRender()))
	}
	done := make(chan tea.Msg, 1)
	go func() { done <- sendMsg(cmd) }()
	select {
	case <-arrived:
	case <-time.After(5 * time.Second):
		t.Fatal("request never reached the server")
	}
	m = press(m, "esc")
	if !strings.Contains(m.plainRender(), "cancelled") {
		t.Fatalf("no cancelled title:\n%s", m.plainRender())
	}
	select {
	case msg := <-done:
		next, _ = m.Update(msg)
		m = next.(Model)
	case <-time.After(5 * time.Second):
		t.Fatal("cancel did not stop the send")
	}
	if !strings.Contains(m.plainRender(), "cancelled") {
		t.Fatal("the cancelled send's result replaced the cancelled title")
	}
	if entries, _ := history.Open(histPath).Load(); len(entries) != 0 {
		t.Fatalf("cancelled send was recorded: %+v", entries)
	}
}

func TestSpinnerAdvancesWhileSending(t *testing.T) {
	srv := serve(t, &recorder{body: []byte("ok")})
	m, _ := sendModel(t, fixtureWS(t), srv.URL)
	next, cmd := m.Update(keyMsg("s"))
	m = next.(Model)
	tick := m.spin.Tick()
	next, tickCmd := m.Update(tick)
	m = next.(Model)
	if !strings.Contains(m.plainRender(), "⠙ sending…") {
		t.Fatalf("spinner did not advance:\n%s", m.plainRender())
	}
	if tickCmd == nil {
		t.Fatal("spinner should schedule its next tick while sending")
	}
	m = drain(m, cmd)
	if _, c := m.Update(m.spin.Tick()); c != nil {
		t.Fatal("spinner should stop ticking once the send is done")
	}
}

func TestHistoryAppendFailureShownInStatus(t *testing.T) {
	srv := serve(t, &recorder{body: []byte("ok")})
	m, _ := sendModel(t, fixtureWS(t), srv.URL)
	m.deps.History = history.Open(t.TempDir())
	m = pressRun(m, "s")
	if !strings.HasPrefix(m.Status(), "history: ") {
		t.Fatalf("status = %q", m.Status())
	}
	if !strings.Contains(m.plainRender(), "200 GET  get user") {
		t.Fatal("the in-memory history list should still get the entry")
	}
}

func TestSecondSendCancelsFirst(t *testing.T) {
	srv := serve(t, &recorder{body: []byte("second")})
	m, histPath := sendModel(t, fixtureWS(t), srv.URL)
	next, first := m.Update(keyMsg("s"))
	next, second := next.(Model).Update(keyMsg("s"))
	m = next.(Model)
	stale := sendMsg(first)
	if _, ok := stale.(sendResultMsg); !ok {
		t.Fatalf("first command yielded %T, want sendResultMsg", stale)
	}
	next, _ = m.Update(stale)
	if m = next.(Model); !strings.Contains(m.plainRender(), "sending… (esc to cancel)") {
		t.Fatalf("stale result was applied:\n%s", m.plainRender())
	}
	m = drain(m, second)
	if !strings.Contains(m.plainRender(), "200 OK") {
		t.Fatalf("second send not shown:\n%s", m.plainRender())
	}
	if entries, _ := history.Open(histPath).Load(); len(entries) != 1 {
		t.Fatalf("history entries = %d, want 1", len(entries))
	}
}

func TestNetworkErrorShown(t *testing.T) {
	closed := httptest.NewServer(http.NotFoundHandler())
	closed.Close()
	m, histPath := sendModel(t, fixtureWS(t), closed.URL)
	m = pressRun(m, "s")
	out := m.plainRender()
	if !strings.Contains(out, "error:") || !strings.Contains(out, "[5] Response ── ERR") {
		t.Fatalf("error not shown:\n%s", out)
	}
	entries, err := history.Open(histPath).Load()
	if err != nil || len(entries) != 1 || entries[0].Err == "" {
		t.Fatalf("entries = %+v, err = %v", entries, err)
	}
	if !strings.Contains(out, "ERR GET  get user") {
		t.Fatalf("history row should show ERR:\n%s", out)
	}
}

func TestBinaryResponseShowsPlaceholder(t *testing.T) {
	png := append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 5<<20)...)
	srv := serve(t, &recorder{contentType: "image/png", body: png})
	m, _ := sendModel(t, fixtureWS(t), srv.URL)
	m = pressRun(m, "s")
	if out := m.plainRender(); !strings.Contains(out, "binary body") || !strings.Contains(out, "5.0MB") {
		t.Fatalf("binary placeholder missing:\n%s", out)
	}
}

func TestInvalidJSONShownRaw(t *testing.T) {
	srv := serve(t, &recorder{contentType: "application/json", body: []byte("{oops")})
	m, _ := sendModel(t, fixtureWS(t), srv.URL)
	if out := pressRun(m, "s").plainRender(); !strings.Contains(out, "{oops") {
		t.Fatalf("raw body missing:\n%s", out)
	}
}

func TestHistoryResendUsesCurrentToken(t *testing.T) {
	rec := &recorder{}
	srv := serve(t, rec)
	ws := fixtureWS(t)
	bearerFolder(t, ws)
	m, histPath := sendModel(t, ws, srv.URL)
	m = pressRun(m, "s")
	envByName(t, ws, "staging").Secrets["token"] = "fresh-token"
	m = pressRun(press(m, "3"), "s")
	if got := rec.lastAuth(); got != "Bearer fresh-token" {
		t.Fatalf("resend Authorization = %q", got)
	}
	if entries, _ := history.Open(histPath).Load(); len(entries) != 2 {
		t.Fatalf("history entries = %d, want 2", len(entries))
	}
	if len(m.history.entries) != 2 || m.Focus() != PanelHistory {
		t.Fatalf("history panel entries = %d focus = %v", len(m.history.entries), m.Focus())
	}
}

func TestHistoryEntryShowsSentAndResponse(t *testing.T) {
	srv := serve(t, &recorder{contentType: "text/plain", body: []byte("stored body")})
	m, _ := sendModel(t, fixtureWS(t), srv.URL)
	m = press(pressRun(m, "s"), "3")
	out := m.plainRender()
	for _, want := range []string{srv.URL + "/users/1", "stored body", "200 OK · "} {
		if !strings.Contains(out, want) {
			t.Errorf("history view missing %q:\n%s", want, out)
		}
	}
}

func TestCopyBody(t *testing.T) {
	srv := serve(t, &recorder{contentType: "text/plain", body: []byte("hello body")})
	m, _ := sendModel(t, fixtureWS(t), srv.URL)
	m = press(pressRun(m, "s"), "5")
	next, cmd := m.Update(keyMsg("y"))
	m = next.(Model)
	if got := m.deps.Clip.(*fakeClip).text; got != "hello body" {
		t.Fatalf("clipboard = %q", got)
	}
	if cmd == nil {
		t.Fatal("y should also return an OSC 52 clipboard command")
	}
	if m.Status() != "copied" {
		t.Fatalf("status = %q", m.Status())
	}
}

func TestSendRedactsResponseInHistory(t *testing.T) {
	srv := serve(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Echo", r.Header.Get("Authorization"))
		_, _ = w.Write([]byte("echo " + r.Header.Get("Authorization")))
	}))
	ws := fixtureWS(t)
	bearerFolder(t, ws)
	m, histPath := sendModel(t, ws, srv.URL)
	m = pressRun(m, "s")
	if !strings.Contains(m.plainRender(), "echo Bearer staging-secret-token") {
		t.Fatalf("live response should stay raw:\n%s", m.plainRender())
	}
	entries, err := history.Open(histPath).Load()
	if err != nil || len(entries) != 1 {
		t.Fatalf("entries = %d, err = %v", len(entries), err)
	}
	e := entries[0]
	if string(e.Body) != "echo Bearer ••••" || e.Header.Get("X-Echo") != "Bearer ••••" {
		t.Fatalf("body = %q, X-Echo = %q", e.Body, e.Header.Get("X-Echo"))
	}
}
