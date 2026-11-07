package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidMethod(t *testing.T) {
	for _, m := range []string{"GET", "POST", "PROPFIND", "M-SEARCH", "x!#$%&'*+.^_`|~1"} {
		if !ValidMethod(m) {
			t.Errorf("ValidMethod(%q) = false, want true", m)
		}
	}
	for _, m := range []string{"", "GET;$'\\143'", "GE T", "GET\n", "GÉT", "a\"b", "a(b)", "a/b"} {
		if ValidMethod(m) {
			t.Errorf("ValidMethod(%q) = true, want false", m)
		}
	}
}

func TestLoadRejectsInvalidMethod(t *testing.T) {
	root := t.TempDir()
	bad := filepath.Join(root, "collections", "bad.yaml")
	write(t, bad, "name: bad\nmethod: \"GET;$'\\\\143'\"\nurl: http://a.io\n")
	write(t, filepath.Join(root, "collections", "good.yaml"), "name: good\nmethod: post\nurl: http://a.io\n")

	ws, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(ws.Collections.Requests) != 1 || ws.Collections.Requests[0].Name != "good" {
		t.Fatalf("requests = %+v, want only good", ws.Collections.Requests)
	}
	if len(ws.LoadErrors) != 1 || ws.LoadErrors[0].Path != bad || !strings.Contains(ws.LoadErrors[0].Err.Error(), "invalid method") {
		t.Fatalf("LoadErrors = %+v, want invalid method for %s", ws.LoadErrors, bad)
	}
}

func TestSaveEnvWritesSecretFileFirst(t *testing.T) {
	ws := newWS(t)
	plain, secret := ws.envPaths("dev")

	if err := os.Mkdir(plain, 0o755); err != nil {
		t.Fatal(err)
	}
	err := ws.SaveEnv(&Env{Name: "dev", Vars: map[string]string{"a": "1"}, Secrets: map[string]string{"token": "s3cret"}})
	if err == nil {
		t.Fatal("SaveEnv succeeded, want error from the plain file")
	}
	data, rerr := os.ReadFile(secret)
	if rerr != nil || !strings.Contains(string(data), "s3cret") {
		t.Fatalf("secret file = %q, %v; want it written before the plain file", data, rerr)
	}
}
