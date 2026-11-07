package ui

import (
	"fmt"
	"image/color"

	"charm.land/lipgloss/v2"
	"strings"
	"testing"
)

func editBodyTo(t *testing.T, m Model, text string) Model {
	t.Helper()
	return m.setBodyContent(text)
}

func bodyTabLines(m Model) string {
	return strings.Join(strings.Split(press(m, "B").plainRender(), "\n")[:8], "\n")
}

func TestEditedBodyGetsAType(t *testing.T) {
	for _, tc := range []struct{ text, typ string }{{`{"a":1}`, "json"}, {"plain text", "raw"}} {
		m := resize(newTestModel(t, emptyWS(t)), 100, 24)
		m = press(typeText(press(m, "N"), "http://x.test"), "enter")
		m = press(typeText(press(m, "P", "a"), "q: 1"), "enter")
		m = editBodyTo(t, press(m, "B"), tc.text)
		if got := string(m.scratch.Body.Type); got != tc.typ || m.scratch.Body.Content != tc.text {
			t.Fatalf("%q: body = %+v", tc.text, m.scratch.Body)
		}
		if out := bodyTabLines(m); !strings.Contains(out, "Type: "+tc.typ) || !strings.Contains(out, tc.text) {
			t.Fatalf("%q: body tab:\n%s", tc.text, out)
		}
		if len(m.scratch.Params) != 1 || !strings.Contains(strings.Join(strings.Split(press(m, "P").plainRender(), "\n")[:6], "\n"), "q=1") {
			t.Fatalf("%q: params lost: %+v", tc.text, m.scratch.Params)
		}
	}
}

func TestEditedBodyKeepsChosenType(t *testing.T) {
	m := resize(newTestModel(t, emptyWS(t)), 100, 24)
	m = press(typeText(press(m, "N"), "http://x.test"), "enter")
	m = press(m, "B", "t", "t", "t", "t")
	m = editBodyTo(t, m, `{"looks":"json"}`)
	if got := string(m.scratch.Body.Type); got != "raw" {
		t.Fatalf("type = %q, want raw kept", got)
	}
}

func TestBodyTabShowsTypeAndHint(t *testing.T) {
	m := resize(newTestModel(t, emptyWS(t)), 100, 24)
	m = press(typeText(press(m, "N"), "http://x.test"), "enter")
	if out := bodyTabLines(m); !strings.Contains(out, "Type: none") || !strings.Contains(out, "no body · enter: write it, t: type") {
		t.Fatalf("no body:\n%s", out)
	}
	m = press(m, "B", "t")
	if out := bodyTabLines(m); !strings.Contains(out, "Type: json") || !strings.Contains(out, "empty · enter: edit") {
		t.Fatalf("empty json:\n%s", out)
	}
}

func TestEditedBodyWithTemplatesIsJSON(t *testing.T) {
	m := resize(newTestModel(t, emptyWS(t)), 100, 24)
	m = press(typeText(press(m, "N"), "http://x.test"), "enter")
	m = editBodyTo(t, press(m, "B"), `{"id": {{user_id}}, "n": "{{name}}"}`)
	if got := string(m.scratch.Body.Type); got != "json" {
		t.Fatalf("type = %q, want json", got)
	}
}

func TestBodyEditorStylesUse16Colours(t *testing.T) {
	ed := newBodyEditor("json", "{}", 100, 30)
	s := ed.ta.Styles()
	for name, st := range map[string]lipgloss.Style{
		"cursor line": s.Focused.CursorLine, "line number": s.Focused.LineNumber,
		"cursor line number": s.Focused.CursorLineNumber, "prompt": s.Focused.Prompt,
		"end of buffer": s.Focused.EndOfBuffer, "placeholder": s.Focused.Placeholder,
	} {
		for _, c := range []color.Color{st.GetForeground(), st.GetBackground()} {

			if typ := fmt.Sprintf("%T", c); strings.Contains(typ, "Indexed") || strings.Contains(typ, "RGB") {
				t.Errorf("%s uses %s %v", name, typ, c)
			}
		}
	}
}
