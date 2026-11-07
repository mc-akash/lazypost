package workspace

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func newWS(t *testing.T) *Workspace {
	t.Helper()
	ws, err := Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return ws
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRequestRoundTrip(t *testing.T) {
	ws := newWS(t)
	r, err := ws.CreateRequest(ws.Collections, "Full")
	if err != nil {
		t.Fatal(err)
	}
	r.Method = "POST"
	r.URL = "{{base_url}}/x"
	r.Params = []KV{{Key: "a", Value: "1"}, {Key: "b", Value: "2", Disabled: true}}
	r.Headers = []KV{{Key: "H", Value: "v", Disabled: true}}
	r.Auth = Auth{Type: "apikey", Key: "k", Value: "v", In: "query"}
	r.Body = Body{Type: "multipart", Parts: []Part{{Key: "f", Value: "/tmp/x", File: true}, {Key: "t", Value: "txt", Disabled: true}}}
	if err := ws.SaveRequest(r); err != nil {
		t.Fatal(err)
	}
	ws2, err := Load(ws.Root)
	if err != nil {
		t.Fatal(err)
	}
	got := ws2.FindRequest(r.Path)
	if got == nil {
		t.Fatal("request not found after load")
	}
	a, b := *r, *got
	a.Path, a.Parent, b.Path, b.Parent = "", nil, "", nil
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("round trip mismatch:\n%+v\n%+v", a, b)
	}
}

func TestDisabledOmittedWhenFalse(t *testing.T) {
	ws := newWS(t)
	r, _ := ws.CreateRequest(ws.Collections, "x")
	r.Params = []KV{{Key: "a", Value: "1"}}
	if err := ws.SaveRequest(r); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(r.Path)
	if strings.Contains(string(data), "disabled") {
		t.Fatalf("unexpected disabled key:\n%s", data)
	}
}

func TestLoadSkipsMalformedFile(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "collections", "good.yaml"), "name: Good\nmethod: GET\nurl: http://x\n")
	write(t, filepath.Join(root, "collections", "bad.yaml"), "method: [unclosed")
	ws, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(ws.LoadErrors) != 1 || !strings.HasSuffix(ws.LoadErrors[0].Path, "bad.yaml") {
		t.Fatalf("LoadErrors = %+v", ws.LoadErrors)
	}
	if len(ws.Collections.Requests) != 1 || ws.Collections.Requests[0].Name != "Good" {
		t.Fatalf("requests = %+v", ws.Collections.Requests)
	}
}

func TestLoadDefaults(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "collections", "r.yaml"), "name: R\nurl: http://x\n")
	ws, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if m := ws.Collections.Requests[0].Method; m != "GET" {
		t.Fatalf("method = %q", m)
	}
}

func TestLoadRootUnreadable(t *testing.T) {
	f := filepath.Join(t.TempDir(), "file")
	write(t, f, "x")
	if _, err := Load(filepath.Join(f, "sub")); err == nil {
		t.Fatal("expected error")
	}
}

func TestTreeOrdering(t *testing.T) {
	root := t.TempDir()
	c := filepath.Join(root, "collections")
	for _, p := range []string{"zeta/r.yaml", "alpha/r.yaml", "mid/r.yaml", "b.yaml", "a.yaml"} {
		write(t, filepath.Join(c, p), "name: n\nurl: u\n")
	}
	ws, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	var fs, rs []string
	for _, f := range ws.Collections.Folders {
		fs = append(fs, f.Name)
	}
	for _, r := range ws.Collections.Requests {
		rs = append(rs, filepath.Base(r.Path))
	}
	if !reflect.DeepEqual(fs, []string{"alpha", "mid", "zeta"}) || !reflect.DeepEqual(rs, []string{"a.yaml", "b.yaml"}) {
		t.Fatalf("folders=%v requests=%v", fs, rs)
	}
}

