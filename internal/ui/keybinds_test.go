package ui

import (
	"regexp"
	"strings"
	"testing"

	"github.com/mc-akash/lazypost/internal/config"
)

func keymapWith(set func(*config.Keymap)) config.Keymap {
	k := config.DefaultKeymap()
	set(&k)
	return k
}

func rebind(t *testing.T, set func(*config.Keymap)) {
	t.Helper()
	t.Cleanup(func() {
		if err := ApplyKeybinds(config.DefaultKeymap()); err != nil {
			t.Fatal(err)
		}
	})
	if err := ApplyKeybinds(keymapWith(set)); err != nil {
		t.Fatal(err)
	}
}

func TestDefaultKeybindsValid(t *testing.T) {
	rebind(t, func(*config.Keymap) {})
	if km.Send[0] != "s" || km.NextMatch[0] != "n" || km.NewUnsaved[0] != "N" {
		t.Fatalf("send=%q next_match=%q new_unsaved=%q", km.Send, km.NextMatch, km.NewUnsaved)
	}
}

func TestKeybindOverrideSends(t *testing.T) {
	rebind(t, func(k *config.Keymap) { k.Send = config.Keys{"ctrl+r"} })
	m, _ := sendModel(t, fixtureWS(t), "http://127.0.0.1:1")
	if press(m, "s").flight != nil {
		t.Fatal("s still sends after send was rebound")
	}
	if press(m, "ctrl+r").flight == nil {
		t.Fatal("ctrl+r does not send")
	}
}

func TestKeybindListAllSend(t *testing.T) {
	rebind(t, func(k *config.Keymap) { k.Send = config.Keys{"s", "ctrl+r"} })
	m, _ := sendModel(t, fixtureWS(t), "http://127.0.0.1:1")
	for _, k := range []string{"s", "ctrl+r"} {
		if press(m, k).flight == nil {
			t.Fatalf("%s does not send", k)
		}
	}
}

func TestKeybindNextItemReplacesDefaults(t *testing.T) {
	rebind(t, func(k *config.Keymap) { k.NextItem = config.Keys{"x"} })
	m := press(newTestModel(t, fixtureWS(t)), "2")
	start := m.collections.cursor
	for _, k := range []string{"j", "down"} {
		if press(m, k).collections.cursor != start {
			t.Fatalf("%s still moves the cursor", k)
		}
	}
	if press(m, "x").collections.cursor != start+1 {
		t.Fatal("x does not move the cursor")
	}
}

func TestKeybindJumpToPanel(t *testing.T) {
	rebind(t, func(k *config.Keymap) { k.JumpToPanel = config.Keys{"b", "o", "v", "x", "z"} })
	m := newTestModel(t, fixtureWS(t))
	if press(m, "z").focus != PanelResponse || press(m, "b").focus != PanelEnvs {
		t.Fatal("rebound jump keys do not focus panels")
	}
	if press(m, "1").focus == PanelEnvs {
		t.Fatal("1 still jumps to [1]")
	}
}

func TestKeybindConfirmInInputDialog(t *testing.T) {
	rebind(t, func(k *config.Keymap) { k.Confirm = config.Keys{"ctrl+o"} })
	m := press(newTestModel(t, fixtureWS(t)), "j", "j", "n")
	if m = press(typeText(m, "Ping"), "enter"); m.input() == nil {
		t.Fatal("enter still confirms")
	}
	if m = press(m, "ctrl+o"); m.input() != nil || m.selectedRowLabel() != "Ping" {
		t.Fatalf("ctrl+o does not confirm: selected %q", m.selectedRowLabel())
	}
}

func TestKeybindGotoTop(t *testing.T) {
	rebind(t, func(k *config.Keymap) { k.GotoTop = config.Keys{"home"} })
	m := press(newTestModel(t, fixtureWS(t)), "2", "G")
	if press(m, "g").selectedRowLabel() == "orders" {
		t.Fatal("g still goes to the top")
	}
	if got := press(m, "home").selectedRowLabel(); got != "orders" {
		t.Fatalf("selected %q, want orders", got)
	}
	if p := press(typeText(press(requestModel(t), "m"), "p"), "home").picker(); p.filter != "p" {
		t.Fatalf("home in the method picker changed the filter to %q", p.filter)
	}
}

func TestKeybindTypingContextTypesPrintableNav(t *testing.T) {
	rebind(t, func(k *config.Keymap) { k.NextItem = config.Keys{"x", "down"} })
	m := press(jenkinsModel(t), "E", "/", "x")
	if p := m.picker(); p.filter != "x" || p.cursor != 0 {
		t.Fatalf("filter %q cursor %d, want x typed", p.filter, p.cursor)
	}
	m = press(newTestModel(t, fixtureWS(t)), "4", "a", "x")
	if p := m.picker(); p == nil || p.filter != "x" {
		t.Fatal("x not typed into the header picker")
	}
}

