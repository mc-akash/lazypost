package ui

import (
	"slices"
	"strings"
	"testing"

	"github.com/mc-akash/lazypost/internal/workspace"
)

func lastHeader(t *testing.T, m Model) workspace.KV {
	t.Helper()
	hs := currentTmpl(m).Headers
	if len(hs) == 0 {
		t.Fatal("no headers")
	}
	return hs[len(hs)-1]
}

func TestHeaderPickerAcceptJSON(t *testing.T) {
	m := press(requestModel(t), "a")
	if m.picker() == nil || m.picker().title != dialogAddHeader {
		t.Fatal("a on Headers should open the header picker")
	}
	if !strings.Contains(m.plainRender(), dialogAddHeader) {
		t.Fatal("picker title not drawn")
	}
	m = press(typeText(m, "acc"), "enter")
	if m.picker() == nil || !slices.Contains(m.picker().items, "application/json") {
		t.Fatal("Accept should offer value suggestions")
	}
	if slices.Contains(m.picker().items, "custom…") {
		t.Fatalf("value picker should have no custom entry: %v", m.picker().items)
	}
	m = press(m, "enter")
	if m.picker() != nil || m.input() != nil {
		t.Fatal("picking a value should finish")
	}
	if got := lastHeader(t, m); got != (workspace.KV{Key: "Accept", Value: "application/json"}) {
		t.Fatalf("header = %+v", got)
	}
	if m.reqRow() != len(currentTmpl(m).Headers)-1 {
		t.Fatalf("new row not selected: row %d", m.reqRow())
	}
	if !strings.Contains(m.plainRender(), "[4] Request ●") {
		t.Fatal("draft marker missing")
	}
}