func TestFolderFileLoaded(t *testing.T) {
	root := t.TempDir()
	d := filepath.Join(root, "collections", "api")
	write(t, filepath.Join(d, "_folder.yaml"), "vars:\n  x: \"1\"\nauth:\n  type: bearer\n  token: t\n")
	write(t, filepath.Join(d, "r.yaml"), "name: r\nurl: u\n")
	ws, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	f := ws.Collections.Folders[0]
	if f.Vars["x"] != "1" || f.Auth.Type != "bearer" || f.Auth.Token != "t" {
		t.Fatalf("folder = %+v", f)
	}
	if len(f.Requests) != 1 {
		t.Fatalf("requests = %+v", f.Requests)
	}
}

func TestLoadEnvsAndSecrets(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "lazypost.yaml"), "globals:\n  g: \"1\"\n")
	write(t, filepath.Join(root, "secret.yaml"), "shared: s\n")
	write(t, filepath.Join(root, "envs", "dev.yaml"), "base_url: d\n")
	write(t, filepath.Join(root, "envs", "dev.secret.yaml"), "token: t\n")
	write(t, filepath.Join(root, "envs", "orphan.secret.yaml"), "token: o\n")
	ws, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(ws.LoadErrors) != 0 || ws.Globals["g"] != "1" || ws.SharedSecrets["shared"] != "s" {
		t.Fatalf("ws = %+v", ws)
	}
	if len(ws.Envs) != 2 || ws.Envs[0].Name != "dev" || ws.Envs[1].Name != "orphan" {
		t.Fatalf("envs = %+v", ws.Envs)
	}
	if ws.Envs[0].Vars["base_url"] != "d" || ws.Envs[0].Secrets["token"] != "t" {
		t.Fatalf("dev = %+v", ws.Envs[0])
	}
	if len(ws.Envs[1].Vars) != 0 || ws.Envs[1].Secrets["token"] != "o" {
		t.Fatalf("orphan = %+v", ws.Envs[1])
	}
}

func TestFolderChain(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "collections", "a", "b", "r.yaml"), "name: r\nurl: u\n")
	ws, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	r := ws.Collections.Folders[0].Folders[0].Requests[0]
	chain := ws.FolderChain(r)
	if len(chain) != 3 || chain[0] != ws.Collections || chain[1].Name != "a" || chain[2].Name != "b" {
		t.Fatalf("chain = %+v", chain)
	}
}

func TestLoadLargeWorkspaceIsFast(t *testing.T) {
	if testing.Short() || raceEnabled {
		t.Skip("perf test skipped in short/race mode")
	}
	root := t.TempDir()
	for f := 0; f < 50; f++ {
		for r := 0; r < 10; r++ {
			p := filepath.Join(root, "collections", "f"+string(rune('a'+f/26))+string(rune('a'+f%26)), "r"+string(rune('0'+r))+".yaml")
			write(t, p, "name: R\nmethod: POST\nurl: http://x/y\nheaders:\n  - key: A\n    value: b\nbody:\n  type: json\n  content: '{\"a\":1}'\n")
		}
	}
	start := time.Now()
	ws, err := Load(root)
	elapsed := time.Since(start)
	if err != nil || len(ws.LoadErrors) != 0 {
		t.Fatal(err, ws.LoadErrors)
	}
	if elapsed > 100*time.Millisecond {
		t.Fatalf("Load took %v", elapsed)
	}
}

func TestLoadRecordsUnreadableSubfolder(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permissions not enforced for root")
	}
	root := t.TempDir()
	write(t, filepath.Join(root, "collections", "ok.yaml"), "name: ok\nurl: u\n")
	locked := filepath.Join(root, "collections", "locked")
	write(t, filepath.Join(locked, "r.yaml"), "name: r\nurl: u\n")
	if err := os.Chmod(locked, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(locked, 0o755) })
	ws, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(ws.LoadErrors) != 1 || ws.LoadErrors[0].Path != locked {
		t.Fatalf("LoadErrors = %+v", ws.LoadErrors)
	}
	if len(ws.Collections.Folders) != 0 || len(ws.Collections.Requests) != 1 {
		t.Fatalf("collections = %+v", ws.Collections)
	}
}
