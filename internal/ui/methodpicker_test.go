package ui

import (
	"strings"
	"testing"
)

func requestModel(t *testing.T) Model {
	t.Helper()
	m, _ := sendModel(t, fixtureWS(t), "http://example.invalid")
	return press(m, "4")
}

func TestMethodPickerFilterPicksPatch(t *testing.T) {
	m := press(requestModel(t), "m")
	if m.picker() == nil {
		t.Fatal("m should open the method picker")
	}
	out := m.plainRender()
	for _, want := range []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"} {
		if !strings.Contains(out, want) {
			t.Fatalf("picker lacks %s:\n%s", want, out)
		}
	}
	if !strings.Contains(out, "● GET") {
		t.Fatalf("current method not marked:\n%s", out)
	}
	m = press(typeText(m, "pa"), "enter")
	if m.picker() != nil {
		t.Fatal("enter should close the picker")
	}
	if got := currentTmpl(m).Method; got != "PATCH" {
		t.Fatalf("method = %q, want PATCH", got)
	}
	if !strings.Contains(m.plainRender(), "[4] Request ●") {
		t.Fatal("draft marker missing")
	}
}

func TestMethodPickerEscKeepsMethod(t *testing.T) {
	m := press(requestModel(t), "m", "j", "esc")
	if m.picker() != nil || currentTmpl(m).Method != "GET" || len(m.drafts) != 0 {
		t.Fatalf("esc changed something: method %q drafts %v", currentTmpl(m).Method, m.drafts)
	}
}

func TestMethodPickerSameMethodNoDraft(t *testing.T) {
	m := press(requestModel(t), "m", "enter")
	if m.picker() != nil || currentTmpl(m).Method != "GET" || len(m.drafts) != 0 {
		t.Fatalf("enter on the current method: method %q drafts %v", currentTmpl(m).Method, m.drafts)
	}
}

func TestMethodPickerNavigation(t *testing.T) {
	m := press(requestModel(t), "m", "j", "enter")
	if got := currentTmpl(m).Method; got != "POST" {
		t.Fatalf("j, enter: %q", got)
	}
	m = press(m, "m", "j", "j", "up", "enter")
	if got := currentTmpl(m).Method; got != "PUT" {
		t.Fatalf("j, j, up, enter: %q", got)
	}
	m = press(m, "m", "g", "enter")
	if got := currentTmpl(m).Method; got != "GET" {
		t.Fatalf("g, enter: %q", got)
	}
}

func TestMethodPickerFilterOrderAndNoMatch(t *testing.T) {
	m := press(requestModel(t), "m", "p")
	if got := m.picker().matches(); strings.Join(got, ",") != "POST,PUT,PATCH,OPTIONS" {
		t.Fatalf("matches for p = %v (prefix matches first)", got)
	}
	if m = press(m, "backspace", "D"); strings.Join(m.picker().matches(), ",") != "DELETE,HEAD" {
		t.Fatalf("matches for D = %v", m.picker().matches())
	}
	m = press(m, "x", "enter")
	if m.picker() != nil || currentTmpl(m).Method != "GET" || len(m.drafts) != 0 {
		t.Fatal("enter with no match should close without a change")
	}
}

func TestMethodPickerWithNothingSelectedStartsScratch(t *testing.T) {
	m := press(newTestModel(t, fixtureWS(t)), "3", "4", "m")
	if m.picker() == nil || !m.scratchSelected() {
		t.Fatalf("picker %v scratch %v status %q", m.picker(), m.scratchSelected(), m.Status())
	}
}
