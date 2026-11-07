package ui

import (
	"strings"
	"testing"

	"github.com/mc-akash/lazypost/internal/workspace"
)

func restoredScratchModel(t *testing.T, ws *workspace.Workspace) Model {
	t.Helper()
	d := testDeps(t, ws)
	d.Session.Scratch = &workspace.Request{Method: "POST", URL: "http://old.test/call"}
	return resize(newModel(d), 120, 30)
}

func TestRestoredScratchShownWhenNothingSelected(t *testing.T) {
	for name, ws := range map[string]func(*testing.T) *workspace.Workspace{"empty": emptyWS, "folder": fixtureWS} {
		m := press(restoredScratchModel(t, ws(t)), "4")
		out := m.plainRender()
		if !strings.Contains(out, unsavedTitle) || !strings.Contains(out, "http://old.test/call") {
			t.Fatalf("%s: [4] does not show the unsaved request:\n%s", name, out)
		}
		before := out
		m = press(m, "enter")
		if got := m.plainRender(); !strings.Contains(got, "http://old.test/call") || strings.Count(got, unsavedTitle) != strings.Count(before, unsavedTitle) {
			t.Fatalf("%s: [4] changed after enter", name)
		}
	}
}

func TestSelectedRowWinsOverScratch(t *testing.T) {
	m := restoredScratchModel(t, fixtureWS(t))
	m = press(m, "j", "4")
	out := m.plainRender()
	if strings.Contains(out, unsavedTitle) || !strings.Contains(out, "{{base_url}}/orders") {
		t.Fatalf("[4] should show the selected request:\n%s", out)
	}
}
