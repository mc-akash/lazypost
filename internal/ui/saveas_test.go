package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mc-akash/lazypost/internal/workspace"
)

func submitInput(t *testing.T, m Model, v string) Model {
	t.Helper()
	if m.input() == nil {
		t.Fatal("no input open")
	}
	in := *m.input()
	in.ti.SetValue(v)
	m.overlay = &in
	return press(m, "enter")
}

func selectedReq(t *testing.T, m Model) *workspace.Request {
	t.Helper()
	row, ok := m.collections.selected(m.ws().Collections)
	if !ok || row.req == nil {
		t.Fatal("no request selected")
	}
	return row.req
}

func TestSaveAsCopiesDraftIntoNewRequest(t *testing.T) {
	ws, orig := acceptHeaderWS(t)
	before := readFile(t, orig.Path)
	m, _ := sendModel(t, ws, "http://example.invalid")
	m = press(m, "4", "space")
	m = m.editRequest(func(r *workspace.Request) { r.URL = "http://draft.example/x" })

	m = press(m, "W")
	if m.input() == nil || m.input().ti.Value() != "users/get user" {
		t.Fatalf("prefill = %v", m.input())
	}
	m = submitInput(t, m, "orders/copy of a")

	if got := m.Status(); got != "saved as orders/copy of a" {
		t.Fatalf("status = %q", got)
	}
	newPath := filepath.Join(ws.Root, "collections", "orders", "copy-of-a.yaml")
	saved := readFile(t, newPath)
	for _, want := range []string{"name: copy of a", "http://draft.example/x", "disabled: true"} {
		if !strings.Contains(saved, want) {
			t.Fatalf("new file lacks %q:\n%s", want, saved)
		}
	}
	if readFile(t, orig.Path) != before {
		t.Fatal("original file changed")
	}
	if _, ok := m.drafts[getUserKey]; ok || len(m.drafts) != 0 {
		t.Fatalf("drafts = %v, want none", m.drafts)
	}
	if sel := selectedReq(t, m); sel.Path != newPath || sel.Name != "copy of a" {
		t.Fatalf("selected %q (%s)", sel.Name, sel.Path)
	}
	if m.deps.Session.LastRequest != "collections/orders/copy-of-a.yaml" {
		t.Fatalf("session last request = %q", m.deps.Session.LastRequest)
	}
}

func TestSaveAsCreatesMissingFolders(t *testing.T) {
	ws := fixtureWS(t)
	m, _ := sendModel(t, ws, "http://example.invalid")
	m = submitInput(t, press(m, "W"), "Billing API/v2/charge it")
	want := filepath.Join(ws.Root, "collections", "billing-api", "v2", "charge-it.yaml")
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("new request file: %v", err)
	}
	if sel := selectedReq(t, m); sel.Path != want {
		t.Fatalf("selected %s", sel.Path)
	}
	if got := m.Status(); got != "saved as billing-api/v2/charge it" {
		t.Fatalf("status = %q", got)
	}
}

func TestSaveAsReusesFolderAndSuffixesCollision(t *testing.T) {
	ws := fixtureWS(t)
	m, _ := sendModel(t, ws, "http://example.invalid")
	m = press(m, "W", "enter")
	want := filepath.Join(ws.Root, "collections", "users", "get-user-2.yaml")
	if sel := selectedReq(t, m); sel.Path != want || sel.Name != "get user" {
		t.Fatalf("selected %q (%s)", sel.Name, sel.Path)
	}
	if n := len(folderByName(t, m.ws(), "users").Requests); n != 3 {
		t.Fatalf("users has %d requests, want 3", n)
	}
	if len(m.ws().Collections.Folders) != 2 {
		t.Fatal("no folder should be created")
	}
}

func TestSaveAsFromCollectionsPanel(t *testing.T) {
	m, _ := sendModel(t, fixtureWS(t), "http://example.invalid")
	m = submitInput(t, press(m, "2", "W"), "copy")
	if sel := selectedReq(t, m); sel.Name != "copy" || sel.Parent != m.ws().Collections {
		t.Fatalf("selected %q in %v", sel.Name, sel.Parent)
	}
}

func TestSaveAsEmptyCancels(t *testing.T) {
	for _, v := range []string{"", "  ", "orders/", "orders/  "} {
		m, _ := sendModel(t, fixtureWS(t), "http://example.invalid")
		m = submitInput(t, press(m, "W"), v)
		if m.Status() != "save as cancelled" {
			t.Fatalf("%q: status = %q", v, m.Status())
		}
		if selectedReq(t, m).Name != "get user" {
			t.Fatalf("%q: selection moved", v)
		}
	}
}
