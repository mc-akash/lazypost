package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mc-akash/lazypost/internal/workspace"
)

func TestOutsideEditBeforeDraftIsStale(t *testing.T) {
	ws, req := acceptHeaderWS(t)
	m, _ := sendModel(t, ws, "http://example.invalid")
	later := time.Now().Add(time.Hour)
	if err := os.Chtimes(req.Path, later, later); err != nil {
		t.Fatal(err)
	}
	m = press(m, "4", "space").reload()
	if !m.stale[getUserKey] || !strings.Contains(m.plainRender(), "get user !") {
		t.Fatalf("outside edit before the draft not flagged stale:\n%s", m.plainRender())
	}
}

func TestSaveRefusesOutsideEditWithoutReload(t *testing.T) {
	ws, req := acceptHeaderWS(t)
	m, _ := sendModel(t, ws, "http://example.invalid")
	outside := "name: get user\nmethod: GET\nurl: http://outside.example/\n"
	if err := os.WriteFile(req.Path, []byte(outside), 0o644); err != nil {
		t.Fatal(err)
	}
	later := time.Now().Add(time.Hour)
	if err := os.Chtimes(req.Path, later, later); err != nil {
		t.Fatal(err)
	}
	m = press(m, "4", "space", "w")
	if got := readFile(t, req.Path); got != outside {
		t.Fatalf("first w overwrote the outside edit:\n%s", got)
	}
	if want := "changed on disk: get user (w overwrites)"; m.Status() != want || !m.stale[getUserKey] {
		t.Fatalf("status %q stale %v, want %q", m.Status(), m.stale[getUserKey], want)
	}
	if !strings.Contains(m.plainRender(), "get user !") {
		t.Fatalf("stale marker missing:\n%s", m.plainRender())
	}
	m = press(m, "w")
	if m.Status() != statusSaved || readFile(t, req.Path) == outside {
		t.Fatalf("second w on the stale draft did not overwrite: status %q", m.Status())
	}
	if _, ok := m.drafts[getUserKey]; ok || m.stale[getUserKey] {
		t.Fatal("draft or stale flag left after overwrite")
	}
}

func TestOwnWritesAreNotStale(t *testing.T) {
	ws, _ := acceptHeaderWS(t)
	m, _ := sendModel(t, ws, "http://example.invalid")
	m = press(m, "4", "space").reload()
	if m.stale[getUserKey] {
		t.Fatal("draft on a file lazypost wrote is flagged stale")
	}
	m = press(m, "w")
	if m.Status() != statusSaved {
		t.Fatalf("status = %q", m.Status())
	}
	m = press(m, "space").reload()
	if _, ok := m.drafts[getUserKey]; !ok || m.stale[getUserKey] {
		t.Fatalf("draft after own save: drafts %v stale %v", m.drafts, m.stale)
	}
	m = press(m, "w")
	if _, ok := m.drafts[getUserKey]; ok || m.Status() != statusSaved {
		t.Fatalf("second w after own save: status %q", m.Status())
	}
}

func TestSaveWithoutOutsideEditWritesFile(t *testing.T) {
	ws, req := acceptHeaderWS(t)
	m, _ := sendModel(t, ws, "http://example.invalid")
	m = press(m, "4", "space", "w")
	if m.Status() != statusSaved || !strings.Contains(readFile(t, req.Path), "disabled: true") {
		t.Fatalf("plain save failed: status %q\n%s", m.Status(), readFile(t, req.Path))
	}
}

func TestCreatingFirstEnvActivatesIt(t *testing.T) {
	ws, err := workspace.Load(filepath.Join(t.TempDir(), "ws"))
	if err != nil {
		t.Fatal(err)
	}
	m := resize(newModel(testDeps(t, ws)), 120, 40)
	if m.envName != "" {
		t.Fatalf("fresh workspace active env = %q", m.envName)
	}
	m, _ = submitNewEnv(m, "dev")
	if m.envName != "dev" || m.deps.Session.LastEnv != "dev" {
		t.Fatalf("first env not activated: env %q session %q", m.envName, m.deps.Session.LastEnv)
	}
	m, _ = submitNewEnv(m, "prod")
	if m.envName != "dev" || m.deps.Session.LastEnv != "dev" {
		t.Fatalf("second env switched active env to %q", m.envName)
	}
}

func TestRequestPanelScrollsToSelectedHeader(t *testing.T) {
	ws, req := acceptHeaderWS(t)
	req.Headers = nil
	for i := range 30 {
		req.Headers = append(req.Headers, workspace.KV{Key: fmt.Sprintf("X-H%02d", i), Value: fmt.Sprintf("v%02d", i)})
	}
	if err := ws.SaveRequest(req); err != nil {
		t.Fatal(err)
	}
	m, _ := sendModel(t, ws, "http://example.invalid")
	m = press(resize(m, 100, 24), "4", "G")
	out := m.plainRender()
	for _, want := range []string{"X-H29: v29", "/users/1", "Headers  Body"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q after G:\n%s", want, out)
		}
	}
	if strings.Contains(out, "X-H00: v00") {
		t.Fatalf("first header still shown, panel did not scroll:\n%s", out)
	}
}