func TestHeaderPickerWorkspaceHeadersFirst(t *testing.T) {
	ws := fixtureWS(t)
	users := folderByName(t, ws, "users")
	get := requestByName(t, users, "get user")
	get.Headers = []workspace.KV{{Key: "x-tenant", Value: "a"}, {Key: "accept", Value: "*/*"}}
	create := requestByName(t, users, "create user")

	create.Headers = []workspace.KV{{Key: "X-Tenant", Value: "b", Disabled: true}, {Key: "Zeta", Value: "1"}}
	for _, r := range []*workspace.Request{get, create} {
		if err := ws.SaveRequest(r); err != nil {
			t.Fatal(err)
		}
	}
	m, _ := sendModel(t, ws, "http://example.invalid")
	m = press(m, "4", "a")
	items := m.picker().items

	if got := strings.Join(items[:3], ","); got != "X-Tenant,accept,Zeta" {
		t.Fatalf("workspace headers = %q", got)
	}
	n := 0
	for _, it := range items {
		if strings.EqualFold(it, "accept") {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("Accept listed %d times: %v", n, items)
	}
	if items[3] != "Accept-Encoding" || items[len(items)-1] != builtinHeaders[len(builtinHeaders)-1] {
		t.Fatalf("built-ins misplaced: %v", items)
	}
}

func TestHeaderNameItemsBuiltins(t *testing.T) {
	items := headerNameItems(nil)
	if !slices.Equal(items, builtinHeaders) {
		t.Fatalf("items = %v", items)
	}
}

func TestHeaderPickerOpensWithEmptyInput(t *testing.T) {
	m := press(requestModel(t), "a")
	if m.picker().filter != "" || len(m.picker().matches()) != len(m.picker().items) {
		t.Fatalf("filter %q, %d of %d items", m.picker().filter, len(m.picker().matches()), len(m.picker().items))
	}
	if !strings.Contains(m.plainRender(), pickerPrompt) {
		t.Fatal("input line not drawn")
	}
	if got := hintLine(m.hints()); !strings.Contains(got, "filter: type") || !strings.Contains(got, "pick (or use typed): enter") || strings.Contains(got, "filter: /") {
		t.Fatalf("hints = %q", got)
	}
}

func TestHeaderPickerTypingFilters(t *testing.T) {
	m := typeText(press(requestModel(t), "a"), "content")
	if m.picker().filter != "content" || m.picker().filtering {
		t.Fatalf("filter %q filtering %v", m.picker().filter, m.picker().filtering)
	}
	if got := m.picker().matches(); len(got) != 1 || got[0] != "Content-Type" {
		t.Fatalf("matches = %v", got)
	}
	if out := m.plainRender(); !strings.Contains(out, "> content") || strings.Contains(out, "Authorization") {
		t.Fatalf("render should show the input and only the matches:\n%s", out)
	}
	m = press(m, "backspace", "backspace", "backspace", "backspace", "backspace", "backspace", "backspace")
	if m.picker().filter != "" || len(m.picker().matches()) != len(m.picker().items) {
		t.Fatalf("backspace should clear the filter, got %q", m.picker().filter)
	}
}

func TestHeaderPickerJKAndGType(t *testing.T) {
	m := typeText(press(requestModel(t), "a"), "jkgG/")
	if m.picker() == nil || m.picker().filter != "jkgG/" || m.picker().cursor != 0 {
		t.Fatalf("j/k/g/G// should type into the filter, picker = %+v", m.picker())
	}
}

func TestHeaderPickerArrowsMove(t *testing.T) {
	m := press(requestModel(t), "a")
	want := m.picker().matches()[2]
	m = press(m, "down", "down", "up")
	if m.picker().cursor != 1 {
		t.Fatalf("cursor = %d, want 1", m.picker().cursor)
	}
	m = press(m, "ctrl+n", "ctrl+n", "ctrl+p")
	if m.picker().cursor != 2 {
		t.Fatalf("cursor = %d, want 2", m.picker().cursor)
	}
	m = press(m, "enter")
	title := want + " value"
	if (m.picker() == nil || m.picker().title != title) && (m.input() == nil || m.input().title != title) {
		t.Fatalf("enter should pick %q, picker %+v input %+v", want, m.picker(), m.input())
	}
}

func TestHeaderPickerEnterPicksHighlightedMatch(t *testing.T) {
	m := press(typeText(press(requestModel(t), "a"), "auth"), "enter")
	if m.picker() != nil || m.input() == nil || m.input().title != "Authorization value" {
		t.Fatalf("enter should pick Authorization, input = %+v", m.input())
	}
	m = press(typeText(m, "Bearer x"), "enter")
	if got := lastHeader(t, m); got != (workspace.KV{Key: "Authorization", Value: "Bearer x"}) {
		t.Fatalf("header = %+v", got)
	}
}

func TestHeaderPickerNoMatchUsesTypedName(t *testing.T) {
	m := press(typeText(press(requestModel(t), "a"), " X-Foo "), "enter")
	if m.picker() != nil || m.input() == nil || m.input().title != "X-Foo value" || m.input().ti.Value() != "" {
		t.Fatalf("no match should take the typed name, input = %+v", m.input())
	}
	m = press(typeText(m, "bar"), "enter")
	if got := lastHeader(t, m); got != (workspace.KV{Key: "X-Foo", Value: "bar"}) {
		t.Fatalf("header = %+v", got)
	}
}

func TestHeaderPickerBlankNoMatchDoesNothing(t *testing.T) {
	m := press(typeText(press(requestModel(t), "a"), "   "), "enter")
	if m.picker() == nil || m.input() != nil || len(m.drafts) != 0 {
		t.Fatalf("blank input with no match should do nothing, picker %v input %v", m.picker(), m.input())
	}
}

func TestHeaderPickerRejectsInvalidTypedName(t *testing.T) {
	for _, name := range []string{"X:Foo", "X Foo"} {
		m := press(typeText(press(requestModel(t), "a"), name), "enter")
		if m.input() != nil || len(m.drafts) != 0 || m.Status() != statusBadHeaderName {
			t.Fatalf("%q: input %v drafts %v status %q", name, m.input(), m.drafts, m.Status())
		}
		if p := m.picker(); p == nil || p.title != dialogAddHeader || p.filter != name {
			t.Fatalf("%q: picker should stay open with the typed text, got %+v", name, p)
		}
	}
}

func TestHeaderPickerNameWithoutSuggestions(t *testing.T) {
	m := press(typeText(press(requestModel(t), "a"), "x-request"), "enter")
	if m.picker() != nil || m.input() == nil || m.input().title != "X-Request-ID value" || m.input().ti.Value() != "" {
		t.Fatal("a name without suggestions should open an empty value input")
	}
	m = press(typeText(m, "abc"), "enter")
	if got := lastHeader(t, m); got != (workspace.KV{Key: "X-Request-ID", Value: "abc"}) {
		t.Fatalf("header = %+v", got)
	}
}

func TestHeaderValuePickerFilters(t *testing.T) {
	m := press(typeText(press(requestModel(t), "a"), "content-t"), "enter")
	if m.picker() == nil || m.picker().title != "Content-Type value" || m.picker().mode != typeInput {
		t.Fatalf("value picker = %+v", m.picker())
	}
	m = press(typeText(m, "multi"), "enter")
	if got := lastHeader(t, m); got != (workspace.KV{Key: "Content-Type", Value: "multipart/form-data"}) {
		t.Fatalf("header = %+v", got)
	}
}

func TestHeaderValuePickerUsesTypedText(t *testing.T) {
	m := press(typeText(press(requestModel(t), "a"), "content-t"), "enter")
	m = press(typeText(m, "text/csv "), "enter")
	if m.picker() != nil || m.input() != nil {
		t.Fatal("typed value with no match should finish")
	}
	if got := lastHeader(t, m); got != (workspace.KV{Key: "Content-Type", Value: "text/csv"}) {
		t.Fatalf("header = %+v", got)
	}
}

func TestHeaderPickerEscCancelsEveryStep(t *testing.T) {
	steps := map[string][]string{
		"name picker":        {"a", "esc"},
		"name picker typed":  {"a", "x", "y", "esc"},
		"value picker":       {"a", "a", "c", "c", "enter", "esc"},
		"value picker typed": {"a", "a", "c", "c", "enter", "z", "esc"},
		"value input":        {"a", "o", "r", "i", "g", "enter", "esc"},
		"custom value input": {"a", "x", "-", "q", "enter", "esc"},
	}
	for name, keys := range steps {
		m := press(requestModel(t), keys...)
		if m.picker() != nil || m.input() != nil || len(m.drafts) != 0 {
			t.Fatalf("%s: esc left picker %v input %v drafts %v", name, m.picker(), m.input(), m.drafts)
		}
	}
}

func TestParamsAddStillUsesInput(t *testing.T) {
	m := press(requestModel(t), "[", "a")
	if m.picker() != nil || m.input() == nil || m.input().title != dialogAddParam {
		t.Fatal("a on Params should open the Add param dialog")
	}
}

func TestHeaderPickerWithNothingSelectedStartsScratch(t *testing.T) {
	m := press(newTestModel(t, fixtureWS(t)), "3", "4", "a")
	if m.picker() == nil || !m.scratchSelected() {
		t.Fatalf("picker %v scratch %v status %q", m.picker(), m.scratchSelected(), m.Status())
	}
}
