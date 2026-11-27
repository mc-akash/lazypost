package ui

import (
	"strings"
	"testing"
)

func TestHelpOverlay(t *testing.T) {
	m := newTestModel(t, fixtureWS(t))
	if strings.Contains(m.plainRender(), "send the selected request") {
		t.Fatal("help visible before ?")
	}
	m = press(m, "?")
	if out := m.plainRender(); !strings.Contains(out, "send the selected request") {
		t.Fatal("help overlay should list send")
	}
	if m = press(m, "esc"); m.overlay != nil {
		t.Fatal("esc should close help")
	}
}

func TestHelpGroupedByPanel(t *testing.T) {
	var titles []string
	for _, r := range helpRows(helpEntries("")) {
		if r.entry < 0 {
			titles = append(titles, r.line)
		}
	}
	want := []string{"Global", "Environments", "Variables", "Collections", "Request", "Body editor", "Response", "History"}
	if strings.Join(titles, ",") != strings.Join(want, ",") {
		t.Fatalf("sections = %v", titles)
	}
}

func TestHelpEveryEntryReachableAndFits(t *testing.T) {
	for _, size := range [][2]int{{80, 20}, {120, 40}} {
		m := press(resize(newTestModel(t, fixtureWS(t)), size[0], size[1]), "?")
		seen := map[string]bool{}
		for range helpEntries("") {
			out := m.plainRender()
			lines := strings.Split(out, "\n")
			if len(lines) != size[1] {
				t.Fatalf("%v: render has %d lines", size, len(lines))
			}
			for i, l := range lines {
				if w := displayWidth(l); w > size[0] {
					t.Fatalf("%v: line %d is %d cells wide", size, i, w)
				}
			}
			for _, b := range allBindings() {
				if b.help != "" && strings.Contains(out, b.help) {
					seen[b.help] = true
				}
			}
			m = press(m, "down")
		}
		for _, b := range allBindings() {
			if b.help != "" && !seen[b.help] {
				t.Errorf("%v: moving down never revealed %q", size, b.help)
			}
		}
		if m.Focus() != PanelCollections {
			t.Fatal("moving in help must not move panel focus")
		}
	}
}

func TestHelpSearchMatchesKeyOrDescription(t *testing.T) {
	m := typeText(press(newTestModel(t, fixtureWS(t)), "?"), "cURL")
	out := m.plainRender()
	if !strings.Contains(out, "> cURL") || !strings.Contains(out, "copy request as cURL") {
		t.Fatalf("search by description:\n%s", out)
	}
	if strings.Contains(out, "send the selected request") {
		t.Fatal("non-matching rows should be hidden")
	}
	for _, e := range helpEntries("ctrl+s") {
		if !strings.Contains(e.b.keyText(), "ctrl+s") && !strings.Contains(e.b.help, "ctrl+s") {
			t.Fatalf("unexpected match %q", e.b.help)
		}
	}
	if len(helpEntries("ctrl+s")) == 0 {
		t.Fatal("search by key should match the body editor save")
	}
	if m = typeText(press(m, "esc"), "zzzz"); !strings.Contains(m.plainRender(), pickerNoMatch) {
		t.Fatal("no match should say so")
	}
	if m = press(m, "esc"); m.help() == nil || m.help().query != "" {
		t.Fatal("esc with a query should clear it, not close")
	}
	if m = press(m, "esc"); m.overlay != nil {
		t.Fatal("esc with no query should close")
	}
}

func TestHelpTypingDoesNotRunKeys(t *testing.T) {
	m := typeText(press(newTestModel(t, fixtureWS(t)), "?"), "qjg?")
	if m.help() == nil || m.help().query != "qjg?" {
		t.Fatalf("letters should go to the search, overlay = %#v", m.overlay)
	}
	if m = press(m, "backspace"); m.help().query != "qjg" {
		t.Fatalf("backspace query = %q", m.help().query)
	}
}

func TestHelpEnterRunsActionInItsPanel(t *testing.T) {
	m := press(newTestModel(t, fixtureWS(t)), "2", "j", "enter")
	m = press(typeText(press(m, "2", "?"), "pick method"), "enter")
	if m.Focus() != PanelRequest || m.picker() == nil || m.picker().title != methodPickerTitle {
		t.Fatalf("focus = %v, overlay = %#v", m.Focus(), m.overlay)
	}
}

func TestHelpEnterRunsGlobalAction(t *testing.T) {
	m := press(typeText(press(newTestModel(t, fixtureWS(t)), "?"), "jump to panel"), "enter")
	if m.overlay != nil || m.Focus() != PanelEnvs {
		t.Fatalf("focus = %v, overlay = %#v", m.Focus(), m.overlay)
	}
}

func TestHelpBodyEditorRowsDoNotRun(t *testing.T) {
	m := press(typeText(press(newTestModel(t, fixtureWS(t)), "?"), "save the body"), "enter")
	if m.help() == nil {
		t.Fatal("body editor rows only work inside the editor; enter should keep help open")
	}
}
