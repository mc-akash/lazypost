package ui

import (
	"slices"
	"strings"
	"testing"

	"github.com/mc-akash/lazypost/internal/workspace"
)

func clipText(m Model) string { return m.deps.Clip.(*fakeClip).text }

func TestCopyAsCurl(t *testing.T) {
	ws := fixtureWS(t)
	bearerFolder(t, ws)
	m, _ := sendModel(t, ws, "http://staging.example")
	for _, keys := range [][]string{{"c"}, {"4", "c"}} {
		m.deps.Clip.(*fakeClip).text = ""
		m = press(m, keys...)
		got := clipText(m)
		if !strings.HasPrefix(got, "curl ") {
			t.Fatalf("%v: clipboard = %q, want a curl command", keys, got)
		}
		for _, want := range []string{"http://staging.example/users/1", "Bearer staging-secret-token"} {
			if !strings.Contains(got, want) {
				t.Fatalf("%v: clipboard %q lacks %q", keys, got, want)
			}
		}
		if m.Status() != "copied as cURL" {
			t.Fatalf("%v: status = %q", keys, m.Status())
		}
	}
}

func TestCopyAsCurlUsesDraft(t *testing.T) {
	m, _ := sendModel(t, fixtureWS(t), "http://staging.example")
	m = press(m, "4", "m", "j", "enter", "c")
	if got := clipText(m); !strings.HasPrefix(got, "curl -X 'POST' ") {
		t.Fatalf("clipboard = %q, want the draft's POST", got)
	}
}

func TestImportCurlCreatesRequest(t *testing.T) {
	ws := fixtureWS(t)
	m := newTestModel(t, ws)
	orders := folderByName(t, ws, "orders")
	before := slices.Clone(orders.Requests)

	m = press(m, "i")
	if ed := m.bodyEd(); ed == nil || ed.title != dialogPasteCurl || ed.ta.Value() != "" {
		t.Fatal("i should open an empty Paste cURL editor")
	}
	if out := m.plainRender(); !strings.Contains(out, "┌"+dialogPasteCurl) || !strings.Contains(out, "import: ctrl+s") || strings.Contains(out, "format:") {
		t.Fatalf("editor should be titled Paste cURL and hint import only:\n%s", out)
	}
	m = press(typeText(m, `curl -X POST https://a.io/x -d '{"a":1}'`), "ctrl+s")
	if m.input() == nil || m.input().title != dialogRequestName || m.input().ti.Value() != "POST /x" {
		t.Fatalf("want Request name dialog prefilled with the imported name, got %+v", m.input())
	}
	m = press(m, "enter")

	var created *workspace.Request
	for _, r := range orders.Requests {
		if !slices.Contains(before, r) {
			created = r
		}
	}
	if created == nil {
		t.Fatalf("no request created; status %q", m.Status())
	}
	file := readFile(t, created.Path)
	for _, want := range []string{"method: POST", "url: https://a.io/x", "type: json", `{"a":1}`} {
		if !strings.Contains(file, want) {
			t.Fatalf("file lacks %q:\n%s", want, file)
		}
	}
	if m.selectedRowLabel() != "POST /x" {
		t.Fatalf("selected = %q, want the new request", m.selectedRowLabel())
	}
}

func TestImportCurlPrefillsFromClipboard(t *testing.T) {
	m := newTestModel(t, fixtureWS(t))
	m.deps.Clip.(*fakeClip).text = "curl https://a.io/y"
	if m = press(m, "i"); m.bodyEd() == nil || m.bodyEd().ta.Value() != "curl https://a.io/y" {
		t.Fatal("editor should be prefilled with a curl clipboard")
	}
	if m = press(typeText(m, " -v"), "esc"); m.overlay != nil {
		t.Fatal("esc should close the editor without asking")
	}
	m.deps.Clip.(*fakeClip).text = "not a command"
	if m = press(m, "i"); m.bodyEd() == nil || m.bodyEd().ta.Value() != "" {
		t.Fatal("editor should be empty when the clipboard is not a curl command")
	}
}

func TestImportCurlError(t *testing.T) {
	ws := fixtureWS(t)
	m := newTestModel(t, ws)
	orders := folderByName(t, ws, "orders")
	n := len(orders.Requests)
	m = press(typeText(press(m, "i"), "curl --proxy x"), "ctrl+s")
	if !strings.Contains(m.Status(), "curl:") || m.prompt() == nil || m.prompt().title != dialogError {
		t.Fatalf("status = %q, prompt = %v", m.Status(), m.prompt())
	}
	if m = press(m, "x"); m.bodyEd() == nil || m.bodyEd().ta.Value() != "curl --proxy x" {
		t.Fatal("a failed import should keep the editor and its text")
	}
	if len(orders.Requests) != n {
		t.Fatal("a failed import must not create a request")
	}
}

func TestTreeShowsDraftMethod(t *testing.T) {
	m, _ := sendModel(t, fixtureWS(t), "http://example.invalid")
	m = press(m, "4", "m", "j", "enter")
	if out := m.plainRender(); !strings.Contains(out, "POST get user ●") {
		t.Fatalf("tree should show the draft's method:\n%s", out)
	}
}

func TestEnterOnFormBodyDoesNotOpenEditor(t *testing.T) {
	m, _ := sendModel(t, fixtureWS(t), "http://example.invalid")
	for _, typ := range []string{"form", "multipart"} {
		m = press(m, "4", "]")
		for currentTmpl(m).Body.Type != workspace.BodyType(typ) {
			m = press(m, "t")
		}
		next, cmd := m.Update(keyMsg("enter"))
		m = next.(Model)
		if cmd != nil {
			t.Fatalf("%s: enter should not open $EDITOR", typ)
		}
		if m.Status() != "edit form fields with e (full request in $EDITOR)" {
			t.Fatalf("%s: status = %q", typ, m.Status())
		}
		m = press(m, "[")
	}
}
