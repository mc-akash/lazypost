package ui

import (
	"slices"
	"strings"
	"testing"

	"github.com/mc-akash/lazypost/internal/workspace"
)

const secretCurl = `curl -H 'Authorization: Bearer tok-abc123' -b 'sid=cookieval9' -u alice:pw-xyz789 https://a.io/x`

var importedSecrets = []string{"tok-abc123", "cookieval9", "pw-xyz789"}

func importAs(t *testing.T, m Model, ws *workspace.Workspace, cmd, name string) (Model, *workspace.Request) {
	t.Helper()
	orders := folderByName(t, ws, "orders")
	before := slices.Clone(orders.Requests)
	m = press(typeText(press(m, "i"), cmd), "enter")
	m.input().ti.SetValue(name)
	m = press(m, "enter")
	for _, r := range orders.Requests {
		if !slices.Contains(before, r) {
			return m, r
		}
	}
	t.Fatalf("no request created; status %q", m.Status())
	return m, nil
}

func TestImportCurlMovesSecretsToActiveEnv(t *testing.T) {
	ws := fixtureWS(t)
	m := newTestModel(t, ws)
	m, r := importAs(t, m, ws, secretCurl, "My Req")

	if m.Status() != "created My Req (3 secrets → staging)" {
		t.Fatalf("status = %q", m.Status())
	}
	file := readFile(t, r.Path)
	for _, s := range importedSecrets {
		if strings.Contains(file, s) {
			t.Fatalf("request file holds secret %q:\n%s", s, file)
		}
	}
	for _, want := range []string{"Bearer {{my_req_authorization}}", "value: '{{my_req_cookie}}'", "password: '{{my_req_password}}'", "username: alice"} {
		if !strings.Contains(file, want) {
			t.Fatalf("request file lacks %q:\n%s", want, file)
		}
	}
	if plain := readFile(t, envFile(ws, "staging")); strings.Contains(plain, "tok-abc123") {
		t.Fatalf("plain env file holds a secret:\n%s", plain)
	}
	sec := envByName(t, ws, "staging").Secrets
	want := map[string]string{"my_req_authorization": "tok-abc123", "my_req_cookie": "sid=cookieval9", "my_req_password": "pw-xyz789"}
	for k, v := range want {
		if sec[k] != v {
			t.Errorf("staging secret %s = %q, want %q", k, sec[k], v)
		}
		if !strings.Contains(readFile(t, secretFile(ws, "staging")), v) {
			t.Errorf("secret file lacks %s", k)
		}
	}

	m, r2 := importAs(t, m, ws, `curl -H 'Authorization: Bearer other-tok' https://a.io/y`, "My Req")
	if sec := envByName(t, ws, "staging").Secrets; sec["my_req_authorization"] != "tok-abc123" || sec["my_req_authorization_2"] != "other-tok" {
		t.Fatalf("secrets after second import = %v", sec)
	}
	if file := readFile(t, r2.Path); !strings.Contains(file, "Bearer {{my_req_authorization_2}}") {
		t.Fatalf("second request file:\n%s", file)
	}
	if m.Status() != "created My Req (1 secret → staging)" {
		t.Fatalf("status = %q", m.Status())
	}
}

func TestImportCurlWithoutEnvBecomesDraft(t *testing.T) {
	ws := fixtureWS(t)
	m := newTestModel(t, ws)
	m.envName = ""
	m, r := importAs(t, m, ws, secretCurl, "My Req")

	if m.Status() != "imported as draft: no environment for secrets (w writes)" {
		t.Fatalf("status = %q", m.Status())
	}
	file := readFile(t, r.Path)
	for _, s := range importedSecrets {
		if strings.Contains(file, s) {
			t.Fatalf("request file holds secret %q:\n%s", s, file)
		}
	}
	if strings.Contains(file, "https://a.io/x") {
		t.Fatalf("imported content must stay in the draft, file:\n%s", file)
	}
	d, ok := m.drafts[m.draftKey(r.Path)]
	if !ok || d.Request.URL != "https://a.io/x" || d.Request.Auth.Password != "pw-xyz789" {
		t.Fatalf("draft = %+v (ok %v), want the imported request", d, ok)
	}
	if m.selectedRowLabel() != "My Req" || m.draftMark(r.Path) != markDraft {
		t.Fatalf("selected = %q mark = %q", m.selectedRowLabel(), m.draftMark(r.Path))
	}
}

func TestImportCurlWithoutSecretsSavesDirectly(t *testing.T) {
	ws := fixtureWS(t)
	m := newTestModel(t, ws)
	m.envName = ""
	m, r := importAs(t, m, ws, `curl https://a.io/z -H 'Accept: text/plain'`, "Plain")
	if m.Status() != "created Plain" {
		t.Fatalf("status = %q", m.Status())
	}
	if file := readFile(t, r.Path); !strings.Contains(file, "https://a.io/z") || !strings.Contains(file, "text/plain") {
		t.Fatalf("file:\n%s", file)
	}
}

func TestExtractSecretsEdgeCases(t *testing.T) {
	env := &workspace.Env{Vars: map[string]string{"x_cookie": "plain"}, Secrets: map[string]string{"x_authorization": "same"}}
	in := workspace.Request{Headers: []workspace.KV{
		{Key: "authorization", Value: "same"},
		{Key: "Authorization", Value: "  "},
		{Key: "COOKIE", Value: "a=b"},
		{Key: "X-Other", Value: "keep"},
	}}
	out, secrets := extractSecrets(in, "x", env)
	got := []string{out.Headers[0].Value, out.Headers[1].Value, out.Headers[2].Value, out.Headers[3].Value}
	want := []string{"{{x_authorization}}", "  ", "{{x_cookie_2}}", "keep"}
	if !slices.Equal(got, want) {
		t.Fatalf("headers = %q, want %q", got, want)
	}
	if len(secrets) != 2 || secrets[0] != (importSecret{"x_authorization", "same"}) || secrets[1] != (importSecret{"x_cookie_2", "a=b"}) {
		t.Fatalf("secrets = %+v", secrets)
	}
	if in.Headers[0].Value != "same" {
		t.Fatal("extractSecrets modified its input")
	}
}
