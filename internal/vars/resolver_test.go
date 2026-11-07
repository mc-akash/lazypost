package vars

import (
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mc-akash/lazypost/internal/workspace"
)

func TestSourceString(t *testing.T) {
	want := map[Source]string{
		SrcGlobal: "global", SrcShared: "shared", SrcCollection: "collection",
		SrcEnv: "env", SrcSecret: "secret", SrcBuiltin: "builtin",
	}
	for s, w := range want {
		if got := s.String(); got != w {
			t.Errorf("%d.String() = %q, want %q", s, got, w)
		}
	}
	if got := Source(99).String(); got != "unknown" {
		t.Errorf("Source(99).String() = %q", got)
	}
}

func TestPrecedence(t *testing.T) {
	r := NewResolver(
		Layer{SrcGlobal, map[string]string{"a": "g", "b": "g", "c": "g", "d": "g", "e": "g"}},
		Layer{SrcShared, map[string]string{"b": "sh", "c": "sh", "d": "sh", "e": "sh"}},
		Layer{SrcCollection, map[string]string{"c": "co", "d": "co", "e": "co"}},
		Layer{SrcEnv, map[string]string{"d": "en", "e": "en"}},
		Layer{SrcSecret, map[string]string{"e": "se"}})
	cases := []struct {
		name, val string
		src       Source
	}{
		{"a", "g", SrcGlobal}, {"b", "sh", SrcShared}, {"c", "co", SrcCollection},
		{"d", "en", SrcEnv}, {"e", "se", SrcSecret},
	}
	for _, c := range cases {
		v, src, ok := r.Lookup(c.name)
		if !ok || v != c.val || src != c.src {
			t.Errorf("Lookup(%q) = %q,%v,%v; want %q,%v,true", c.name, v, src, ok, c.val, c.src)
		}
	}
	if _, _, ok := r.Lookup("zzz"); ok {
		t.Error("Lookup(zzz) ok = true")
	}
}

func TestExpandMissingAndSpaces(t *testing.T) {
	r := NewResolver(Layer{SrcEnv, map[string]string{"base": "http://x", "id": "7"}})
	out, missing := r.Expand("{{ base }}/u/{{id}}/{{nope}}/{{nope}}")
	if out != "http://x/u/7/{{nope}}/{{nope}}" {
		t.Errorf("out = %q", out)
	}
	if !reflect.DeepEqual(missing, []string{"nope"}) {
		t.Errorf("missing = %v", missing)
	}
	out, missing = r.Expand("plain text")
	if out != "plain text" || missing != nil {
		t.Errorf("plain: %q %v", out, missing)
	}
}

func TestExpandMissingOrder(t *testing.T) {
	r := NewResolver(Layer{SrcEnv, map[string]string{"n": "{{inner}}"}})
	_, missing := r.Expand("{{z}} {{n}} {{a}} {{z}}")
	if !reflect.DeepEqual(missing, []string{"z", "inner", "a"}) {
		t.Errorf("missing = %v", missing)
	}
}

