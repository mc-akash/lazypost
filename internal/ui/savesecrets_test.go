package ui

import (
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/mc-akash/lazypost/internal/workspace"
)

func addHeader(t *testing.T, m Model, kv string) Model {
	t.Helper()
	name, value, _ := strings.Cut(kv, ":")
	m = press(typeText(press(m, "a"), strings.TrimSpace(name)), "enter")
	value = strings.TrimSpace(value)
	if m.picker() != nil {
		return press(typeText(m, value), "enter")
	}
	return submitInput(t, m, value)
}

func assertSecretMoved(t *testing.T, ws *workspace.Workspace, env, name, value, path string) {
	t.Helper()
	file := readFile(t, path)
	if strings.Contains(file, value) {
		t.Fatalf("tracked request file holds the credential:\n%s", file)
	}
	if !strings.Contains(file, "{{"+name+"}}") {
		t.Fatalf("request file lacks {{%s}}:\n%s", name, file)
	}
	if got := envByName(t, ws, env).Secrets[name]; got != value {
		t.Fatalf("%s secret %s = %q, want %q", env, name, got, value)
	}
	if !strings.Contains(readFile(t, secretFile(ws, env)), value) {
		t.Fatalf("secret file lacks %s", name)
	}
	if strings.Contains(readFile(t, envFile(ws, env)), value) {
		t.Fatal("plain env file holds the credential")
	}
}

func TestSaveScratchMovesCredentialsToEnv(t *testing.T) {
	ws := fixtureWS(t)
	m := newScratch(t, newTestModel(t, ws), "{{base_url}}/charges")
	m = addHeader(t, m, "Authorization: Bearer tok-live-123")
	m = submitInput(t, press(m, "w"), "billing/charge")

	path := filepath.Join(ws.Root, "collections", "billing", "charge.yaml")
	assertSecretMoved(t, ws, "staging", "charge_authorization", "tok-live-123", path)
	if !strings.Contains(readFile(t, path), "Bearer {{charge_authorization}}") {
		t.Fatalf("file:\n%s", readFile(t, path))
	}
	if m.Status() != "saved as billing/charge (1 secret → staging)" {
		t.Fatalf("status = %q", m.Status())
	}
	if m.scratch != nil {
		t.Fatal("scratch should be gone after saving")
	}
}

func TestSaveScratchRefusedWithoutEnv(t *testing.T) {
	ws := fixtureWS(t)
	before := requestFiles(t, ws)
	m := newTestModel(t, ws)
	m.envName = ""
	m = newScratch(t, m, "{{base_url}}/charges")
	m = addHeader(t, m, "Authorization: Bearer tok-live-123")
	m = submitInput(t, press(m, "W"), "billing/charge")

	if m.Status() != statusSaveNeedsEnv() {
		t.Fatalf("status = %q", m.Status())
	}
	if after := requestFiles(t, ws); !slices.Equal(before, after) {
		t.Fatalf("files written: %v", after)
	}
	if _, err := os.Stat(filepath.Join(ws.Root, "collections", "billing")); !os.IsNotExist(err) {
		t.Fatalf("folder created on a refused save: %v", err)
	}
	if s := m.scratch; s == nil || !slices.Contains(s.Headers, workspace.KV{Key: "Authorization", Value: "Bearer tok-live-123"}) || m.requestTitle() != unsavedTitle {
		t.Fatalf("scratch changed: %+v", s)
	}
}

func TestWriteDraftMovesCredentialsToEnv(t *testing.T) {
	ws := fixtureWS(t)
	m, _ := sendModel(t, ws, "http://example.invalid")
	m = addHeader(t, press(m, "4"), "Authorization: Bearer tok-draft-9")
	m = press(m, "w")

	path := requestByName(t, folderByName(t, ws, "users"), "get user").Path
	assertSecretMoved(t, ws, "staging", "get_user_authorization", "tok-draft-9", path)
	if m.Status() != "saved (1 secret → staging)" {
		t.Fatalf("status = %q", m.Status())
	}
	if len(m.drafts) != 0 {
		t.Fatalf("drafts = %v, want none", m.drafts)
	}
}

