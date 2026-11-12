package ui

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/mc-akash/lazypost/internal/history"
	"github.com/mc-akash/lazypost/internal/workspace"
)

func TestHistoryEntryHasNoLiteralCredentials(t *testing.T) {
	srv := serve(t, &recorder{body: []byte("ok")})
	ws := fixtureWS(t)
	req := requestByName(t, folderByName(t, ws, "users"), "get user")
	req.Headers = append(req.Headers,
		workspace.KV{Key: "Proxy-Authorization", Value: "Basic lit-proxy-cred"},
		workspace.KV{Key: "Authorization", Value: "Bearer lit-bearer-abc123"},
		workspace.KV{Key: "X-Token", Value: "{{token}}"},
		workspace.KV{Key: "Cookie", Value: "session=lit-cookie-sess"},
	)
	req.Auth = workspace.Auth{Type: "basic", Username: "bob", Password: "lit-basic-pw-xyz"}
	m, histPath := sendModel(t, ws, srv.URL)
	m = pressRun(m, "s")

	if len(m.history.entries) != 1 {
		t.Fatalf("history entries = %d", len(m.history.entries))
	}
	inMemory, err := json.Marshal(m.history.entries[0])
	if err != nil {
		t.Fatal(err)
	}
	onDisk, err := os.ReadFile(histPath)
	if err != nil {
		t.Fatal(err)
	}
	for name, raw := range map[string][]byte{"entry": inMemory, "disk": onDisk} {
		for _, lit := range []string{"lit-bearer-abc123", "lit-basic-pw-xyz", "lit-cookie-sess", "lit-proxy-cred"} {
			if bytes.Contains(raw, []byte(lit)) {
				t.Errorf("%s leaks %q:\n%s", name, lit, raw)
			}
		}
		if !bytes.Contains(raw, []byte("{{token}}")) {
			t.Errorf("%s lost the {{token}} reference:\n%s", name, raw)
		}
	}
	tmpl := m.history.entries[0].Template
	if tmpl.Headers[len(tmpl.Headers)-3].Value != "Bearer ••••" || tmpl.Auth.Username != "bob" {
		t.Fatalf("template = %+v", tmpl)
	}
}

func TestEnvsFocusKeepsRequestInContext(t *testing.T) {
	rec := &recorder{body: []byte("ok")}
	srv := serve(t, rec)
	m, _ := sendModel(t, fixtureWS(t), srv.URL)
	m = press(m, "1")
	if got := press(m, "4").selectedRequest().Name; got != "get user" {
		t.Fatalf("selected after 1,4 = %q", got)
	}
	m = pressRun(m, "s")
	if rec.hitCount() != 1 || len(m.history.entries) != 1 {
		t.Fatalf("collections source: hits = %d entries = %d", rec.hitCount(), len(m.history.entries))
	}

	m = pressRun(press(m, "3", "1"), "s")
	if rec.hitCount() != 2 || len(m.history.entries) != 2 {
		t.Fatalf("history source: hits = %d entries = %d", rec.hitCount(), len(m.history.entries))
	}
	if _, ok := m.selection(); !ok {
		t.Fatal("no selection after resend from history via [1]")
	}

	if m = press(m, "1", "0"); m.Focus() != PanelVars {
		t.Fatalf("0 from [1]: focus = %v, want vars", m.Focus())
	}
	if m = press(m, "1", "4", "esc"); m.Focus() != PanelEnvs {
		t.Fatalf("esc after 1,4: focus = %v, want envs", m.Focus())
	}
	if m = press(m, "2", "4", "esc"); m.Focus() != PanelCollections {
		t.Fatalf("esc after 2,4: focus = %v, want collections", m.Focus())
	}
}

func TestHistoryResendWithoutPathShowsFullResponse(t *testing.T) {
	big := []byte(strings.Repeat("a", history.MaxStoredBody+5000))
	srv := serve(t, &recorder{contentType: "text/plain", body: big})
	m, _ := sendModel(t, fixtureWS(t), srv.URL)
	m.history = m.history.add(history.Entry{Template: workspace.Request{Name: "adhoc", Method: "GET", URL: srv.URL}}, true, m.historyMax())
	m = pressRun(press(m, "3"), "s")

	sel, ok := m.selection()
	if !ok || sel.entry == nil || sel.entry.RequestPath != "" || sel.entry.Template.Name != "adhoc" {
		t.Fatalf("selection = %+v ok = %v", sel, ok)
	}
	if got := len(m.currentResponse().body); got != len(big) {
		t.Fatalf("response body = %d bytes, want the full %d", got, len(big))
	}
}
