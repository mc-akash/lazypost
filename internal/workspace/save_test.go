package workspace

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func exists(p string) bool { _, err := os.Stat(p); return err == nil }

func TestSlugAndCollision(t *testing.T) {
	if got := Slug("Get User!"); got != "get-user" {
		t.Fatalf("slug = %q", got)
	}
	if got := Slug("  "); got != "request" {
		t.Fatalf("slug = %q", got)
	}
	ws := newWS(t)
	a, _ := ws.CreateRequest(ws.Collections, "Get User")
	b, _ := ws.CreateRequest(ws.Collections, "Get User")
	if filepath.Base(a.Path) != "get-user.yaml" || filepath.Base(b.Path) != "get-user-2.yaml" {
		t.Fatalf("paths %s %s", a.Path, b.Path)
	}
	if a.Method != "GET" || a.URL != "{{base_url}}/" || a.Parent != ws.Collections {
		t.Fatalf("request = %+v", a)
	}
}

func TestCreateRequestKeepsSorted(t *testing.T) {
	ws := newWS(t)
	for _, n := range []string{"b", "c", "a"} {
		if _, err := ws.CreateRequest(ws.Collections, n); err != nil {
			t.Fatal(err)
		}
	}
	var got []string
	for _, r := range ws.Collections.Requests {
		got = append(got, filepath.Base(r.Path))
	}
	if !reflect.DeepEqual(got, []string{"a.yaml", "b.yaml", "c.yaml"}) {
		t.Fatalf("got %v", got)
	}
}

func TestFolderCreateDelete(t *testing.T) {
	ws := newWS(t)
	f, err := ws.CreateFolder(ws.Collections, "My API")
	if err != nil {
		t.Fatal(err)
	}
	f2, _ := ws.CreateFolder(ws.Collections, "My API")
	if f.Name != "my-api" || f2.Name != "my-api-2" || f.Parent != ws.Collections {
		t.Fatalf("folders %+v %+v", f, f2)
	}
	r, _ := ws.CreateRequest(f, "r")
	if err := ws.DeleteRequest(r); err != nil || exists(r.Path) || len(f.Requests) != 0 {
		t.Fatalf("delete request: %v", err)
	}
	r2, _ := ws.CreateRequest(f, "r2")
	if err := ws.DeleteFolder(f); err != nil {
		t.Fatal(err)
	}
	if exists(f.Path) || exists(r2.Path) || len(ws.Collections.Folders) != 1 || ws.Collections.Folders[0] != f2 {
		t.Fatal("folder not removed")
	}
}

func TestEnvLifecycle(t *testing.T) {
	ws := newWS(t)
	if _, err := ws.CreateEnv("staging"); err != nil {
		t.Fatal(err)
	}
	if _, err := ws.CreateEnv("staging"); err == nil {
		t.Fatal("expected duplicate error")
	}
	if _, err := ws.CreateEnv("prod.secret"); err == nil {
		t.Fatal("expected .secret name error")
	}
	if _, err := ws.CreateEnv("../x"); err == nil {
		t.Fatal("expected invalid name error")
	}
	d, _ := ws.CreateEnv("dev")
	if ws.Envs[0] != d {
		t.Fatal("envs not sorted")
	}
	d.Secrets = map[string]string{"t": "s"}
	if err := ws.SaveEnv(d); err != nil {
		t.Fatal(err)
	}
	if err := ws.DeleteEnv(d); err != nil {
		t.Fatal(err)
	}
	if exists(filepath.Join(ws.Root, "envs", "dev.yaml")) || exists(filepath.Join(ws.Root, "envs", "dev.secret.yaml")) || len(ws.Envs) != 1 {
		t.Fatal("env not removed")
	}
}

func TestSaveEnvSplitsSecrets(t *testing.T) {
	ws := newWS(t)
	e, _ := ws.CreateEnv("staging")
	e.Vars = map[string]string{"base_url": "u"}
	e.Secrets = map[string]string{"token": "s3cr3t"}
	if err := ws.SaveEnv(e); err != nil {
		t.Fatal(err)
	}
	plain, _ := os.ReadFile(filepath.Join(ws.Root, "envs", "staging.yaml"))
	sp := filepath.Join(ws.Root, "envs", "staging.secret.yaml")
	sec, _ := os.ReadFile(sp)
	if string(plain) == "" || strings.Contains(string(plain), "s3cr3t") || !strings.Contains(string(sec), "s3cr3t") {
		t.Fatalf("plain=%q secret=%q", plain, sec)
	}
	if fi, _ := os.Stat(sp); fi.Mode().Perm() != 0o600 {
		t.Fatalf("secret mode %v", fi.Mode())
	}
	e.Secrets = nil
	if err := ws.SaveEnv(e); err != nil {
		t.Fatal(err)
	}
	if exists(sp) {
		t.Fatal("secret file should be removed")
	}
}

func TestOldSettingsReported(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "lazypost.yaml")
	write(t, path, "timeout: 7s\ninsecure: true\nglobals:\n  g: \"1\"\n")
	ws, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(ws.LoadErrors) != 1 || ws.LoadErrors[0].Path != path || !strings.Contains(ws.LoadErrors[0].Err.Error(), "timeout and insecure moved to ~/.config/lazypost/config.yaml") {
		t.Fatalf("LoadErrors = %+v", ws.LoadErrors)
	}
	if ws.Globals["g"] != "1" {
		t.Fatalf("globals = %v", ws.Globals)
	}
}

func TestAtomicWriteLeavesNoTemp(t *testing.T) {
	ws := newWS(t)
	r, _ := ws.CreateRequest(ws.Collections, "x")
	_ = ws.SaveRequest(r)
	ents, _ := os.ReadDir(ws.Collections.Path)
	if len(ents) != 1 {
		t.Fatalf("entries = %v", ents)
	}
}

func TestCreateRequestPropagatesStatError(t *testing.T) {
	ws := newWS(t)
	file := filepath.Join(ws.Root, "afile")
	write(t, file, "x")
	bad := &Folder{Path: file, Parent: ws.Collections}
	if _, err := ws.CreateRequest(bad, "x"); err == nil {
		t.Fatal("expected error")
	}
	if _, err := ws.CreateFolder(bad, "x"); err == nil {
		t.Fatal("expected error")
	}
}

func TestSaveEnvRefusesWhenEnvFileFailedToLoad(t *testing.T) {
	for _, file := range []string{"dev.secret.yaml", "dev.yaml"} {
		t.Run(file, func(t *testing.T) {
			root := t.TempDir()
			write(t, filepath.Join(root, "envs", "dev.yaml"), "a: \"1\"\n")
			write(t, filepath.Join(root, "envs", "dev.secret.yaml"), "s: \"x\"\n")
			broken := filepath.Join(root, "envs", file)
			write(t, broken, "{not yaml")
			ws, err := Load(root)
			if err != nil {
				t.Fatal(err)
			}
			e := ws.Envs[0]
			e.Vars["b"] = "2"
			err = ws.SaveEnv(e)
			if err == nil || !strings.Contains(err.Error(), "fix envs/"+file+" first") {
				t.Fatalf("err = %v", err)
			}
			if b, _ := os.ReadFile(broken); string(b) != "{not yaml" {
				t.Fatalf("broken file changed: %q", b)
			}
		})
	}
}
