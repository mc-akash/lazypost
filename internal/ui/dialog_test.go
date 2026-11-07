package ui

import (
	"os"
	"strings"
	"testing"
)

func statusLine(s string) string {
	lines := strings.Split(s, "\n")
	return lines[len(lines)-2]
}

func dialogLine(t *testing.T, out, title string) string {
	t.Helper()
	for _, l := range strings.Split(out, "\n") {
		if strings.Contains(l, "┌"+title) {
			return l
		}
	}
	t.Fatalf("no dialog titled %q:\n%s", title, out)
	return ""
}

func assertCentered(t *testing.T, m Model, title string) {
	t.Helper()
	out := m.plainRender()
	for i, l := range strings.Split(out, "\n") {
		if displayWidth(l) > m.width {
			t.Fatalf("line %d is %d cells wide at width %d: %q", i, displayWidth(l), m.width, l)
		}
	}
	line := dialogLine(t, out, title)
	start := strings.Index(line, "┌"+title)
	left := displayWidth(line[:start])
	end := start + strings.Index(line[start:], "┐") + len("┐")
	right := m.width - displayWidth(line[:end])
	if d := left - right; d < -1 || d > 1 {
		t.Fatalf("dialog not centred at %dx%d: left %d, right %d", m.width, m.height, left, right)
	}
}

func TestNewRequestOpensDialog(t *testing.T) {
	ws := fixtureWS(t)
	m := press(newTestModel(t, ws), "2", "n")
	out := m.plainRender()
	dialogLine(t, out, dialogNewRequest)
	for _, want := range []string{"end with / to create a folder", "confirm: enter  cancel: esc"} {
		if !strings.Contains(out, want) {
			t.Fatalf("dialog lacks %q:\n%s", want, out)
		}
	}
	if st := statusLine(out); strings.Contains(st, "name") || strings.Contains(st, dialogNewRequest) {
		t.Fatalf("prompt still in the status line: %q", st)
	}
	if hint := lastLine(out); !strings.Contains(hint, "confirm: enter") || strings.Contains(hint, "send: s") {
		t.Fatalf("hint line should show the dialog keys: %q", hint)
	}
	m = press(typeText(m, "Ping"), "enter")
	if m.input() != nil || m.selectedRowLabel() != "Ping" {
		t.Fatalf("input %v, selected %q", m.input(), m.selectedRowLabel())
	}
	if strings.Contains(m.plainRender(), "┌"+dialogNewRequest) {
		t.Fatal("dialog still drawn after enter")
	}
}

func TestDeleteRequestDialog(t *testing.T) {
	ws := fixtureWS(t)
	m := press(newTestModel(t, ws), "2", "G")
	if m.selectedRowLabel() != "get user" {
		t.Fatalf("selected %q", m.selectedRowLabel())
	}
	row, _ := m.collections.selected(ws.Collections)
	req := row.req
	m = press(m, "d")
	out := m.plainRender()
	dialogLine(t, out, "Delete request")
	for _, want := range []string{`Delete "get user"?`, "yes: y  no: n/esc"} {
		if !strings.Contains(out, want) {
			t.Fatalf("dialog lacks %q:\n%s", want, out)
		}
	}
	if hint := lastLine(out); !strings.Contains(hint, "yes: y") {
		t.Fatalf("hint line = %q", hint)
	}
	m = press(m, "n")
	if m.prompt() != nil || strings.Contains(m.plainRender(), "┌Delete request") {
		t.Fatal("n should close the dialog")
	}
	if _, err := os.Stat(req.Path); err != nil {
		t.Fatalf("n should keep %s: %v", req.Path, err)
	}
}

func TestDialogsStayInBounds(t *testing.T) {
	for _, size := range [][2]int{{80, 20}, {200, 50}, {120, 40}} {
		m := resize(newModel(testDeps(t, fixtureWS(t))), size[0], size[1])
		assertCentered(t, press(m, "2", "n"), dialogNewRequest)
		long := press(m, "2", "n")
		long = typeText(long, strings.Repeat("abcdefghij", 12))
		assertCentered(t, long, dialogNewRequest)
		assertCentered(t, press(m, "2", "G", "d"), "Delete request")
	}
}

func TestSecretValueDialogMasked(t *testing.T) {
	ws := fixtureWS(t)
	m := varsRow(t, press(newTestModel(t, ws), "V"), "token")
	m = typeText(press(m, "enter"), "zq9x")
	out := m.plainRender()
	dialogLine(t, out, "Edit token")
	if strings.Contains(out, "zq9x") || !strings.Contains(out, "│ ****") {
		t.Fatalf("secret value not masked in the dialog:\n%s", out)
	}
	assertPasswordInput(t, m, true)
}

func TestAddVariableSecretDialog(t *testing.T) {
	ws := fixtureWS(t)
	m := press(newTestModel(t, ws), "V", "a")
	dialogLine(t, m.plainRender(), "Add variable")
	m = press(typeText(m, "api_token"), "enter")
	dialogLine(t, m.plainRender(), "Variable value")
	m = press(typeText(m, "x"), "enter")
	out := m.plainRender()
	dialogLine(t, out, "Secret?")
	if !strings.Contains(out, "yes: enter  yes: y  no: n") {
		t.Fatalf("secret dialog footer missing:\n%s", out)
	}
}

func TestResponseSearchStaysAtBottom(t *testing.T) {
	srv := serve(t, &recorder{contentType: "text/plain", body: numberedBody(60, 30)})
	m, _ := sendModel(t, fixtureWS(t), srv.URL)
	m = typeText(press(pressRun(m, "s"), "5", "/"), "nee")
	out := m.plainRender()
	if st := statusLine(out); !strings.HasPrefix(st, " /nee") {
		t.Fatalf("search should render in the status line: %q", st)
	}
	if strings.Contains(out, "confirm: enter") {
		t.Fatalf("search must not open a dialog:\n%s", out)
	}
}
