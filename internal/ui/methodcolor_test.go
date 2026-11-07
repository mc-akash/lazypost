package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/mc-akash/lazypost/internal/history"
	"github.com/mc-akash/lazypost/internal/workspace"
)

func TestRequestLineShowsFullColouredMethod(t *testing.T) {
	for _, method := range []string{"PATCH", "OPTIONS", "DELETE", "GET"} {
		m := resize(newTestModel(t, emptyWS(t)), 100, 24)
		m = press(typeText(press(m, "N"), "http://x.test"), "enter")
		m = m.storeScratch(workspace.Request{Method: method, URL: "http://x.test"})
		if !strings.Contains(m.Render(), methodStyle(method).Render(method+" ")) {
			t.Errorf("%s: request line does not show the full coloured method:\n%s", method, m.plainRender())
		}
	}
}

func TestHistoryRowsColourTheMethod(t *testing.T) {
	m := resize(newTestModel(t, fixtureWS(t)), 120, 30)
	entries := []history.Entry{
		{ID: "1", Root: m.ws().Root, Time: time.Now(), Code: 200, Template: workspace.Request{Name: "a", Method: "GET"}},
		{ID: "2", Root: m.ws().Root, Time: time.Now(), Code: 201, Template: workspace.Request{Name: "b", Method: "POST"}},
	}
	next, _ := m.Update(historyLoadedMsg{entries: entries})
	m = next.(Model)
	if !strings.Contains(m.Render(), methodStyle("POST").Render(methodLabel("POST"))) {
		t.Fatalf("unselected history row: POST not coloured:\n%s", m.plainRender())
	}
}

func TestMethodPickerColoursItems(t *testing.T) {
	m := resize(newTestModel(t, fixtureWS(t)), 120, 30)
	m = press(m, "j", "enter", "m")
	out := m.Render()
	for _, method := range []string{"POST", "DELETE", "PATCH"} {
		if !strings.Contains(out, methodStyle(method).Render(method)) {
			t.Errorf("picker: %s not coloured", method)
		}
	}
}
