package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/mc-akash/lazypost/internal/history"
	"github.com/mc-akash/lazypost/internal/httpx"
	"github.com/mc-akash/lazypost/internal/workspace"
)

func assertNoInjection(t *testing.T, m Model) {
	t.Helper()
	out := m.Render()
	for _, bad := range []string{"\a", "\x1b]", "\x1bP", "\x1b_", "\x1b^", "\x1bX", "\x1b[2J", "\r"} {
		if strings.Contains(out, bad) {
			t.Fatalf("Render contains injected %q:\n%q", bad, out)
		}
	}
	if n := strings.Count(out, "\n") + 1; n != m.height {
		t.Fatalf("Render has %d lines, want %d (layout broken)", n, m.height)
	}
}

func TestServerStatusSanitized(t *testing.T) {
	m := press(newTestModel(t, fixtureWS(t)), "j")
	sel, ok := m.selection()
	if !ok {
		t.Fatal("no selection")
	}
	resp := &httpx.Response{Code: 200, Status: "200 OK\x1b]52;c;aGk=\a", Body: []byte("{}")}
	m.results = withResult(m.results, sel.key(), sendResult{phase: phaseDone, seq: 1, resp: resp})
	assertNoInjection(t, m)
	if !strings.Contains(m.plainRender(), "200 OK·]52;c;aGk=·") {
		t.Fatalf("sanitized status missing:\n%s", m.plainRender())
	}
}

func TestRequestNameSanitizedOnOneRow(t *testing.T) {
	ws := fixtureWS(t)
	users := ws.Collections.Folders[1]
	users.Requests[0].Name = "a\nb\x1bP"
	users.Requests[0].Method = "GET\x1b[2J"
	m := newTestModel(t, ws)
	for _, keys := range [][]string{nil, {"j", "j", "j"}} {
		m := press(m, keys...)
		assertNoInjection(t, m)
		if !strings.Contains(m.plainRender(), "a·b·P") {
			t.Fatalf("keys %v: sanitized name missing:\n%s", keys, m.plainRender())
		}
	}
}

func TestEnvNameSanitized(t *testing.T) {
	ws := fixtureWS(t)
	ws.Envs[0].Name = "e\x1b]0;x\a"
	m := newTestModel(t, ws)
	for _, keys := range [][]string{{"1"}, {"1", "j"}, {"E"}} {
		m := press(m, keys...)
		assertNoInjection(t, m)
		if !strings.Contains(m.plainRender(), "e·]0;x·") {
			t.Fatalf("keys %v: sanitized env missing:\n%s", keys, m.plainRender())
		}
	}
}

func TestHistoryRowSanitized(t *testing.T) {
	m := newTestModel(t, fixtureWS(t))
	e := history.Entry{
		ID: "x", Time: time.Now(), Code: 200, Root: m.ws().Root,
		Template: workspace.Request{Name: "n\x1b_x\x1b\\", Method: "GET\x1bP"},
		Sent:     workspace.Request{URL: "http://h/\x1b]8;;x\a"},
	}
	m.history = m.history.add(e, true, m.historyMax())
	m.history = m.history.add(history.Entry{ID: "y", Time: time.Now(), Root: m.ws().Root, Template: workspace.Request{Method: "GET"}, Sent: e.Sent}, false, m.historyMax())
	for _, keys := range [][]string{nil, {"3"}, {"3", "j"}} {
		m := press(m, keys...)
		assertNoInjection(t, m)
		if !strings.Contains(m.plainRender(), "n·_x·\\") {
			t.Fatalf("keys %v: sanitized history name missing:\n%s", keys, m.plainRender())
		}
	}
}

func TestStatusAndPromptsSanitized(t *testing.T) {
	m := newTestModel(t, fixtureWS(t))
	m.status = "boom\x1bPq\x1b\\\a"
	assertNoInjection(t, m)

	m.overlay = nil
	m = m.confirmDelete(dialogDelVar, "x\x1b]0;t\a", func(m Model) Model { return m })
	assertNoInjection(t, m)
	m.overlay = nil

	in := newInput("v\x1b]0;t\a", "", submitSearch).withHint("h\x1b]0;t\a")
	m.overlay = &in
	assertNoInjection(t, m)

	in = newBottomInput("v\x1b]0;t\a: ", "", submitSearch)
	m.overlay = &in
	assertNoInjection(t, m)
}