func TestKeybindOverrideTabJump(t *testing.T) {
	rebind(t, func(k *config.Keymap) { k.ParamsTab = config.Keys{"x"} })
	m := press(newTestModel(t, fixtureWS(t)), "j", "enter")
	if press(m, "P").request.tab != reqTabHeaders {
		t.Fatal("P still jumps to Params")
	}
	if press(m, "x").request.tab != reqTabParams {
		t.Fatal("x does not jump to Params")
	}
}

func TestKeybindOverrideShownInHelpAndHints(t *testing.T) {
	rebind(t, func(k *config.Keymap) { k.Send = config.Keys{"ctrl+r", "s"}; k.Search = config.Keys{"F"} })
	m := resize(newTestModel(t, fixtureWS(t)), 160, 80)
	if out := m.plainRender(); !strings.Contains(out, "send: ctrl+r/s") {
		t.Fatalf("hint line should show send: ctrl+r/s:\n%s", out)
	}
	help := press(m, "?").plainRender()
	if !regexp.MustCompile(`ctrl\+r/s\s+send the selected request`).MatchString(help) {
		t.Fatalf("help should list ctrl+r/s for send:\n%s", help)
	}
	if line := hintLine(press(m, "E").hints()); !strings.Contains(line, "filter: F") {
		t.Fatalf("env picker should show the rebound filter key: %q", line)
	}
}

func TestHintsFollowRebindWithoutRebuild(t *testing.T) {
	m := resize(newTestModel(t, fixtureWS(t)), 200, 40)
	km.Send = []string{"s", "ctrl+r"}
	km.New = []string{"+"}
	t.Cleanup(func() { km = config.DefaultKeymap() })
	if hint := lastLine(press(m, "5").plainRender()); !strings.Contains(hint, "send: s/ctrl+r") {
		t.Fatalf("[5] hint line = %q", hint)
	}
	if hint := lastLine(press(m, "1").plainRender()); !strings.Contains(hint, "new: +") || strings.Contains(hint, "new: n") {
		t.Fatalf("[1] hint line = %q", hint)
	}
}

func TestRejectedKeybindsKeepCurrentKeys(t *testing.T) {
	err := ApplyKeybinds(keymapWith(func(k *config.Keymap) { k.Paste = config.Keys{"s"} }))
	if err == nil || !strings.Contains(err.Error(), `send and paste both use "s"`) {
		t.Fatalf("err = %v", err)
	}
	if km.Send[0] != "s" || km.Paste[0] != "p" {
		t.Fatal("a rejected config changed the keys")
	}
}

func TestKeybindSameKeyInSeparatePanels(t *testing.T) {
	rebind(t, func(k *config.Keymap) {
		k.New = config.Keys{"x"}
		k.NextMatch = config.Keys{"x"}
		k.Paste = config.Keys{"m"}
	})
	if km.New[0] != "x" || km.NextMatch[0] != "x" || km.Paste[0] != "m" {
		t.Fatalf("new=%q next_match=%q paste=%q", km.New, km.NextMatch, km.Paste)
	}
}

func TestHintTextsFollowRebinds(t *testing.T) {
	rebind(t, func(k *config.Keymap) { k.SwitchEnv = config.Keys{"X"}; k.EditURL = config.Keys{"U"} })
	if got := saveFailStatus("save failed: ", errSaveNeedsEnv); !strings.Contains(got, "(X)") {
		t.Fatalf("save status = %q", got)
	}
	ws := fixtureWS(t)
	if out := newTestModel(t, ws).plainRender(); !strings.Contains(out, "U: set a URL") {
		t.Fatalf("request panel hint not rebound:\n%s", out)
	}
}

func TestBindingListsDispatch(t *testing.T) {
	m := press(newTestModel(t, fixtureWS(t)), "2", "j")
	for _, s := range helpSections() {
		for _, b := range s.bindings {
			if len(b.keys) == 0 || b.run == nil && b.help == "" {
				t.Errorf("%s: binding %v lacks keys or text", s.title, b.keys)
			}
		}
	}
	for _, b := range m.collectionsBindings() {
		if _, _, ok := dispatch(m, (*b.keys[0])[0], m.panelBindings()); b.run != nil && !ok {
			t.Errorf("[2] %q not dispatched", (*b.keys[0])[0])
		}
	}
	if got := press(m, "w").status; got != statusNoDraft {
		t.Errorf("w status = %q", got)
	}
	if got := clipText(press(m, "c")); !strings.HasPrefix(got, "curl") {
		t.Errorf("c clipboard = %q", got)
	}
	if _, ok := press(m, "d").overlay.(*keyPrompt); !ok {
		t.Error("d should ask to confirm the delete")
	}
}
