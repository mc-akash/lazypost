package ui

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mc-akash/lazypost/internal/history"
	"github.com/mc-akash/lazypost/internal/workspace"
)

func histEntry(id, root, name string, age time.Duration) history.Entry {
	return history.Entry{
		ID: id, Root: root, Time: time.Now().Add(-age), Code: 200,
		Template: workspace.Request{Name: name, Method: "GET"},
	}
}

func TestNewDoesNotReadHistoryUntilInit(t *testing.T) {
	ws := fixtureWS(t)
	d := testDeps(t, ws)
	for _, e := range []history.Entry{
		histEntry("old", ws.Root, "stored request", 5*time.Minute),
		histEntry("other", "/some/other/ws", "other workspace", 4*time.Minute),
		histEntry("legacy", "", "no root", 3*time.Minute),
	} {
		if err := d.History.Append(e); err != nil {
			t.Fatal(err)
		}
	}
	m := resize(newModel(d), 120, 40)
	if len(m.history.entries) != 0 {
		t.Fatalf("New read history: %+v", m.history.entries)
	}
	initCmd := m.Init()
	if initCmd == nil {
		t.Fatal("Init returned no history load command")
	}

	sent := histEntry("new", ws.Root, "sent before load", 0)
	m.history = m.history.add(sent, true, m.historyMax())
	if err := d.History.Append(sent); err != nil {
		t.Fatal(err)
	}

	m = drain(m, initCmd)
	var names []string
	for _, e := range m.history.entries {
		names = append(names, e.Template.Name)
	}
	if got := strings.Join(names, ","); got != "sent before load,stored request" {
		t.Fatalf("history rows = %q, want the in-memory entry then this workspace's stored one", got)
	}
}

func TestHistoryLoadErrorInStatus(t *testing.T) {
	d := testDeps(t, fixtureWS(t))
	d.History = history.Open(t.TempDir())
	m := newModel(d)
	m = drain(m, m.Init())
	if !strings.Contains(m.Status(), "history: ") {
		t.Fatalf("status = %q", m.Status())
	}
}

func TestSendRecordsRoot(t *testing.T) {
	srv := serve(t, &recorder{body: []byte("ok")})
	ws := fixtureWS(t)
	m, histPath := sendModel(t, ws, srv.URL)
	pressRun(m, "s")
	entries, err := history.Open(histPath).Load()
	if err != nil || len(entries) != 1 || entries[0].Root != ws.Root || entries[0].ID == "" {
		t.Fatalf("entries = %+v, err = %v", entries, err)
	}
}

func TestSendErrorRedactsSecrets(t *testing.T) {
	closed := httptest.NewServer(http.NotFoundHandler())
	closed.Close()
	ws := fixtureWS(t)
	envByName(t, ws, "staging").Secrets["api_key"] = "SUPERSECRET"
	folderByName(t, ws, "users").Auth = workspace.Auth{Type: "apikey", Key: "api_key", Value: "{{api_key}}", In: "query"}
	m, histPath := sendModel(t, ws, closed.URL)
	m = pressRun(m, "s")

	raw, err := os.ReadFile(histPath)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte("SUPERSECRET")) {
		t.Fatalf("history leaks the secret:\n%s", raw)
	}
	if !bytes.Contains(raw, []byte(`"err"`)) {
		t.Fatalf("history entry has no error:\n%s", raw)
	}
	for _, view := range []Model{m, press(m, "3")} {
		if out := view.Render(); strings.Contains(out, "SUPERSECRET") || !strings.Contains(out, "error:") {
			t.Fatalf("response view leaks the secret or shows no error:\n%s", view.plainRender())
		}
	}
}

func TestDraftPersistedWithoutQuit(t *testing.T) {
	ws := fixtureWS(t)
	d := testDeps(t, ws)
	press(resize(newModel(d), 120, 40), "j", "j", "j", "j", "4", "m", "j", "enter")
	sess, err := d.StateStore.Load(ws.Root)
	if err != nil {
		t.Fatal(err)
	}
	key := filepath.Join("collections", "users", "get-user.yaml")
	if dr, ok := sess.Drafts[key]; !ok || dr.Request.Method != "POST" {
		t.Fatalf("drafts = %+v, want %s with POST", sess.Drafts, key)
	}
}

func TestSaveSessionExported(t *testing.T) {
	ws := fixtureWS(t)
	d := testDeps(t, ws)
	m := newModel(d)
	if sess, _ := d.StateStore.Load(ws.Root); sess.LastEnv != "" {
		t.Fatalf("state written before SaveSession: %+v", sess)
	}
	if err := m.SaveSession(); err != nil {
		t.Fatal(err)
	}
	sess, err := d.StateStore.Load(ws.Root)
	if err != nil || sess.LastEnv != "staging" {
		t.Fatalf("LastEnv = %q (err %v), want staging", sess.LastEnv, err)
	}
}

func TestStartupStatusShown(t *testing.T) {
	d := testDeps(t, fixtureWS(t))
	d.StartupStatus = "session state unreadable: boom (starting fresh)"
	if m := newModel(d); !strings.Contains(m.Status(), d.StartupStatus) {
		t.Fatalf("status = %q", m.Status())
	}
}

func TestHistoryPanelKeepsConfiguredMax(t *testing.T) {
	m := newTestModel(t, fixtureWS(t))
	m.deps.History = history.Open(filepath.Join(t.TempDir(), "h.jsonl")).WithMax(2)
	for _, id := range []string{"a", "b", "c"} {
		m.history = m.history.add(history.Entry{ID: id}, true, m.historyMax())
	}
	if n := len(m.history.entries); n != 2 || m.history.entries[0].ID != "c" {
		t.Fatalf("entries = %d, newest %q", n, m.history.entries[0].ID)
	}
	loaded := []history.Entry{{ID: "x"}, {ID: "y"}}
	if n := len(historyPanel{}.merge(loaded, 1).entries); n != 1 {
		t.Fatalf("merge kept %d, want 1", n)
	}
}
