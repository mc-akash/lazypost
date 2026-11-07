package history

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mc-akash/lazypost/internal/workspace"
)

func newStore(t *testing.T) (*Store, string) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "history.jsonl")
	return Open(p), p
}

func entry(i int) Entry {
	return Entry{
		ID:   fmt.Sprintf("e%d", i),
		Time: time.Unix(int64(i), 0).UTC(),
		Code: 200,
		Sent: workspace.Request{Method: "GET", URL: fmt.Sprintf("http://x/%d", i)},
	}
}

func TestLoadMissingFile(t *testing.T) {
	s, _ := newStore(t)
	got, err := s.Load()
	if err != nil || len(got) != 0 {
		t.Fatalf("got %v, %v; want empty, nil", got, err)
	}
}

func TestAppendLoadNewestFirst(t *testing.T) {
	s, _ := newStore(t)
	for i := 1; i <= 3; i++ {
		if err := s.Append(entry(i)); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	for i, want := range []string{"e3", "e2", "e1"} {
		if got[i].ID != want {
			t.Fatalf("got[%d].ID = %q, want %q", i, got[i].ID, want)
		}
	}
	if got[0].Sent.URL != "http://x/3" {
		t.Fatalf("Sent.URL = %q", got[0].Sent.URL)
	}
}

func TestAppendAssignsID(t *testing.T) {
	s, _ := newStore(t)
	e := entry(1)
	e.ID = ""
	if err := s.Append(e); err != nil {
		t.Fatal(err)
	}
	got, _ := s.Load()
	if len(got) != 1 || len(got[0].ID) != 16 {
		t.Fatalf("ID = %q, want 16 hex chars", got[0].ID)
	}
}

func TestCapMaxEntries(t *testing.T) {
	s, p := newStore(t)
	for i := 1; i <= 205; i++ {
		if err := s.Append(entry(i)); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != MaxEntries {
		t.Fatalf("len = %d, want %d", len(got), MaxEntries)
	}
	if got[0].ID != "e205" || got[len(got)-1].ID != "e6" {
		t.Fatalf("newest %q oldest %q, want e205/e6", got[0].ID, got[len(got)-1].ID)
	}
	info, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode after trim = %v, want 0600", info.Mode().Perm())
	}
}

func TestBodyTrimmed(t *testing.T) {
	s, _ := newStore(t)
	e := entry(1)
	e.Body = bytes.Repeat([]byte("a"), 300<<10)
	if err := s.Append(e); err != nil {
		t.Fatal(err)
	}
	got, _ := s.Load()
	if len(got[0].Body) != MaxStoredBody || !got[0].Truncated {
		t.Fatalf("len = %d truncated = %v", len(got[0].Body), got[0].Truncated)
	}
	if len(e.Body) != 300<<10 {
		t.Fatal("Append mutated caller's body")
	}
}

func TestCorruptLineSkipped(t *testing.T) {
	s, p := newStore(t)
	if err := s.Append(entry(1)); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("{not json\n\n"); err != nil {
		t.Fatal(err)
	}
	f.Close()
	if err := s.Append(entry(2)); err != nil {
		t.Fatal(err)
	}
	got, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != "e2" || got[1].ID != "e1" {
		t.Fatalf("got %+v", got)
	}
}

func TestFileMode0600(t *testing.T) {
	s, p := newStore(t)
	if err := s.Append(entry(1)); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v, want 0600", info.Mode().Perm())
	}
}

func TestPathParentNotSerialized(t *testing.T) {
	req := workspace.Request{Name: "r", Path: "/abs/r.yaml", Parent: &workspace.Folder{Name: "f"}}
	req.Parent.Requests = []*workspace.Request{&req}
	b, err := json.Marshal(Entry{Template: req})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "/abs/r.yaml") {
		t.Fatalf("Path serialized: %s", b)
	}
}

