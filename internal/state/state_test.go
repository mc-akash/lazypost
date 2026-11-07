package state

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/mc-akash/lazypost/internal/workspace"
)

func TestSessionRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	s := Open(path)
	want := Session{
		LastEnv:     "dev",
		LastRequest: "users/get.yaml",
		Drafts: map[string]Draft{
			"users/get.yaml": {
				Request: workspace.Request{
					Name:    "get",
					Method:  "GET",
					URL:     "https://x/{{id}}",
					Headers: []workspace.KV{{Key: "A", Value: "b", Disabled: true}},
					Auth:    workspace.Auth{Type: "bearer", Token: "t"},
					Body:    workspace.Body{Type: "json", Content: `{"a":1}`},
				},
				BaseModTime: time.Date(2026, 11, 7, 10, 0, 0, 0, time.UTC),
			},
		},
	}
	if err := s.Save("/ws", want); err != nil {
		t.Fatal(err)
	}
	got, err := Open(path).Load("/ws")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v\nwant %+v", got, want)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v, want 0600", fi.Mode().Perm())
	}
}

func TestMissingFileEmpty(t *testing.T) {
	got, err := Open(filepath.Join(t.TempDir(), "nope", "state.json")).Load("/ws")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, Session{Drafts: map[string]Draft{}}) {
		t.Fatalf("got %+v, want empty", got)
	}
}

func TestEmptySessionDraftsNonNil(t *testing.T) {
	s := Open(filepath.Join(t.TempDir(), "state.json"))
	if err := s.Save("/ws", Session{LastEnv: "e"}); err != nil {
		t.Fatal(err)
	}
	got, err := s.Load("/ws")
	if err != nil {
		t.Fatal(err)
	}
	if got.Drafts == nil {
		t.Fatal("Drafts is nil")
	}
}

func TestRootsIndependent(t *testing.T) {
	s := Open(filepath.Join(t.TempDir(), "sub", "state.json"))
	a := Session{LastEnv: "a", Drafts: map[string]Draft{}}
	b := Session{LastEnv: "b", Drafts: map[string]Draft{}}
	if err := s.Save("/a", a); err != nil {
		t.Fatal(err)
	}
	if err := s.Save("/b", b); err != nil {
		t.Fatal(err)
	}
	for root, want := range map[string]Session{"/a": a, "/b": b} {
		got, err := s.Load(root)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("%s: got %+v, want %+v", root, got, want)
		}
	}
}

func TestCorruptFileReturnsError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(path, []byte("{garbage"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := Open(path)
	got, err := s.Load("/ws")
	if err == nil {
		t.Fatal("want error")
	}
	if got.LastEnv != "" || got.LastRequest != "" || len(got.Drafts) != 0 {
		t.Fatalf("got %+v, want zero", got)
	}
	want := Session{LastEnv: "x", Drafts: map[string]Draft{}}
	if err := s.Save("/ws", want); err != nil {
		t.Fatalf("Save over corrupt file: %v", err)
	}
	got, err = s.Load("/ws")
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("after Save: got %+v, %v", got, err)
	}
	old, err := os.ReadFile(path + ".corrupt")
	if err != nil || string(old) != "{garbage" {
		t.Fatalf(".corrupt = %q, %v", old, err)
	}
}

func TestUnreadableAndUnwritablePaths(t *testing.T) {
	dir := t.TempDir()
	if _, err := Open(dir).Load("/ws"); err == nil {
		t.Fatal("Load of a directory: want error")
	}
	blocker := filepath.Join(dir, "file")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Open(filepath.Join(blocker, "state.json")).Save("/ws", Session{}); err == nil {
		t.Fatal("Save under a regular file: want error")
	}
}

func TestWriteAtomicRenameFailureCleansUp(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	if err := os.MkdirAll(filepath.Join(target, "child"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := writeAtomic(target, []byte("x")); err == nil {
		t.Fatal("rename onto non-empty dir: want error")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("temp file left behind: %v", entries)
	}
}

func TestScratchRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	want := Session{
		LastEnv: "dev",
		Drafts:  map[string]Draft{},
		Scratch: &workspace.Request{
			Name:    "unsaved",
			Method:  "POST",
			URL:     "{{base_url}}/x",
			Headers: []workspace.KV{{Key: "Authorization", Value: "Bearer {{token}}"}},
			Body:    workspace.Body{Type: "json", Content: `{"a":1}`},
		},
	}
	if err := Open(path).Save("/ws", want); err != nil {
		t.Fatal(err)
	}
	got, err := Open(path).Load("/ws")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v\nwant %+v", got, want)
	}
	if fi, err := os.Stat(path); err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("state file mode: %v %v", fi, err)
	}
}

func TestOldSessionWithoutScratchLoads(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	old := `{"/ws": {"last_env": "dev", "last_request": "collections/a.yaml", "drafts": {}}}`
	if err := os.WriteFile(path, []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := Open(path).Load("/ws")
	if err != nil {
		t.Fatal(err)
	}
	if got.Scratch != nil || got.LastEnv != "dev" || got.LastRequest != "collections/a.yaml" {
		t.Fatalf("got %+v", got)
	}

	if err := Open(path).Save("/ws", got); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "scratch") {
		t.Fatalf("empty scratch written:\n%s", data)
	}
}