func TestWriteDraftRefusedWithoutEnv(t *testing.T) {
	ws := fixtureWS(t)
	m, _ := sendModel(t, ws, "http://example.invalid")
	m.envName = ""
	path := requestByName(t, folderByName(t, ws, "users"), "get user").Path
	before := readFile(t, path)
	m = addHeader(t, press(m, "4"), "Authorization: Bearer tok-draft-9")
	m = press(m, "w")

	if m.Status() != statusSaveNeedsEnv() {
		t.Fatalf("status = %q", m.Status())
	}
	if readFile(t, path) != before {
		t.Fatal("request file written on a refused save")
	}
	if !slices.Contains(currentTmpl(m).Headers, workspace.KV{Key: "Authorization", Value: "Bearer tok-draft-9"}) || m.draftMark(path) != markDraft {
		t.Fatalf("draft lost: %+v", currentTmpl(m).Headers)
	}
}

func TestSaveAsDraftCredentials(t *testing.T) {
	t.Run("env", func(t *testing.T) {
		ws := fixtureWS(t)
		m, _ := sendModel(t, ws, "http://example.invalid")
		m = addHeader(t, press(m, "4"), "Cookie: sid=cookie-val-7")
		m = submitInput(t, press(m, "W"), "orders/copy")
		path := filepath.Join(ws.Root, "collections", "orders", "copy.yaml")
		assertSecretMoved(t, ws, "staging", "copy_cookie", "sid=cookie-val-7", path)
		if m.Status() != "saved as orders/copy (1 secret → staging)" {
			t.Fatalf("status = %q", m.Status())
		}
	})
	t.Run("no env", func(t *testing.T) {
		ws := fixtureWS(t)
		before := requestFiles(t, ws)
		m, _ := sendModel(t, ws, "http://example.invalid")
		m.envName = ""
		m = addHeader(t, press(m, "4"), "Cookie: sid=cookie-val-7")
		m = submitInput(t, press(m, "W"), "orders/copy")
		if m.Status() != statusSaveNeedsEnv() {
			t.Fatalf("status = %q", m.Status())
		}
		if after := requestFiles(t, ws); !slices.Equal(before, after) {
			t.Fatalf("files written: %v", after)
		}
		if len(m.drafts) != 1 {
			t.Fatalf("draft lost: %v", m.drafts)
		}
	})
}

func TestSaveWithoutCredentialsUnchanged(t *testing.T) {
	ws := fixtureWS(t)
	m, _ := sendModel(t, ws, "http://example.invalid")
	m.envName = ""
	secrets := maps.Clone(envByName(t, ws, "staging").Secrets)
	m = press(m, "4")
	for _, kv := range []string{"Authorization: Bearer {{token}}", "Cookie: sid={{sid}}; theme={{theme}}", "Accept: text/plain"} {
		m = addHeader(t, m, kv)
	}
	m = m.editRequest(func(r *workspace.Request) {
		r.Auth = workspace.Auth{Type: "basic", Username: "alice", Password: "{{pw}}"}
	})
	m = press(m, "w")

	if m.Status() != statusSaved {
		t.Fatalf("status = %q", m.Status())
	}
	file := readFile(t, requestByName(t, folderByName(t, ws, "users"), "get user").Path)
	for _, want := range []string{"Bearer {{token}}", "sid={{sid}}; theme={{theme}}", "text/plain", "password: '{{pw}}'"} {
		if !strings.Contains(file, want) {
			t.Fatalf("file lacks %q:\n%s", want, file)
		}
	}
	if got := envByName(t, ws, "staging").Secrets; !maps.Equal(got, secrets) {
		t.Fatalf("secrets changed: %v", got)
	}

	m = newScratch(t, m, "http://plain.example/x")
	m = submitInput(t, press(m, "w"), "plain")
	if m.Status() != "saved as plain" || m.scratch != nil {
		t.Fatalf("status = %q, scratch %+v", m.Status(), m.scratch)
	}
}

