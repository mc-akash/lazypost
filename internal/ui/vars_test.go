package ui

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mc-akash/lazypost/internal/state"
	"github.com/mc-akash/lazypost/internal/workspace"
)

type errClip struct{ err error }

func (c errClip) Read() (string, error) { return "", c.err }
func (c errClip) Write(string) error    { return c.err }

func typeText(m Model, s string) Model {
	for _, r := range s {
		m = press(m, string(r))
	}
	return m
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func envFile(ws *workspace.Workspace, name string) string {
	return filepath.Join(ws.Root, "envs", name+".yaml")
}

func secretFile(ws *workspace.Workspace, name string) string {
	return filepath.Join(ws.Root, "envs", name+".secret.yaml")
}

func varsRow(t *testing.T, m Model, name string) Model {
	t.Helper()
	for range 20 {
		if row, ok := m.selectedVar(); ok && row.Name == name {
			return m
		}
		m = press(m, "j")
	}
	t.Fatalf("no variable row %q", name)
	return m
}

func TestEnvCursorDoesNotActivate(t *testing.T) {
	rec := &recorder{}
	srv := serve(t, rec)
	ws := fixtureWS(t)
	bearerFolder(t, ws)
	m, _ := sendModel(t, ws, srv.URL)
	m = press(m, "1", "k")
	if got := m.selectedRowLabel(); got != "dev" {
		t.Fatalf("highlighted %q, want dev", got)
	}
	if e := m.activeEnv(); e == nil || e.Name != "staging" {
		t.Fatalf("active env = %v, want staging", e)
	}
	m = pressRun(press(m, "2"), "s")
	if got := rec.lastAuth(); got != "Bearer staging-secret-token" {
		t.Fatalf("server saw %q, want the staging token", got)
	}
	if e := m.activeEnv(); e == nil || e.Name != "staging" {
		t.Fatalf("active env after send = %v, want staging", e)
	}
}

func TestSpaceActivatesAndPersists(t *testing.T) {
	ws := fixtureWS(t)
	d := testDeps(t, ws)
	draftKey := "collections/orders/list-orders.yaml"
	prior := state.Session{LastEnv: "staging", LastRequest: "collections/users/get-user.yaml",
		Drafts: map[string]state.Draft{draftKey: {Request: workspace.Request{Name: "draft"}}}}
	if err := d.StateStore.Save(ws.Root, prior); err != nil {
		t.Fatal(err)
	}
	d.Session = prior
	m := resize(newModel(d), 120, 40)
	m = press(m, "1", "k", "space")
	if e := m.activeEnv(); e == nil || e.Name != "dev" {
		t.Fatalf("active env = %v, want dev", e)
	}
	sess, err := d.StateStore.Load(ws.Root)
	if err != nil {
		t.Fatal(err)
	}
	if sess.LastEnv != "dev" {
		t.Fatalf("LastEnv = %q, want dev", sess.LastEnv)
	}
	if sess.LastRequest != prior.LastRequest || sess.Drafts[draftKey].Request.Name != "draft" {
		t.Fatalf("session clobbered: %+v", sess)
	}
	if !strings.Contains(m.plainRender(), "● dev") {
		t.Fatal("active marker not on dev")
	}
}

func TestFocusEnvsShowsVarsTable(t *testing.T) {
	ws := fixtureWS(t)
	req := requestByName(t, folderByName(t, ws, "users"), "get user")
	req.Headers = append(req.Headers, workspace.KV{Key: "X-Trace", Value: "{{trace_id}}"})
	m, _ := sendModel(t, ws, "http://example.invalid")
	m = press(m, "1")
	out := m.plainRender()
	for _, want := range []string{"[4] Variables · staging", "NAME", "VALUE", "FROM", "base_url", "trace_id", "(missing)", "global", "env", "secret"} {
		if !strings.Contains(out, want) {
			t.Fatalf("vars table missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "[5] Response") {
		t.Fatal("vars table should take the request and response area")
	}
	if !strings.Contains(m.Render(), missingVarStyle.Render("(missing)")) {
		t.Fatal("(missing) should be red")
	}

	if out := press(m, "k").plainRender(); !strings.Contains(out, "[4] Variables · dev") || !strings.Contains(out, "http://dev.example") {
		t.Fatalf("table should show the highlighted env dev:\n%s", out)
	}
}

func TestSecretMaskedUntilReveal(t *testing.T) {
	m := press(newTestModel(t, fixtureWS(t)), "1")
	out := m.plainRender()
	if strings.Contains(out, "staging-secret-token") {
		t.Fatal("secret shown before reveal")
	}
	if !strings.Contains(out, "••••••••") {
		t.Fatalf("masked secret missing:\n%s", out)
	}
	m = varsRow(t, press(m, "V"), "token")
	m = press(m, "r")
	if !strings.Contains(m.plainRender(), "staging-secret-token") {
		t.Fatal("r should reveal the selected secret")
	}
	if m = press(m, "r"); strings.Contains(m.plainRender(), "staging-secret-token") {
		t.Fatal("second r should mask it again")
	}
}

func TestVJumpAndEscReturn(t *testing.T) {
	m := press(newTestModel(t, fixtureWS(t)), "1", "k", "5")
	if m = press(m, "V"); m.Focus() != PanelVars {
		t.Fatalf("V: focus = %v, want PanelVars", m.Focus())
	}
	if !strings.Contains(m.plainRender(), "[4] Variables · staging") {
		t.Fatal("V should show the active env staging")
	}
	if m = press(m, "esc"); m.Focus() != PanelResponse {
		t.Fatalf("esc: focus = %v, want PanelResponse", m.Focus())
	}
	m = press(m, "1", "enter")
	if m.Focus() != PanelVars {
		t.Fatalf("enter from envs: focus = %v, want PanelVars", m.Focus())
	}
	if m = press(m, "esc"); m.Focus() != PanelEnvs {
		t.Fatalf("esc: focus = %v, want PanelEnvs", m.Focus())
	}
}

func TestPasteWritesSecretImmediately(t *testing.T) {
	ws := fixtureWS(t)
	d := testDeps(t, ws)
	d.Clip = &fakeClip{text: "  newtok\n"}
	m := varsRow(t, press(resize(newModel(d), 120, 40), "V"), "token")
	m = press(m, "p")
	if got := readFile(t, secretFile(ws, "staging")); !strings.Contains(got, "newtok") {
		t.Fatalf("secret file lacks pasted value:\n%s", got)
	}
	if got := readFile(t, envFile(ws, "staging")); strings.Contains(got, "newtok") {
		t.Fatalf("secret leaked into env file:\n%s", got)
	}
	if envByName(t, ws, "staging").Secrets["token"] != "newtok" {
		t.Fatal("pasted value not trimmed")
	}
	if m.Status() != "saved token (secret)" {
		t.Fatalf("status = %q", m.Status())
	}
}

func TestPasteFallbackInput(t *testing.T) {
	ws := fixtureWS(t)
	d := testDeps(t, ws)
	d.Clip = errClip{err: errors.New("no clipboard tool")}
	m := varsRow(t, press(resize(newModel(d), 120, 40), "V"), "base_url")
	m = press(m, "p")
	if m.input() == nil {
		t.Fatal("paste with no clipboard should open an input")
	}
	if !strings.Contains(m.plainRender(), "┌"+dialogPasteValue) {
		t.Fatal("paste prompt missing")
	}
	m = press(typeText(m, " abc "), "enter")
	if got := envByName(t, ws, "staging").Vars["base_url"]; got != "abc" {
		t.Fatalf("base_url = %q, want abc", got)
	}
	if !strings.Contains(readFile(t, envFile(ws, "staging")), "abc") {
		t.Fatal("not saved to disk")
	}
	if m.Status() != "saved base_url" {
		t.Fatalf("status = %q", m.Status())
	}
}

func TestEditInheritedCreatesEnvOverride(t *testing.T) {
	ws := fixtureWS(t)
	globalsBefore := readFile(t, filepath.Join(ws.Root, "lazypost.yaml"))
	m := varsRow(t, press(newTestModel(t, ws), "V"), "api_ver")
	m = press(m, "enter")
	if m.input() == nil || m.input().ti.Value() != "v1" {
		t.Fatal("edit input should be prefilled with the current value")
	}
	m = press(typeText(press(m, "backspace"), "2"), "enter")
	if got := readFile(t, envFile(ws, "staging")); !strings.Contains(got, "api_ver: v2") {
		t.Fatalf("staging.yaml lacks override:\n%s", got)
	}
	if got := readFile(t, filepath.Join(ws.Root, "lazypost.yaml")); got != globalsBefore {
		t.Fatalf("lazypost.yaml changed:\n%s", got)
	}
	if ws.Globals["api_ver"] != "v1" {
		t.Fatal("global value changed in memory")
	}
	if m.Status() != "saved api_ver" {
		t.Fatalf("status = %q", m.Status())
	}
}

func TestEditMaskedSecretStartsEmpty(t *testing.T) {
	ws := fixtureWS(t)
	m := varsRow(t, press(newTestModel(t, ws), "V"), "token")
	m = press(m, "enter")
	if m.input() == nil || m.input().ti.Value() != "" {
		t.Fatal("masked secret should open an empty input")
	}
	if strings.Contains(m.plainRender(), "staging-secret-token") {
		t.Fatal("secret shown while editing")
	}
	m = press(typeText(m, "fresh"), "enter")
	if envByName(t, ws, "staging").Secrets["token"] != "fresh" {
		t.Fatal("edit not saved")
	}
	if strings.Contains(readFile(t, envFile(ws, "staging")), "fresh") {
		t.Fatal("secret written to env file")
	}
	if m.Status() != "saved token (secret)" || strings.Contains(m.plainRender(), "fresh") {
		t.Fatalf("status = %q, or the saved secret is shown", m.Status())
	}
}

func TestAddDefaultsSecretForTokenNames(t *testing.T) {
	ws := fixtureWS(t)
	m := press(newTestModel(t, ws), "V")
	m = press(typeText(press(m, "a"), "api_token"), "enter")
	m = press(typeText(m, "x"), "enter")
	if !strings.Contains(m.plainRender(), "yes: enter  yes: y  no: n") {
		t.Fatalf("secret prompt missing:\n%s", m.plainRender())
	}
	m = press(m, "enter")
	staging := envByName(t, ws, "staging")
	if staging.Secrets["api_token"] != "x" {
		t.Fatalf("api_token not in secrets: %v", staging.Secrets)
	}
	if m.Status() != "saved api_token (secret)" {
		t.Fatalf("status = %q", m.Status())
	}
	m = press(typeText(press(m, "a"), "region"), "enter")
	m = press(typeText(m, "eu"), "enter")
	if !strings.Contains(m.plainRender(), "no: enter  yes: y  no: n") {
		t.Fatal("non-secret prompt should default to no")
	}
	m = press(m, "enter")
	if staging.Vars["region"] != "eu" || !strings.Contains(readFile(t, envFile(ws, "staging")), "region: eu") {
		t.Fatalf("region not in vars: %v", staging.Vars)
	}

	m = press(typeText(press(m, "a"), "zone"), "enter")
	m = press(press(typeText(m, "z1"), "enter"), "y")
	if staging.Secrets["zone"] != "z1" || staging.Vars["zone"] != "" {
		t.Fatal("y should save zone as a secret")
	}
	if m.Status() != "saved zone (secret)" {
		t.Fatalf("status = %q", m.Status())
	}
}

func TestDeleteInheritedRefused(t *testing.T) {
	ws := fixtureWS(t)
	m := varsRow(t, press(newTestModel(t, ws), "V"), "api_ver")
	m = press(m, "d")
	if !strings.Contains(m.Status(), "inherited") {
		t.Fatalf("status = %q, want inherited refusal", m.Status())
	}
	if ws.Globals["api_ver"] != "v1" {
		t.Fatal("global deleted")
	}
}

func TestDeleteEnvVar(t *testing.T) {
	ws := fixtureWS(t)
	m := varsRow(t, press(newTestModel(t, ws), "V"), "base_url")
	m = press(m, "d", "n")
	if _, ok := envByName(t, ws, "staging").Vars["base_url"]; !ok {
		t.Fatal("n should keep the variable")
	}
	m = press(m, "d")
	if !strings.Contains(m.plainRender(), `Delete "base_url"?`) {
		t.Fatal("delete confirmation missing")
	}
	press(m, "y")
	if _, ok := envByName(t, ws, "staging").Vars["base_url"]; ok {
		t.Fatal("y should delete the variable")
	}
	if strings.Contains(readFile(t, envFile(ws, "staging")), "base_url") {
		t.Fatal("deletion not saved")
	}
}

func TestMoveToSecret(t *testing.T) {
	ws := fixtureWS(t)
	envByName(t, ws, "staging").Vars["user_id"] = "42"
	m := varsRow(t, press(newTestModel(t, ws), "V"), "user_id")
	m = press(m, "S")
	if got := readFile(t, secretFile(ws, "staging")); !strings.Contains(got, "user_id") {
		t.Fatalf("user_id not moved to secrets:\n%s", got)
	}
	if got := readFile(t, envFile(ws, "staging")); strings.Contains(got, "user_id") {
		t.Fatalf("user_id still in env file:\n%s", got)
	}
	m = press(m, "S")
	if !strings.Contains(readFile(t, envFile(ws, "staging")), "user_id: \"42\"") {
		t.Fatal("second S should move it back to vars")
	}
	if m.Status() != "saved user_id" {
		t.Fatalf("status = %q", m.Status())
	}
}

func TestPickerActivates(t *testing.T) {
	ws := fixtureWS(t)
	d := testDeps(t, ws)
	m := press(resize(newModel(d), 120, 40), "E")
	if !strings.Contains(m.plainRender(), "staging") {
		t.Fatal("picker should list envs")
	}
	m = press(typeText(press(m, "/"), "dev"), "enter")
	if e := m.activeEnv(); e == nil || e.Name != "dev" {
		t.Fatalf("active env = %v, want dev", e)
	}
	if sess, _ := d.StateStore.Load(ws.Root); sess.LastEnv != "dev" {
		t.Fatalf("LastEnv = %q, want dev", sess.LastEnv)
	}
	if m.picker() != nil {
		t.Fatal("picker should close after enter")
	}
	m = press(m, "E", "esc")
	if m.picker() != nil || m.activeEnv().Name != "dev" {
		t.Fatal("esc should close the picker without changes")
	}
}

func TestNewEnv(t *testing.T) {
	ws := fixtureWS(t)
	m := press(newTestModel(t, ws), "1", "n")
	m = press(typeText(m, "prod"), "enter")
	if _, err := os.Stat(envFile(ws, "prod")); err != nil {
		t.Fatalf("prod env file not created: %v", err)
	}
	if m.selectedRowLabel() != "prod" || m.activeEnv().Name != "staging" {
		t.Fatal("new env should be highlighted but not activated")
	}
}

func TestDeleteEnvAsksOnce(t *testing.T) {
	ws := fixtureWS(t)
	m := press(newTestModel(t, ws), "1", "d")
	if !strings.Contains(m.plainRender(), `Delete "staging"?`) {
		t.Fatal("delete confirmation missing")
	}
	m = press(m, "n")
	if _, err := os.Stat(envFile(ws, "staging")); err != nil {
		t.Fatal("n should keep the env")
	}
	m = press(m, "d", "y")
	if len(ws.Envs) != 1 || ws.Envs[0].Name != "dev" {
		t.Fatalf("envs = %v, want only dev", ws.Envs)
	}
	for _, p := range []string{envFile(ws, "staging"), secretFile(ws, "staging")} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Fatalf("%s should be removed", p)
		}
	}
	if e := m.activeEnv(); e == nil || e.Name != "dev" {
		t.Fatalf("deleting the active env should activate dev, got %v", e)
	}
	if sess, err := m.deps.StateStore.Load(ws.Root); err != nil || sess.LastEnv != "dev" {
		t.Fatalf("LastEnv not persisted: %+v %v", sess, err)
	}
	m = press(m, "d", "y")
	if m.activeEnv() != nil || len(ws.Envs) != 0 {
		t.Fatal("deleting the last env should leave none active")
	}
	if sess, _ := m.deps.StateStore.Load(ws.Root); sess.LastEnv != "" {
		t.Fatalf("LastEnv = %q, want empty", sess.LastEnv)
	}
}
