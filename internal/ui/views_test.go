package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/mc-akash/lazypost/internal/workspace"
)

func TestResponseTitleFormat(t *testing.T) {
	cases := []struct {
		size      int64
		truncated bool
		want      string
	}{
		{512, false, "200 OK · 142ms · 512B"},
		{1229, false, "200 OK · 142ms · 1.2KB"},
		{5 << 20, false, "200 OK · 142ms · 5.0MB"},
		{32 << 20, true, "200 OK · 142ms · 32.0MB (truncated)"},
	}
	for _, c := range cases {
		if got := responseTitle("200 OK", 142*time.Millisecond, c.size, c.truncated); got != c.want {
			t.Errorf("responseTitle(%d, %v) = %q, want %q", c.size, c.truncated, got, c.want)
		}
	}
}

func TestUndefinedVarHighlighted(t *testing.T) {
	ws := fixtureWS(t)
	req := requestByName(t, folderByName(t, ws, "users"), "get user")
	req.Headers = append(req.Headers, workspace.KV{Key: "X-Trace", Value: "{{trace_id}}"})
	m, _ := sendModel(t, ws, "http://example.invalid")
	out := m.Render()
	if !strings.Contains(out, missingVarStyle.Render("{{trace_id}}")) {
		t.Fatalf("undefined var not highlighted:\n%q", out)
	}
	if strings.Contains(out, missingVarStyle.Render("{{base_url}}")) {
		t.Fatal("defined var highlighted as missing")
	}
	if !strings.Contains(out, knownVarStyle.Render("{{base_url}}")) {
		t.Fatal("defined var should use the accent style")
	}
}

func TestRequestViewTabs(t *testing.T) {
	ws := fixtureWS(t)
	req := requestByName(t, folderByName(t, ws, "users"), "get user")
	req.Headers = []workspace.KV{{Key: "Accept", Value: "application/json"}, {Key: "X-Off", Value: "1", Disabled: true}}
	req.Params = []workspace.KV{{Key: "page", Value: "2"}}
	req.Body = workspace.Body{Type: "json", Content: "{\"name\": \"ada\"}"}
	bearerFolder(t, ws)
	m, _ := sendModel(t, ws, "http://example.invalid")

	out := m.plainRender()
	for _, want := range []string{"GET {{base_url}}/users/1", "Headers  Body  Auth  Params", "Accept: application/json"} {
		if !strings.Contains(out, want) {
			t.Errorf("headers tab missing %q", want)
		}
	}
	if !strings.Contains(m.Render(), mutedStyle.Render("[ ] X-Off: 1")) {
		t.Error("disabled header should be dimmed")
	}
	m = press(m, "4", "]")
	if out = m.plainRender(); !strings.Contains(out, `{"name": "ada"}`) {
		t.Errorf("body tab missing content:\n%s", out)
	}
	m = press(m, "]")
	if out = m.plainRender(); !strings.Contains(out, "bearer (inherited)") || !strings.Contains(out, "Token: {{token}}") {
		t.Errorf("auth tab missing inherited bearer:\n%s", out)
	}
	m = press(m, "]")
	if out = m.plainRender(); !strings.Contains(out, "page=2") {
		t.Errorf("params tab missing page=2:\n%s", out)
	}
	m = press(m, "]")
	if out = m.plainRender(); !strings.Contains(out, "Accept: application/json") {
		t.Error("] should wrap around to Headers")
	}
	m = press(m, "[")
	if out = m.plainRender(); !strings.Contains(out, "page=2") {
		t.Error("[ should go back to Params")
	}
}

func TestResponseTabs(t *testing.T) {
	srv := serve(t, &recorder{contentType: "text/plain", body: []byte("plain body")})
	m, _ := sendModel(t, fixtureWS(t), srv.URL)
	m = press(pressRun(m, "s"), "5")
	if out := m.plainRender(); !strings.Contains(out, "Body  Headers  Cookies") || !strings.Contains(out, "plain body") {
		t.Fatalf("body tab:\n%s", out)
	}
	m = press(m, "]")
	if out := m.plainRender(); !strings.Contains(out, "Content-Type: text/plain") {
		t.Fatalf("headers tab:\n%s", out)
	}
}

