package ui

import (
	"errors"
	"strings"
	"testing"

	"charm.land/bubbles/v2/textinput"

	"github.com/mc-akash/lazypost/internal/workspace"
)

func TestApplyEditedRequestInvalidMethodKeepsDraft(t *testing.T) {
	m, _ := sendModel(t, fixtureWS(t), "http://example.invalid")
	m = press(m, "4", "m", "j", "enter")
	next := m.applyEditedRequest(writeTemp(t, "name: get user\nmethod: \"GET;$'\\\\143'\"\nurl: http://x/y\n"))
	if !strings.HasPrefix(next.Status(), "invalid method") {
		t.Fatalf("status = %q", next.Status())
	}
	if tmpl := currentTmpl(next); tmpl.Method != "POST" || tmpl.URL != "{{base_url}}/users/1" {
		t.Fatalf("draft changed: %+v", tmpl)
	}
}

func TestMethodPickerOnlyValidMethods(t *testing.T) {
	m, _ := sendModel(t, fixtureWS(t), "http://example.invalid")

	m = m.editRequest(func(r *workspace.Request) { r.Method = "bogus method" })
	m = press(m, "4")
	for i := range len(httpMethods) {
		keys := []string{"m"}
		for range httpMethods {
			keys = append(keys, "k")
		}
		for range i {
			keys = append(keys, "j")
		}
		m = press(m, append(keys, "enter")...)
		if got := currentTmpl(m).Method; got != httpMethods[i] || !workspace.ValidMethod(got) {
			t.Fatalf("picker produced %q, want %q", got, httpMethods[i])
		}
	}
}

func assertPasswordInput(t *testing.T, m Model, want bool) {
	t.Helper()
	if m.input() == nil {
		t.Fatal("no input open")
	}
	if got := m.input().ti.EchoMode == textinput.EchoPassword; got != want {
		t.Fatalf("dialog %q: password echo = %v, want %v", m.input().title, got, want)
	}
}

func TestSecretInputsUsePasswordEcho(t *testing.T) {
	ws := fixtureWS(t)
	m := press(newTestModel(t, ws), "V")

	sec := varsRow(t, m, "token")
	assertPasswordInput(t, press(sec, "enter"), true)
	assertPasswordInput(t, press(sec, "r", "enter"), true)

	assertPasswordInput(t, press(varsRow(t, m, "base_url"), "enter"), false)

	add := press(typeText(press(m, "a"), "api_token"), "enter")
	assertPasswordInput(t, add, true)
	if typed := press(typeText(add, "hunter22"), "enter"); strings.Contains(typed.Render(), "hunter22") {
		t.Fatal("added secret value shown")
	}
	assertPasswordInput(t, press(typeText(press(m, "a"), "region"), "enter"), false)

	d := testDeps(t, ws)
	d.Clip = errClip{err: errors.New("no clipboard tool")}
	pm := press(resize(newModel(d), 120, 40), "V")
	assertPasswordInput(t, press(varsRow(t, pm, "token"), "p"), true)
	assertPasswordInput(t, press(varsRow(t, pm, "base_url"), "p"), false)
}

func TestSecretInputHidesTypedValue(t *testing.T) {
	m := press(varsRow(t, press(newTestModel(t, fixtureWS(t)), "V"), "token"), "enter")
	m = typeText(m, "typed-secret")
	if strings.Contains(m.Render(), "typed-secret") {
		t.Fatal("typed secret visible in the input")
	}
}
