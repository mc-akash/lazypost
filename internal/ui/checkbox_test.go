package ui

import (
	"strings"
	"testing"
)

func TestKVRowsShowCheckbox(t *testing.T) {
	m := resize(newTestModel(t, emptyWS(t)), 100, 24)
	m = press(typeText(press(m, "N"), "http://x.test"), "enter")
	m = press(typeText(press(m, "P", "a"), "q: 1"), "enter")
	m = press(typeText(press(m, "a"), "r: 2"), "enter")
	view := func(m Model) string { return strings.Join(strings.Split(m.plainRender(), "\n")[:6], "\n") }
	if out := view(m); !strings.Contains(out, "[x] q=1") || !strings.Contains(out, "[x] r=2") {
		t.Fatalf("enabled params:\n%s", out)
	}
	m = press(m, "k", "space")
	out := view(m)
	if !strings.Contains(out, "[ ] q=1") || !strings.Contains(out, "[x] r=2") {
		t.Fatalf("after toggle:\n%s", out)
	}

	m = press(m, "j")
	if !strings.Contains(m.Render(), mutedStyle.Render("[ ] q=1")) {
		t.Fatal("disabled row is not dimmed")
	}
	m = press(m, "H", "a")
	m = press(typeText(m, "X-Foo"), "enter")
	m = press(typeText(m, "bar"), "enter")
	if out := view(m); !strings.Contains(out, "[x] X-Foo: bar") {
		t.Fatalf("header row:\n%s", out)
	}
}