func TestExtractSecretsSkipsTemplates(t *testing.T) {
	in := workspace.Request{
		Headers: []workspace.KV{
			{Key: "Authorization", Value: "Bearer {{token}}"},
			{Key: "Authorization", Value: "{{auth}}"},
			{Key: "Cookie", Value: "a={{x}}; b={{ y }}"},
		},
		Auth: workspace.Auth{Type: "basic", Username: "u", Password: "{{pw}}"},
	}
	if out, secrets := extractSecrets(in, "x", &workspace.Env{}); len(secrets) != 0 || !slices.Equal(out.Headers, in.Headers) || out.Auth != in.Auth {
		t.Fatalf("templates moved: %+v %+v", secrets, out)
	}
	mixed := workspace.Request{Headers: []workspace.KV{{Key: "Cookie", Value: "a={{x}}; sid=lit-1"}}}
	if _, secrets := extractSecrets(mixed, "x", &workspace.Env{}); len(secrets) != 1 || secrets[0].value != "a={{x}}; sid=lit-1" {
		t.Fatalf("a cookie with a literal value must move: %+v", secrets)
	}
}

func draftWithAuth(t *testing.T, ws *workspace.Workspace, auth workspace.Auth) Model {
	t.Helper()
	m, _ := sendModel(t, ws, "http://example.invalid")
	m = press(m, "4")
	return m.editRequest(func(r *workspace.Request) { r.Auth = auth })
}

func TestSaveMovesAuthTabCredentials(t *testing.T) {
	cases := []struct {
		name        string
		auth        workspace.Auth
		secret, val string
		ref         string
	}{
		{"bearer", workspace.Auth{Type: "bearer", Token: "tok-bear-1"}, "get_user_token", "tok-bear-1", "token: '{{get_user_token}}'"},
		{"apikey", workspace.Auth{Type: "apikey", Key: "X-Api-Key", Value: "key-val-2"}, "get_user_apikey", "key-val-2", "value: '{{get_user_apikey}}'"},
		{"basic", workspace.Auth{Type: "basic", Username: "alice", Password: "pw-basic-3"}, "get_user_password", "pw-basic-3", "password: '{{get_user_password}}'"},

		{"stale token", workspace.Auth{Type: "basic", Username: "alice", Password: "{{pw}}", Token: "stale-tok-4"}, "get_user_token", "stale-tok-4", "token: '{{get_user_token}}'"},
	}
	for _, c := range cases {
		t.Run(c.name+"/env", func(t *testing.T) {
			ws := fixtureWS(t)
			m := press(draftWithAuth(t, ws, c.auth), "w")
			path := requestByName(t, folderByName(t, ws, "users"), "get user").Path
			assertSecretMoved(t, ws, "staging", c.secret, c.val, path)
			if file := readFile(t, path); !strings.Contains(file, c.ref) {
				t.Fatalf("file lacks %q:\n%s", c.ref, file)
			}
			if m.Status() != "saved (1 secret → staging)" {
				t.Fatalf("status = %q", m.Status())
			}
		})
		t.Run(c.name+"/no env", func(t *testing.T) {
			ws := fixtureWS(t)
			path := requestByName(t, folderByName(t, ws, "users"), "get user").Path
			before := readFile(t, path)
			m := draftWithAuth(t, ws, c.auth)
			m.envName = ""
			m = press(m, "w")
			if m.Status() != statusSaveNeedsEnv() {
				t.Fatalf("status = %q", m.Status())
			}
			if readFile(t, path) != before {
				t.Fatal("request file written on a refused save")
			}
			if got := currentTmpl(m).Auth; got != c.auth {
				t.Fatalf("draft auth = %+v, want %+v", got, c.auth)
			}
		})
	}
}

