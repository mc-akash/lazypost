package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

const multiLineCurl = "curl 'https://a.io/x' \\\n  -H 'A: b' \\\r\n  --data-raw '{\"a\":1}'"

func wantImportedName(t *testing.T, m Model) {
	t.Helper()
	m = press(m, "enter")
	if m.input() == nil || m.input().title != dialogRequestName || m.input().ti.Value() != "POST /x" {
		t.Fatalf("status %q, input %+v: want the Request name dialog", m.Status(), m.input())
	}
}

func TestImportMultiLineCurlFromClipboard(t *testing.T) {
	m := newTestModel(t, fixtureWS(t))
	m.deps.Clip.(*fakeClip).text = multiLineCurl + "\n"
	m = press(m, "i")
	if v := m.input().ti.Value(); strings.Contains(v, `\`) {
		t.Fatalf("prefill kept line continuations: %q", v)
	}
	wantImportedName(t, m)
}

func TestImportMultiLineCurlFromPaste(t *testing.T) {
	m := newTestModel(t, fixtureWS(t))
	m = press(m, "i")
	next, _ := m.Update(tea.PasteMsg{Content: multiLineCurl})
	wantImportedName(t, next.(Model))
}

func importIntoScratchKeys(m Model, url string) Model {
	return press(typeText(press(m, "4", "i"), "curl "+url), "enter")
}

func TestImportIntoExistingScratchAsks(t *testing.T) {
	m := newScratch(t, newTestModel(t, fixtureWS(t)), "https://old.example/")

	m = importIntoScratchKeys(m, "https://new.example/a")
	if p := m.prompt(); p == nil || p.title != dialogReplaceScratch || p.message != msgImportScratch {
		t.Fatalf("want the replace confirm, got %+v", m.prompt())
	}
	m = press(m, "n")
	if m.scratch.URL != "https://old.example/" || m.Status() != statusScratchKept {
		t.Fatalf("no must keep the scratch: %+v, status %q", m.scratch, m.Status())
	}

	m = press(importIntoScratchKeys(m, "https://new.example/a"), "y")
	if m.scratch.URL != "https://new.example/a" {
		t.Fatalf("yes must replace the scratch: %+v", m.scratch)
	}
}

func TestImportIntoNoScratchDoesNotAsk(t *testing.T) {
	m := importIntoScratchKeys(newTestModel(t, fixtureWS(t)), "https://new.example/a")
	if m.prompt() != nil || m.scratch == nil || m.scratch.URL != "https://new.example/a" {
		t.Fatalf("prompt %+v, scratch %+v", m.prompt(), m.scratch)
	}
}

func TestSaveAsUnchangedPrefillKeepsSlashName(t *testing.T) {
	ws := fixtureWS(t)
	users := folderByName(t, ws, "users")
	r := requestByName(t, users, "get user")
	r.Name = "GET /v1/users"
	if err := ws.SaveRequest(r); err != nil {
		t.Fatal(err)
	}
	m, _ := sendModel(t, ws, "http://example.invalid")
	m = press(m, "4", "W")
	if m.input() == nil || m.input().ti.Value() != "users/GET /v1/users" {
		t.Fatalf("prefill = %+v", m.input())
	}
	m = press(m, "enter")
	if got := m.Status(); got != "saved as users/GET /v1/users" {
		t.Fatalf("status = %q", got)
	}
	if n := len(ws.Collections.Folders); n != 2 || len(users.Folders) != 0 {
		t.Fatalf("stray folders created: root %d, users %d", n, len(users.Folders))
	}
}

func TestSaveAsTypedPathCreatesFolders(t *testing.T) {
	ws := fixtureWS(t)
	m, _ := sendModel(t, ws, "http://example.invalid")
	m = submitInput(t, press(m, "4", "W"), "new/sub/copy")
	if got := m.Status(); got != "saved as new/sub/copy" {
		t.Fatalf("status = %q", got)
	}
	sub := folderByName(t, ws, "new").Folders
	if len(sub) != 1 || sub[0].Name != "sub" || len(sub[0].Requests) != 1 {
		t.Fatalf("folders = %+v", sub)
	}
}
