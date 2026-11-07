package ui

import "testing"

func TestTabJumpKeys(t *testing.T) {
	m := resize(newTestModel(t, fixtureWS(t)), 120, 24)
	m = press(m, "j", "enter")
	for _, tc := range []struct {
		key string
		tab string
	}{{"P", "Params"}, {"A", "Auth"}, {"B", "Body"}, {"H", "Headers"}, {"A", "Auth"}, {"A", "Auth"}} {
		m = press(m, tc.key)
		if got := reqTabs[m.request.tab]; got != tc.tab {
			t.Fatalf("%s: tab %s, want %s", tc.key, got, tc.tab)
		}
		if m.request.row != 0 {
			t.Fatalf("%s: row %d, want 0", tc.key, m.request.row)
		}
	}
}

func TestTabJumpKeysOnlyInRequest(t *testing.T) {
	m := resize(newTestModel(t, fixtureWS(t)), 120, 24)
	m = press(m, "j", "enter", "esc", "P")
	if m.request.tab != 0 {
		t.Fatalf("P in Collections moved the tab to %s", reqTabs[m.request.tab])
	}
}
