package ui

import (
	"strings"
	"testing"

	"github.com/mc-akash/lazypost/internal/history"
	"github.com/mc-akash/lazypost/internal/workspace"
)

func TestResponseTop(t *testing.T) {
	srv := serve(t, &recorder{contentType: "text/plain", body: numberedBody(60, -1)})
	m, _ := sendModel(t, fixtureWS(t), srv.URL)
	m = press(pressRun(m, "s"), "5", "G")
	if m.response.scroll == 0 {
		t.Fatal("G should scroll down")
	}
	if m = press(m, "g"); m.response.scroll != 0 {
		t.Fatalf("g: scroll = %d, want 0", m.response.scroll)
	}
}

func TestCollectionsTopAndBottom(t *testing.T) {
	m := press(newTestModel(t, fixtureWS(t)), "2")
	if m = press(m, "G"); m.selectedRowLabel() != "get user" {
		t.Fatalf("G selected %q, want get user", m.selectedRowLabel())
	}
	if m = press(m, "g"); m.selectedRowLabel() != "orders" {
		t.Fatalf("g selected %q, want orders", m.selectedRowLabel())
	}
}

func TestEnvsTopAndBottom(t *testing.T) {
	m := press(newTestModel(t, fixtureWS(t)), "1")
	if m = press(m, "g"); m.selectedRowLabel() != "dev" {
		t.Fatalf("g selected %q", m.selectedRowLabel())
	}
	if m = press(m, "G"); m.selectedRowLabel() != "staging" {
		t.Fatalf("G selected %q", m.selectedRowLabel())
	}
}

func TestHistoryTopAndBottom(t *testing.T) {
	m := newTestModel(t, fixtureWS(t))
	m.history = historyPanel{entries: []history.Entry{
		{ID: "a", Template: workspace.Request{Name: "first"}},
		{ID: "b", Template: workspace.Request{Name: "second"}},
		{ID: "c", Template: workspace.Request{Name: "third"}},
	}}
	m = press(m, "3", "G")
	if m.selectedRowLabel() != "third" {
		t.Fatalf("G selected %q", m.selectedRowLabel())
	}
	if m = press(m, "g"); m.selectedRowLabel() != "first" {
		t.Fatalf("g selected %q", m.selectedRowLabel())
	}
}

func TestVarsTopAndBottom(t *testing.T) {
	m := press(newTestModel(t, fixtureWS(t)), "V")
	rows := m.varRows()
	if m = press(m, "G"); m.variables.cursor != len(rows)-1 {
		t.Fatalf("G cursor = %d, want %d", m.variables.cursor, len(rows)-1)
	}
	if m = press(m, "g"); m.variables.cursor != 0 {
		t.Fatalf("g cursor = %d", m.variables.cursor)
	}
}

func TestRequestRowsTopAndBottom(t *testing.T) {
	m, _ := sendModel(t, fixtureWS(t), "http://example.invalid")
	m = m.editRequest(func(r *workspace.Request) {
		r.Headers = []workspace.KV{{Key: "A"}, {Key: "B"}, {Key: "C"}}
	})
	m = press(m, "4")
	if m = press(m, "G"); m.reqRow() != 2 {
		t.Fatalf("G row = %d", m.reqRow())
	}
	if m = press(m, "g"); m.reqRow() != 0 {
		t.Fatalf("g row = %d", m.reqRow())
	}
}

func TestHelpTopAndBottom(t *testing.T) {
	m := resize(newTestModel(t, fixtureWS(t)), 120, 15)
	m = press(m, "?", "G")
	if m.help().scroll == 0 || m.help().scroll != helpMaxScroll(15) {
		t.Fatalf("G help scroll = %d, want %d", m.help().scroll, helpMaxScroll(15))
	}
	if m = press(m, "g"); m.help().scroll != 0 {
		t.Fatalf("g help scroll = %d", m.help().scroll)
	}
}

func TestEnvPickerTopAndBottom(t *testing.T) {
	m := press(newTestModel(t, fixtureWS(t)), "E")
	if m = press(m, "g"); m.picker().cursor != 0 || m.picker().filter != "" {
		t.Fatalf("g: cursor %d filter %q", m.picker().cursor, m.picker().filter)
	}
	if m = press(m, "G"); m.picker().cursor != 1 || m.picker().filter != "" {
		t.Fatalf("G: cursor %d filter %q", m.picker().cursor, m.picker().filter)
	}
	m = press(typeText(press(m, "/"), "stag"), "enter")
	if m.activeEnv().Name != "staging" || m.picker() != nil {
		t.Fatal(`filter "stag" should pick staging`)
	}
	if out := m.plainRender(); strings.Contains(out, "Switch environment") {
		t.Fatal("picker still drawn")
	}
}
