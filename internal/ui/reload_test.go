package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mc-akash/lazypost/internal/workspace"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestReloadPicksUpDiskChangesAndKeepsState(t *testing.T) {
	srv := serve(t, &recorder{contentType: "text/plain", body: []byte("hello")})
	ws := fixtureWS(t)
	m, _ := sendModel(t, ws, srv.URL)
	m = pressRun(m, "s")
	m = m.editRequest(func(r *workspace.Request) { r.URL = "http://draft.example/" })

	m = press(m, "2", "g", "g", "enter", "G")
	if m.selectedRowLabel() != "get user" {
		t.Fatalf("setup: selected %q", m.selectedRowLabel())
	}

	writeFile(t, filepath.Join(ws.Root, "collections", "users", "new-one.yaml"),
		"name: new one\nmethod: GET\nurl: http://x/\n")
	writeFile(t, envFile(ws, "staging"), "base_url: http://reloaded.example\n")

	m = press(m, "R")
	if m.Status() != "reloaded" {
		t.Fatalf("status = %q", m.Status())
	}
	if m.ws() == ws {
		t.Fatal("workspace not reloaded")
	}
	if m.selectedRowLabel() != "get user" {
		t.Fatalf("selection = %q, want get user", m.selectedRowLabel())
	}
	if m.activeEnv() == nil || m.activeEnv().Name != "staging" {
		t.Fatalf("active env = %v", m.activeEnv())
	}
	if _, ok := m.drafts[getUserKey]; !ok || currentTmpl(m).URL != "http://draft.example/" {
		t.Fatal("draft lost")
	}
	if !m.collections.collapsed[filepath.Join(ws.Root, "collections", "orders")] {
		t.Fatal("collapsed folder expanded by reload")
	}
	out := m.plainRender()
	if !strings.Contains(out, "new one") || strings.Contains(out, "list orders") {
		t.Fatalf("tree not reloaded:\n%s", out)
	}
	if !strings.Contains(out, "hello") {
		t.Fatalf("last response lost:\n%s", out)
	}
	if out := press(m, "V").plainRender(); !strings.Contains(out, "http://reloaded.example") {
		t.Fatalf("vars table not reloaded:\n%s", out)
	}
}

func TestReloadDropsOrphanDrafts(t *testing.T) {
	ws := fixtureWS(t)
	m, _ := sendModel(t, ws, "http://example.invalid")
	m = press(m, "2", "g", "g", "j")
	m = m.editRequest(func(r *workspace.Request) { r.URL = "http://draft/" })
	if err := os.Remove(filepath.Join(ws.Root, listOrdersKey)); err != nil {
		t.Fatal(err)
	}
	m = press(m, "R")
	if m.Status() != "reloaded; dropped 1 orphan draft(s)" {
		t.Fatalf("status = %q", m.Status())
	}
	if len(m.drafts) != 0 {
		t.Fatalf("drafts = %v", m.drafts)
	}
	if m.selectedRowLabel() != "orders" {
		t.Fatalf("selection = %q, want orders", m.selectedRowLabel())
	}
	if len(m.deps.Session.Drafts) != 0 {
		t.Fatal("session not persisted")
	}
}

func TestReloadReportsLoadErrorsAndEnvFallback(t *testing.T) {
	ws := fixtureWS(t)
	m := newTestModel(t, ws)
	writeFile(t, filepath.Join(ws.Root, "collections", "users", "bad.yaml"), "method: [unclosed\n")
	if err := os.Remove(envFile(ws, "staging")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(secretFile(ws, "staging")); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	m = press(m, "R")
	want := "reloaded — 1 file(s) failed to load: collections/users/bad.yaml: yaml: "
	if !strings.HasPrefix(m.Status(), want) {
		t.Fatalf("status = %q, want prefix %q", m.Status(), want)
	}
	if m.activeEnv() == nil || m.activeEnv().Name != "dev" {
		t.Fatalf("active env = %v, want dev", m.activeEnv())
	}
}

func TestReloadIgnoredWhileInputOpen(t *testing.T) {
	ws := fixtureWS(t)
	m := press(newTestModel(t, ws), "2", "n", "R")
	if m.ws() != ws || m.input() == nil || m.input().ti.Value() != "R" {
		t.Fatal("R should type into the open input")
	}
}

func TestReloadKeepsDraftOfFileThatFailedToLoad(t *testing.T) {
	ws := fixtureWS(t)
	m, _ := sendModel(t, ws, "http://example.invalid")
	m = press(m, "2", "g", "g", "j")
	m = m.editRequest(func(r *workspace.Request) { r.URL = "http://draft/" })
	writeFile(t, filepath.Join(ws.Root, listOrdersKey), "method: [unclosed\n")
	m = press(m, "R")
	if strings.Contains(m.Status(), "orphan") {
		t.Fatalf("status = %q", m.Status())
	}
	if d, ok := m.drafts[listOrdersKey]; !ok || d.Request.URL != "http://draft/" {
		t.Fatalf("drafts = %v", m.drafts)
	}
	if _, ok := m.deps.Session.Drafts[listOrdersKey]; !ok {
		t.Fatal("draft dropped from session")
	}
}