func TestRedact(t *testing.T) {
	req := workspace.Request{
		Method:  "POST",
		URL:     "https://api.example.com/v1?token=abc123&q=hello",
		Params:  []workspace.KV{{Key: "token", Value: "abc123"}, {Key: "q", Value: "hello ab"}},
		Headers: []workspace.KV{{Key: "Authorization", Value: "Bearer abc123"}},
		Auth:    workspace.Auth{Type: "basic", Username: "alice", Password: "s3cretpw"},
		Body: workspace.Body{
			Type:    "json",
			Content: `{"password":"s3cretpw","name":"bob"}`,
			Form:    []workspace.KV{{Key: "k", Value: "xabc123x"}},
			Parts:   []workspace.Part{{Key: "p", Value: "s3cretpw"}},
		},
	}
	got := Redact(req, []string{"abc123", "s3cretpw", "ab", ""})

	checks := map[string][2]string{
		"url":    {got.URL, "https://api.example.com/v1?token=••••&q=hello"},
		"param":  {got.Params[0].Value, "••••"},
		"param2": {got.Params[1].Value, "hello ab"},
		"header": {got.Headers[0].Value, "Bearer ••••"},
		"user":   {got.Auth.Username, "alice"},
		"pass":   {got.Auth.Password, "••••"},
		"body":   {got.Body.Content, `{"password":"••••","name":"bob"}`},
		"form":   {got.Body.Form[0].Value, "x••••x"},
		"part":   {got.Body.Parts[0].Value, "••••"},
	}
	for name, c := range checks {
		if c[0] != c[1] {
			t.Errorf("%s: got %q, want %q", name, c[0], c[1])
		}
	}
	if req.Headers[0].Value != "Bearer abc123" || req.Params[0].Value != "abc123" ||
		req.Body.Form[0].Value != "xabc123x" || req.Body.Parts[0].Value != "s3cretpw" {
		t.Fatal("Redact mutated input")
	}
}

func TestRedactAuthTokenAndAPIKey(t *testing.T) {
	got := Redact(workspace.Request{Auth: workspace.Auth{Token: "tok999", Key: "X-Key", Value: "val777"}},
		[]string{"tok999", "val777"})
	if got.Auth.Token != "••••" || got.Auth.Value != "••••" || got.Auth.Key != "X-Key" {
		t.Fatalf("got %+v", got.Auth)
	}
}

func TestRedactAuthHeaderAlwaysMasked(t *testing.T) {
	req := workspace.Request{Headers: []workspace.KV{
		{Key: "Authorization", Value: "Basic dTpwYXNz"},
		{Key: "authorization", Value: "Token abc"},
		{Key: "Proxy-Authorization", Value: "opaque"},
		{Key: "X-Other", Value: "Basic dTpwYXNz"},
	}}
	cases := []struct {
		name    string
		secrets []string
	}{{"with secrets", []string{"pass"}}, {"no secrets", nil}}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Redact(req, c.secrets)
			want := []string{"Basic ••••", "Token ••••", "••••", "Basic dTpwYXNz"}
			for i, w := range want {
				if got.Headers[i].Value != w {
					t.Errorf("%s: got %q, want %q", got.Headers[i].Key, got.Headers[i].Value, w)
				}
			}
			if req.Headers[0].Value != "Basic dTpwYXNz" {
				t.Fatal("Redact mutated input")
			}
		})
	}
}

func TestRedactString(t *testing.T) {
	in := `Get "http://h/?api_key=SUPERSECRET&k=a%2Bb%2Fc": dial tcp: refused; ab`
	got := RedactString(in, []string{"SUPERSECRET", "a+b/c", "ab", ""})
	want := `Get "http://h/?api_key=••••&k=••••": dial tcp: refused; ab`
	if got != want {
		t.Fatalf("got %q\nwant %q", got, want)
	}
	if RedactString("plain", nil) != "plain" {
		t.Fatal("no secrets must leave the string unchanged")
	}
}

func TestRedactMasksQueryEscapedSecret(t *testing.T) {
	got := Redact(workspace.Request{URL: "http://h/?k=a%2Bb%2Fc"}, []string{"a+b/c"})
	if got.URL != "http://h/?k=••••" {
		t.Fatalf("URL = %q", got.URL)
	}
}

func TestRootRoundTrips(t *testing.T) {
	s, _ := newStore(t)
	e := entry(1)
	e.Root = "/ws/a"
	if err := s.Append(e); err != nil {
		t.Fatal(err)
	}
	got, err := s.Load()
	if err != nil || len(got) != 1 || got[0].Root != "/ws/a" {
		t.Fatalf("Load = %+v, %v", got, err)
	}
}

func TestNewIDUnique(t *testing.T) {
	a, errA := NewID()
	b, errB := NewID()
	if errA != nil || errB != nil || a == "" || a == b {
		t.Fatalf("NewID = %q, %q (%v, %v)", a, b, errA, errB)
	}
}

func TestRedactStringLongestSecretFirst(t *testing.T) {
	secrets := []string{"abc123", "abc123def"}
	if got := RedactString("x abc123def y abc123", secrets); got != "x •••• y ••••" {
		t.Fatalf("got %q", got)
	}
	if secrets[0] != "abc123" {
		t.Fatalf("caller slice mutated: %v", secrets)
	}
}