func numberedBody(n, needleAt int) []byte {
	var b strings.Builder
	for i := range n {
		if i == needleAt {
			fmt.Fprintf(&b, "line %d needle\n", i)
			continue
		}
		fmt.Fprintf(&b, "line %d\n", i)
	}
	return []byte(b.String())
}

func TestResponseSearch(t *testing.T) {
	srv := serve(t, &recorder{contentType: "text/plain", body: numberedBody(60, 30)})
	m, _ := sendModel(t, fixtureWS(t), srv.URL)
	m = press(pressRun(m, "s"), "5")
	if strings.Contains(m.plainRender(), "line 30 needle") {
		t.Fatal("line 30 should start out of view")
	}
	m = press(m, "/", "n", "e", "e", "d", "l", "e", "enter")
	if !strings.Contains(m.plainRender(), "line 30 needle") {
		t.Fatalf("search did not scroll line 30 into view:\n%s", m.plainRender())
	}
	m = press(m, "g")
	if strings.Contains(m.plainRender(), "line 30 needle") {
		t.Fatal("g should scroll back to the top")
	}
	if m = press(m, "n"); !strings.Contains(m.plainRender(), "line 30 needle") {
		t.Fatal("n should jump to the match again")
	}
	if m = press(m, "g", "g", "N"); !strings.Contains(m.plainRender(), "line 30 needle") {
		t.Fatal("N should jump to the match again")
	}
}

func TestResponseSearchEscCancels(t *testing.T) {
	srv := serve(t, &recorder{contentType: "text/plain", body: numberedBody(60, 30)})
	m, _ := sendModel(t, fixtureWS(t), srv.URL)
	m = press(pressRun(m, "s"), "5")
	m = press(m, "/", "n", "e", "esc")
	if strings.Contains(m.plainRender(), "line 30 needle") {
		t.Fatal("esc should cancel the search")
	}
	if m = press(m, "q"); m.input() != nil {
		t.Fatal("input should be closed after esc")
	}
}

func TestResponseScroll(t *testing.T) {
	srv := serve(t, &recorder{contentType: "text/plain", body: numberedBody(60, -1)})
	m, _ := sendModel(t, fixtureWS(t), srv.URL)
	m = press(pressRun(m, "s"), "5")
	if m = press(m, "G"); !strings.Contains(m.plainRender(), "│ line 59 ") || strings.Contains(m.plainRender(), "│ line 0 ") {
		t.Fatalf("G should show the last line:\n%s", m.plainRender())
	}
	m = press(m, "g", "g", "j")
	if out := m.plainRender(); strings.Contains(out, "│ line 0 ") || !strings.Contains(out, "│ line 1 ") {
		t.Fatalf("j should scroll one line:\n%s", out)
	}
	m = press(m, "ctrl+d")
	if out := m.plainRender(); strings.Contains(out, "│ line 1 ") {
		t.Fatalf("ctrl+d should scroll half a page:\n%s", out)
	}
	m = press(m, "ctrl+u", "k")
	if out := m.plainRender(); !strings.Contains(out, "│ line 0 ") {
		t.Fatalf("ctrl+u, k should return to the top:\n%s", out)
	}
}

func TestInputGetsAllKeys(t *testing.T) {
	m := newTestModel(t, fixtureWS(t))
	var got string
	in := newInput("> ", "", func(m Model, v string) (Model, tea.Cmd) {
		got = v
		return m, nil
	})
	m.overlay = &in
	m = press(m, "q", "3", "enter")
	if got != "q3" {
		t.Fatalf("submitted %q, want q3", got)
	}
	if m.Focus() != PanelCollections || m.input() != nil {
		t.Fatal("keys leaked past the open input")
	}
}

func TestPanelContentPadded(t *testing.T) {
	out := newTestModel(t, fixtureWS(t)).plainRender()
	if !strings.Contains(out, "│ ▾ orders") {
		t.Fatalf("panel content should have one space of left padding:\n%s", out)
	}
}
