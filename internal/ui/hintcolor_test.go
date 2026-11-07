package ui

import (
	"charm.land/lipgloss/v2"

	"strings"
	"testing"
)

func TestHintLineColours(t *testing.T) {
	m := resize(newTestModel(t, fixtureWS(t)), 200, 24)
	m = press(m, "j", "enter")
	lines := strings.Split(m.Render(), "\n")
	line := lines[len(lines)-1]
	for _, tc := range []struct {
		b     binding
		style hintKind
	}{
		{binding{keys: keys(&km.EditURL), hint: "url"}, hintDefault},
		{binding{keys: keys(&km.Method), hint: "method"}, hintDefault},
		{binding{keys: keys(&km.Send), hint: "send"}, hintDefault},
		{binding{keys: keys(&km.Add), hint: "add"}, hintCreate},
		{binding{keys: keys(&km.SaveAs), hint: "save as"}, hintCreate},
		{binding{keys: keys(&km.ImportCurl), hint: "import cURL"}, hintCreate},
		{binding{keys: keys(&km.NewUnsaved), hint: "new unsaved request"}, hintCreate},
		{binding{keys: keys(&km.Delete), hint: "delete"}, hintDestroy},
	} {
		want := hintStyles[tc.style].Render(hintText(tc.b))
		if !strings.Contains(line, want) {
			t.Errorf("%q not rendered as %v in %q", hintText(tc.b), tc.style, line)
		}
	}
	if kindOf(binding{keys: keys(&km.New), hint: "new"}) != hintCreate {
		t.Error("n: new should be a create hint")
	}
}

func TestHintLineFitsWidth(t *testing.T) {
	m := resize(newTestModel(t, fixtureWS(t)), 80, 24)
	m = press(m, "j", "enter")
	lines := strings.Split(m.Render(), "\n")
	hint := lines[len(lines)-1]
	if w := displayWidth(hint); w > 80 {
		t.Fatalf("hint line is %d cells wide", w)
	}
	plain := ansiSeq.ReplaceAllString(hint, "")
	if !strings.Contains(plain, "…") || !strings.HasSuffix(plain, "keybinds: ?") || !strings.HasPrefix(plain, " edit: enter") {
		t.Fatalf("plain hint %q", plain)
	}
	if strings.Count(hint, "\x1b[") == 0 {
		t.Fatal("hint line has no colour")
	}
}

func TestHintLinePipeSeparated(t *testing.T) {
	m := resize(newTestModel(t, fixtureWS(t)), 200, 24)
	lines := strings.Split(m.plainRender(), "\n")
	hint := lines[len(lines)-1]
	if !strings.Contains(hint, "send: s | expand/open: enter | new: n") {
		t.Fatalf("hint %q", hint)
	}
}

func TestFocusedPaneGreen(t *testing.T) {
	m := resize(newTestModel(t, fixtureWS(t)), 120, 24)
	out := m.Render()
	if !strings.Contains(out, focusTitle.Render("[2] Collections")) {
		t.Fatal("focused title not in the focus style")
	}
	if focusBorder.GetForeground() != lipgloss.Green || focusTitle.GetForeground() != lipgloss.Green {
		t.Fatal("focus style is not green")
	}
	if strings.Contains(out, focusTitle.Render("[1] Environments")) {
		t.Fatal("unfocused pane drawn in the focus style")
	}
}

func TestHintLineOmitsBasicKeys(t *testing.T) {
	basic := []string{": j/k", ": ↑/↓", "quit: q", "back: esc", "close: esc", "cancel: esc", "tab: [ ]"}
	m := resize(newTestModel(t, fixtureWS(t)), 300, 30)
	m = press(m, "j", "enter")
	check := func(where string, m Model) {
		t.Helper()
		hint := lastLine(m.plainRender())
		for _, b := range basic {
			if strings.Contains(hint, b) {
				t.Errorf("%s: hint %q shows %q", where, hint, b)
			}
		}
	}
	for _, p := range []string{"1", "2", "3", "4", "5"} {
		check("panel "+p, press(m, p))
	}
	check("vars table", press(m, "1", "enter"))
	check("env picker", press(m, "E"))
	if hint := lastLine(press(m, "E").plainRender()); !strings.Contains(hint, "activate: enter") {
		t.Errorf("env picker hint %q should say enter activates", hint)
	}
	check("method picker", press(m, "4", "m"))
	check("header picker", press(m, "4", "H", "a"))
	if hint := lastLine(press(m, "?").plainRender()); strings.Contains(hint, "quit: q") || strings.Contains(hint, ": j/k") {
		t.Errorf("help: hint %q", hint)
	}
}

func TestHintLineKeybinds(t *testing.T) {
	for _, w := range []int{200, 80} {
		m := resize(newTestModel(t, fixtureWS(t)), w, 24)
		m = press(m, "j", "enter")
		for _, p := range []string{"1", "2", "3", "4", "5"} {
			hint := lastLine(press(m, p).plainRender())
			if !strings.HasSuffix(hint, "keybinds: ?") {
				t.Errorf("width %d panel %s: hint %q", w, p, hint)
			}
			if displayWidth(hint) > w {
				t.Errorf("width %d panel %s: hint is %d wide", w, p, displayWidth(hint))
			}
		}
	}
}
