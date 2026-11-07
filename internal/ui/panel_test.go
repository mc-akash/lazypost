package ui

import "testing"

func TestPanelEnum(t *testing.T) {
	cases := []struct {
		p          Panel
		name, key  string
		side, main bool
	}{
		{PanelEnvs, "Environments", "1", true, false},
		{PanelCollections, "Collections", "2", true, false},
		{PanelHistory, "History", "3", true, false},
		{PanelRequest, "Request", "4", false, true},
		{PanelResponse, "Response", "5", false, true},
		{PanelVars, "Variables", "", false, false},
	}
	for _, c := range cases {
		if c.p.String() != c.name || c.p.isSide() != c.side || c.p.isMain() != c.main {
			t.Errorf("%d: %q side=%v main=%v", int(c.p), c.p.String(), c.p.isSide(), c.p.isMain())
		}
		if c.key != "" && km.JumpToPanel[int(c.p)-1] != c.key {
			t.Errorf("panel %v jump key %s, want %s", c.p, km.JumpToPanel[int(c.p)-1], c.key)
		}
	}
	if Panel(0).String() != "Panel(0)" {
		t.Errorf("zero value: %q", Panel(0).String())
	}
}