func TestExpandNestedAndCycle(t *testing.T) {
	r := NewResolver(Layer{SrcEnv, map[string]string{"base_url": "https://{{host}}", "host": "api.io"}})
	if out, _ := r.Expand("{{base_url}}"); out != "https://api.io" {
		t.Errorf("nested = %q", out)
	}

	r = NewResolver(Layer{SrcEnv, map[string]string{"a": "{{b}}", "b": "{{a}}"}})
	done := make(chan string, 1)
	go func() { out, _ := r.Expand("{{a}}"); done <- out }()
	select {
	case out := <-done:
		if !strings.Contains(out, "{{") {
			t.Errorf("cycle out = %q", out)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Expand hung on cycle")
	}

	self := NewResolver(Layer{SrcEnv, map[string]string{"s": "x{{s}}"}})
	if out, missing := self.Expand("{{s}}"); out != "x{{s}}" || !reflect.DeepEqual(missing, []string{"s"}) {
		t.Errorf("self cycle = %q %v", out, missing)
	}
}

func TestExpandDepthOverflow(t *testing.T) {

	m := map[string]string{}
	for i := 0; i < 9; i++ {
		m["v"+strconv.Itoa(i)] = "{{v" + strconv.Itoa(i+1) + "}}"
	}
	m["v9"] = "end"
	r := NewResolver(Layer{SrcEnv, m})
	out, missing := r.Expand("{{v0}}")
	if !strings.Contains(out, "{{") || strings.Contains(out, "end") {
		t.Errorf("depth overflow out = %q", out)
	}
	if !reflect.DeepEqual(missing, []string{"v5"}) {
		t.Errorf("missing = %v", missing)
	}

	short := NewResolver(Layer{SrcEnv, map[string]string{"a": "{{b}}", "b": "{{c}}", "c": "ok"}})
	if out, _ := short.Expand("{{a}}"); out != "ok" {
		t.Errorf("short chain = %q", out)
	}
}

func TestBuiltins(t *testing.T) {
	r := NewResolver()
	ts, src, ok := r.Lookup("$timestamp")
	if !ok || src != SrcBuiltin || !regexp.MustCompile(`^\d{10}$`).MatchString(ts) {
		t.Errorf("$timestamp = %q %v %v", ts, src, ok)
	}
	uuidRe := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	u, _, _ := r.Lookup("$uuid")
	if !uuidRe.MatchString(u) {
		t.Errorf("$uuid = %q", u)
	}
	g1, _, _ := r.Lookup("$guid")
	g2, _, _ := r.Lookup("$guid")
	if g1 == g2 || !uuidRe.MatchString(g1) {
		t.Errorf("$guid %q %q", g1, g2)
	}
	for i := 0; i < 50; i++ {
		s, _, _ := r.Lookup("$randomInt")
		n, err := strconv.Atoi(s)
		if err != nil || n < 0 || n > 1000 {
			t.Fatalf("$randomInt = %q", s)
		}
	}
	if _, _, ok := r.Lookup("$nope"); ok {
		t.Error("$nope ok")
	}
}

func TestBuiltinsFixedClock(t *testing.T) {
	r := NewResolver()
	r.now = func() time.Time { return time.Date(2026, 11, 7, 12, 0, 0, 0, time.FixedZone("x", 3600)) }
	out, missing := r.Expand("{{$timestamp}} {{ $isoTimestamp }} {{$bogus}}")
	if out != "1794049200 2026-11-07T11:00:00Z {{$bogus}}" {
		t.Errorf("out = %q", out)
	}
	if !reflect.DeepEqual(missing, []string{"$bogus"}) {
		t.Errorf("missing = %v", missing)
	}
}

func TestForRequestLayers(t *testing.T) {
	root := &workspace.Folder{Vars: map[string]string{"x": "root", "r": "root"}}
	outer := &workspace.Folder{Name: "outer", Parent: root, Vars: map[string]string{"x": "outer", "o": "outer"}}
	inner := &workspace.Folder{Name: "inner", Parent: outer, Vars: map[string]string{"x": "inner"}}
	req := &workspace.Request{Name: "r", Parent: inner}
	ws := &workspace.Workspace{
		Globals:       map[string]string{"g": "glob", "x": "glob"},
		SharedSecrets: map[string]string{"s": "shh"},
		Collections:   root,
	}

	r := ForRequest(ws, nil, req)
	for name, want := range map[string]string{"x": "inner", "o": "outer", "r": "root", "g": "glob", "s": "shh"} {
		if v, _, ok := r.Lookup(name); !ok || v != want {
			t.Errorf("Lookup(%q) = %q,%v; want %q", name, v, ok, want)
		}
	}
	if _, src, _ := r.Lookup("x"); src != SrcCollection {
		t.Errorf("x src = %v", src)
	}

	env := &workspace.Env{Name: "dev", Vars: map[string]string{"x": "env"}, Secrets: map[string]string{"tok": "t0p"}}
	r = ForRequest(ws, env, req)
	if v, src, _ := r.Lookup("x"); v != "env" || src != SrcEnv {
		t.Errorf("x with env = %q %v", v, src)
	}
	if got := r.SecretValues(); !reflect.DeepEqual(got, []string{"shh", "t0p"}) {
		t.Errorf("SecretValues = %v", got)
	}

	r = ForRequest(ws, env, nil)
	if v, _, _ := r.Lookup("x"); v != "env" {
		t.Errorf("nil req x = %q", v)
	}
	if _, _, ok := ForRequest(nil, nil, nil).Lookup("x"); ok {
		t.Error("empty resolver found x")
	}
}

func TestSecretValues(t *testing.T) {
	r := NewResolver(
		Layer{SrcGlobal, map[string]string{"g": "public"}},
		Layer{SrcShared, map[string]string{"a": "s1", "e": ""}},
		Layer{SrcSecret, map[string]string{"b": "s2", "c": "s1"}})
	if got := r.SecretValues(); !reflect.DeepEqual(got, []string{"s1", "s2"}) {
		t.Errorf("SecretValues = %v", got)
	}
}

func TestRequestRefs(t *testing.T) {
	req := workspace.Request{
		URL:     "{{base}}/u/{{id}}",
		Params:  []workspace.KV{{Key: "{{pk}}", Value: "{{id}}"}},
		Headers: []workspace.KV{{Key: "X-Trace", Value: "{{trace}}", Disabled: true}},
		Auth: workspace.Auth{Type: "bearer", Token: "{{token}}", Username: "{{user}}", Password: "{{pass}}",
			Key: "{{akey}}", Value: "{{aval}}"},
		Body: workspace.Body{
			Content: `{"x":"{{bodyv}}","id":"{{id}}"}`,
			Form:    []workspace.KV{{Key: "f", Value: "{{formv}}"}},
			Parts:   []workspace.Part{{Key: "p", Value: "{{partv}}"}},
		},
	}
	want := []string{"base", "id", "pk", "trace", "token", "user", "pass", "akey", "aval", "bodyv", "formv", "partv"}
	if got := RequestRefs(req); !reflect.DeepEqual(got, want) {
		t.Errorf("RequestRefs = %v\nwant %v", got, want)
	}
}
