package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/mc-akash/lazypost/internal/history"
	"github.com/mc-akash/lazypost/internal/workspace"
)

func TestEditWithNothingSelectedStartsScratch(t *testing.T) {
	for _, tc := range []struct {
		name string
		ws   func(*testing.T) *Model
	}{
		{"empty workspace", func(t *testing.T) *Model { m := resize(newTestModel(t, emptyWS(t)), 100, 24); return &m }},
		{"cursor on folder", func(t *testing.T) *Model { m := resize(newTestModel(t, fixtureWS(t)), 100, 24); return &m }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := press(*tc.ws(t), "4", "u")
			if m.input() == nil {
				t.Fatalf("u opened no dialog; status %q", m.status)
			}
			if !m.scratchSelected() {
				t.Fatal("scratch not selected after u")
			}
			m = typeText(m, "https://example.test/x")
			m = press(m, "enter")
			if got := m.scratch.URL; got != "https://example.test/x" {
				t.Fatalf("scratch URL = %q", got)
			}
			if !strings.Contains(m.plainRender(), unsavedTitle) {
				t.Fatal("title does not say unsaved")
			}
		})
	}
}

func TestEditWithNothingSelectedKeepsScratch(t *testing.T) {
	m := resize(newTestModel(t, emptyWS(t)), 100, 24)
	m = press(m, "N")
	m = typeText(m, "https://keep.test")
	m = press(m, "enter")
	m.onScratch = false
	m = press(m, "4", "m")
	if !m.scratchSelected() || m.scratch.URL != "https://keep.test" {
		t.Fatalf("scratch reset or not selected: %+v", m.scratch)
	}
}

func TestEditOnSelectedRequestUnaffected(t *testing.T) {
	m := resize(newTestModel(t, fixtureWS(t)), 100, 24)
	m = press(m, "j", "enter", "u")
	if m.scratchSelected() {
		t.Fatal("editing a collection request switched to the scratch")
	}
	if m.input() == nil {
		t.Fatal("u on a selected request opened no dialog")
	}
}

func withHistoryEntry(t *testing.T, m Model) (Model, history.Entry) {
	t.Helper()
	e := history.Entry{
		ID: "h1", Root: m.ws().Root, Time: time.Now(), Code: 200,
		Template: workspace.Request{
			Name: "get user", Method: "POST", URL: "http://hist.test/u",
			Headers: []workspace.KV{{Key: "Accept", Value: "text/plain"}},
		},
	}
	next, _ := m.Update(historyLoadedMsg{entries: []history.Entry{e}})
	return press(next.(Model), "3", "4"), e
}

func TestEditHistoryEntryCopiesIntoScratch(t *testing.T) {
	m, e := withHistoryEntry(t, resize(newTestModel(t, fixtureWS(t)), 100, 24))
	m = press(m, "u")
	if m.input() == nil || m.input().title != dialogEditURL || m.input().ti.Value() != e.Template.URL {
		t.Fatalf("u on a history entry: input %+v status %q", m.input(), m.Status())
	}
	m = press(typeText(m, "/x"), "enter")
	if !m.scratchSelected() || m.scratch.URL != e.Template.URL+"/x" || m.scratch.Method != "POST" ||
		len(m.scratch.Headers) != 1 || m.scratch.Headers[0].Value != "text/plain" {
		t.Fatalf("scratch = %+v", m.scratch)
	}
	if got := m.history.entries[0].Template.URL; got != e.Template.URL {
		t.Fatalf("history entry changed: %q", got)
	}
}

func TestEditHistoryEntryAsksBeforeReplacingScratch(t *testing.T) {
	m := resize(newTestModel(t, fixtureWS(t)), 100, 24)
	m = press(typeText(press(m, "N"), "http://mine.test"), "enter")
	m, e := withHistoryEntry(t, m)
	m.onScratch = false
	m = press(m, "3", "4")

	no := press(m, "a")
	if no.prompt() == nil {
		t.Fatalf("no confirmation before replacing the unsaved request (status %q)", no.Status())
	}
	no = press(no, "n")
	if no.picker() != nil || no.scratch.URL != "http://mine.test" {
		t.Fatalf("n: picker %v scratch %+v", no.picker() != nil, no.scratch)
	}

	yes := press(press(m, "a"), "y")
	if yes.picker() == nil || !yes.scratchSelected() || yes.scratch.URL != e.Template.URL {
		t.Fatalf("y: picker %v scratch %+v", yes.picker() != nil, yes.scratch)
	}
}

func TestEditHistoryEntrySameAsScratchNoPrompt(t *testing.T) {
	m := resize(newTestModel(t, fixtureWS(t)), 100, 24)
	m, e := withHistoryEntry(t, m)
	m = m.storeScratch(e.Template)
	m = press(m, "m")
	if m.prompt() != nil || m.picker() == nil {
		t.Fatalf("prompt %v picker %v", m.prompt() != nil, m.picker() != nil)
	}
}

func TestRowEditsOnHistoryEntryCopyIntoScratch(t *testing.T) {
	for _, tc := range []struct {
		keys  []string
		check func(r workspace.Request) bool
	}{
		{[]string{"space"}, func(r workspace.Request) bool { return len(r.Headers) == 1 && r.Headers[0].Disabled }},
		{[]string{"d"}, func(r workspace.Request) bool { return len(r.Headers) == 0 }},
		{[]string{"B", "t"}, func(r workspace.Request) bool { return r.Body.Type == "json" }},
	} {
		m, _ := withHistoryEntry(t, resize(newTestModel(t, fixtureWS(t)), 100, 24))
		m = press(m, tc.keys...)
		if !m.scratchSelected() || !tc.check(*m.scratch) {
			t.Fatalf("%v: status %q scratch %+v", tc.keys, m.Status(), m.scratch)
		}
	}
}