func TestSaveScratchMovesAuthToken(t *testing.T) {
	ws := fixtureWS(t)
	m := newScratch(t, newTestModel(t, ws), "{{base_url}}/x")
	m = m.editRequest(func(r *workspace.Request) { r.Auth = workspace.Auth{Type: "bearer", Token: "tok-scr-5"} })
	m = submitInput(t, press(m, "W"), "api/thing")
	path := filepath.Join(ws.Root, "collections", "api", "thing.yaml")
	assertSecretMoved(t, ws, "staging", "thing_token", "tok-scr-5", path)
	if m.scratch != nil || m.Status() != "saved as api/thing (1 secret → staging)" {
		t.Fatalf("status = %q, scratch %+v", m.Status(), m.scratch)
	}
}

func TestSaveAuthTemplatesNeedNoEnv(t *testing.T) {
	for _, auth := range []workspace.Auth{
		{Type: "bearer", Token: "{{token}}"},
		{Type: "apikey", Key: "X-Api-Key", Value: "{{ api_key }}", In: "query"},
	} {
		ws := fixtureWS(t)
		m := draftWithAuth(t, ws, auth)
		m.envName = ""
		if m = press(m, "w"); m.Status() != statusSaved {
			t.Fatalf("%+v: status = %q", auth, m.Status())
		}
	}
}

func TestAuthSecretNamesDoNotClash(t *testing.T) {
	env := &workspace.Env{
		Vars:    map[string]string{"x_token": "a plain var"},
		Secrets: map[string]string{"x_apikey": "other"},
	}
	in := workspace.Request{
		Headers: []workspace.KV{{Key: "Authorization", Value: "Bearer h-1"}, {Key: "Cookie", Value: "c=2"}},
		Auth:    workspace.Auth{Type: "apikey", Key: "k", Value: "v-3", Token: "t-4", Password: "p-5"},
	}
	out, secrets := extractSecrets(in, "x", env)
	got := map[string]string{}
	for _, s := range secrets {
		got[s.name] = s.value
	}
	want := map[string]string{
		"x_authorization": "h-1", "x_cookie": "c=2", "x_password": "p-5",
		"x_token_2": "t-4", "x_apikey_2": "v-3",
	}
	if !maps.Equal(got, want) {
		t.Fatalf("secrets = %v, want %v", got, want)
	}
	if out.Auth.Token != "{{x_token_2}}" || out.Auth.Value != "{{x_apikey_2}}" || out.Auth.Password != "{{x_password}}" {
		t.Fatalf("auth = %+v", out.Auth)
	}
}

func TestSaveAsSecretsCommittedOnlyAfterRequestFile(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	ws := fixtureWS(t)
	m := newScratch(t, newTestModel(t, ws), "{{base_url}}/charges")
	m = addHeader(t, m, "Authorization: Bearer tok-live-123")
	users := filepath.Join(ws.Root, "collections", "users")
	if err := os.Chmod(users, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(users, 0o755) })
	m = submitInput(t, press(m, "W"), "users/charge")
	if !strings.HasPrefix(m.Status(), "save as failed: ") {
		t.Fatalf("status = %q", m.Status())
	}
	if _, ok := envByName(t, ws, "staging").Secrets["charge_authorization"]; ok {
		t.Fatal("secret committed although the request file was not created")
	}
	if strings.Contains(readFile(t, secretFile(ws, "staging")), "tok-live-123") {
		t.Fatal("secret file written although the request file was not created")
	}
}

func TestImportRemovesRequestFileWhenSecretsFail(t *testing.T) {
	ws := fixtureWS(t)
	before := requestFiles(t, ws)
	m := newTestModel(t, ws)
	ws.LoadErrors = append(ws.LoadErrors, workspace.FileError{Path: secretFile(ws, "staging")})
	m = m.createImported("charge", workspace.Request{Method: "POST", URL: "http://x/charge",
		Headers: []workspace.KV{{Key: "Authorization", Value: "Bearer tok-live-123"}}})
	if !strings.HasPrefix(m.Status(), "create failed: fix envs/staging.secret.yaml first") {
		t.Fatalf("status = %q", m.Status())
	}
	if after := requestFiles(t, ws); !slices.Equal(before, after) {
		t.Fatalf("request file left behind: %v", after)
	}
}
