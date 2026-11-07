package ui

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mc-akash/lazypost/internal/workspace"
)

func TestErrorOpensDialogAndAnyKeyCloses(t *testing.T) {
	m := newTestModel(t, fixtureWS(t))
	long := "save failed: " + strings.Repeat("very long reason ", 10) + "END"
	m = m.fail(long)
	p := m.prompt()
	if p == nil || p.title != dialogError {
		t.Fatalf("no error dialog: %v", m.overlay)
	}
	out := m.plainRender()
	if !strings.Contains(out, "Error") || !strings.Contains(out, "END") || !strings.Contains(out, "close: any key") {
		t.Fatalf("dialog should show the whole wrapped message:\n%s", out)
	}
	if m.Status() != long {
		t.Fatalf("status = %q", m.Status())
	}
	if m = press(m, "x"); m.overlay != nil {
		t.Fatal("any key should close the error dialog")
	}
}

func TestErrorsStackInOneDialog(t *testing.T) {
	m := newTestModel(t, fixtureWS(t)).fail("first").fail("second")
	if p := m.prompt(); p == nil || p.message != "first\nsecond" {
		t.Fatalf("prompt = %+v", p)
	}
}

func TestErrorInBodyEditorKeepsEditor(t *testing.T) {
	m := press(scratchBodyModel(t), "enter")
	m = press(typeText(m, `{"a":`), "ctrl+f")
	ed := m.bodyEd()
	if ed == nil || ed.confirm == nil || ed.confirm.title != dialogError {
		t.Fatalf("error should open over the editor: %v", m.overlay)
	}
	m = press(m, "esc")
	if ed := m.bodyEd(); ed == nil || ed.confirm != nil || edText(t, m) != `{"a":` {
		t.Fatalf("closing the error should keep the editor and its text: %v", m.overlay)
	}
}

func TestStartupErrorsOpenDialog(t *testing.T) {
	ws := fixtureWS(t)
	bad := filepath.Join(ws.Root, "collections", "users", "broken.yaml")
	ws.LoadErrors = []workspace.FileError{{Path: bad, Err: errors.New("yaml: bad")}}
	d := testDeps(t, ws)
	d.StartupStatus = "session state unreadable: x (starting fresh)"
	p := newModel(d).prompt()
	if p == nil || p.title != dialogError {
		t.Fatal("startup errors should open the error dialog")
	}
	if !strings.Contains(p.message, "broken.yaml: yaml: bad") || !strings.Contains(p.message, "session state unreadable") {
		t.Fatalf("message = %q", p.message)
	}
	if newTestModel(t, fixtureWS(t)).overlay != nil {
		t.Fatal("no dialog without startup errors")
	}
}

type failClip struct{}

func (failClip) Read() (string, error) { return "", errors.New("no clipboard tool") }
func (failClip) Write(string) error    { return errors.New("no clipboard tool") }

func TestErrorHidesQuotedValues(t *testing.T) {
	m := newTestModel(t, fixtureWS(t)).fail("invalid YAML: cannot unmarshal !!str `Bearer sk-live-123` into []KV")
	if strings.Contains(m.prompt().message, "sk-live") || strings.Contains(m.Status(), "sk-live") {
		t.Fatalf("quoted value shown: %q / %q", m.prompt().message, m.Status())
	}
}

func TestOSC52OnlyCopyIsNotAnError(t *testing.T) {
	srv := serve(t, &recorder{contentType: "text/plain", body: []byte("hello body")})
	m, _ := sendModel(t, fixtureWS(t), srv.URL)
	m.deps.Clip = failClip{}
	m = press(pressRun(m, "s"), "5", "y")
	if m.overlay != nil || !strings.Contains(m.Status(), "OSC 52 only") {
		t.Fatalf("overlay = %v, status = %q", m.overlay, m.Status())
	}
}
