package ui

import (
	"strings"
	"testing"

	"github.com/mc-akash/lazypost/internal/workspace"
)

func TestEnterOnEmptyHeadersOpensPicker(t *testing.T) {
	m := press(requestModel(t), "H", "enter")
	if m.picker() == nil || m.picker().title != dialogAddHeader {
		t.Fatalf("enter on no headers should open the header picker, picker = %+v", m.picker())
	}
	m = press(typeText(m, "X-Id"), "enter")
	m = press(typeText(m, "7"), "enter")
	if got := currentTmpl(m).Headers; len(got) != 1 || got[0] != (workspace.KV{Key: "X-Id", Value: "7"}) {
		t.Fatalf("headers = %+v", got)
	}
}

func TestEnterOnEmptyParamsOpensAddParam(t *testing.T) {
	m := press(requestModel(t), "P", "enter")
	if m.input() == nil || m.input().title != dialogAddParam {
		t.Fatalf("enter on no params should open %q, input = %+v", dialogAddParam, m.input())
	}
	m = press(typeText(m, "page: 2"), "enter")
	if got := currentTmpl(m).Params; len(got) != 1 || got[0] != (workspace.KV{Key: "page", Value: "2"}) {
		t.Fatalf("params = %+v", got)
	}

	m = press(m, "enter")
	if m.input() == nil || m.input().title != dialogEditParam || m.input().ti.Value() != "page: 2" {
		t.Fatalf("enter on a param row should edit it, input = %+v", m.input())
	}
}

func TestEnterOnHeaderRowEdits(t *testing.T) {
	ws, _ := acceptHeaderWS(t)
	m, _ := sendModel(t, ws, "http://example.invalid")
	m = press(m, "4", "enter")
	if m.picker() != nil || m.input() == nil || m.input().title != dialogEditHeader {
		t.Fatalf("enter on a header row should edit it: picker %v input %+v", m.picker(), m.input())
	}
}

func TestEmptyTabHints(t *testing.T) {
	m := requestModel(t)
	if out := press(m, "H").plainRender(); !strings.Contains(out, "no headers · enter: add") {
		t.Fatalf("headers tab:\n%s", out)
	}
	if out := press(m, "P").plainRender(); !strings.Contains(out, "no params · enter: add") {
		t.Fatalf("params tab:\n%s", out)
	}
}

func TestAuthTypePicker(t *testing.T) {
	m := press(requestModel(t), "A", "enter")
	if m.picker() == nil || m.picker().title != authPickerTitle {
		t.Fatalf("enter on the type row should open the auth picker, picker = %+v", m.picker())
	}
	out := m.plainRender()
	for _, want := range []string{"● inherit", "none", "bearer", "basic", "apikey"} {
		if !strings.Contains(out, want) {
			t.Fatalf("picker lacks %q:\n%s", want, out)
		}
	}
	if hint := lastLine(out); !strings.Contains(hint, "set auth type: enter") {
		t.Fatalf("hint %q", hint)
	}
	m = press(typeText(m, "bas"), "enter")
	if m.picker() != nil || currentTmpl(m).Auth.Type != "basic" {
		t.Fatalf("picker %v, auth type %q, want basic", m.picker(), currentTmpl(m).Auth.Type)
	}
	if !strings.Contains(m.plainRender(), "[4] Request ●") {
		t.Fatal("picking a type should create a draft")
	}
	if out := press(m, "enter").plainRender(); !strings.Contains(out, "● basic") {
		t.Fatalf("current type not marked:\n%s", out)
	}
	m = press(m, "enter", "j", "esc")
	if m.picker() != nil || currentTmpl(m).Auth.Type != "basic" {
		t.Fatalf("esc should leave the type: picker %v, type %q", m.picker(), currentTmpl(m).Auth.Type)
	}
}

func TestAuthTypePickerNoChangeNoDraft(t *testing.T) {
	m := press(requestModel(t), "A", "enter", "enter")
	if m.picker() != nil || currentTmpl(m).Auth.Type != "" || len(m.drafts) != 0 {
		t.Fatalf("enter on the current type: type %q drafts %v", currentTmpl(m).Auth.Type, m.drafts)
	}
	m = press(m, "enter", "j", "esc")
	if m.picker() != nil || len(m.drafts) != 0 {
		t.Fatalf("esc: picker %v drafts %v", m.picker(), m.drafts)
	}
}
