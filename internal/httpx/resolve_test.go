package httpx

import (
	"reflect"
	"testing"

	"github.com/mc-akash/lazypost/internal/vars"
	"github.com/mc-akash/lazypost/internal/workspace"
)

func resolver(m map[string]string) *vars.Resolver {
	return vars.NewResolver(vars.Layer{Source: vars.SrcEnv, Vars: m})
}

func header(req workspace.Request, key string) []string {
	var out []string
	for _, h := range req.Headers {
		if h.Key == key {
			out = append(out, h.Value)
		}
	}
	return out
}

func TestResolveDropsDisabledAndExpands(t *testing.T) {
	req := workspace.Request{
		Method: "GET",
		URL:    "{{base}}/items",
		Params: []workspace.KV{{Key: "q", Value: "{{v}}"}, {Key: "off", Value: "x", Disabled: true}},
		Headers: []workspace.KV{
			{Key: "X-{{h}}", Value: "{{v}}"},
			{Key: "X-Off", Value: "y", Disabled: true},
		},
		Body: workspace.Body{
			Type:    "json",
			Content: `{"v":"{{v}}"}`,
			Form:    []workspace.KV{{Key: "f", Value: "{{v}}"}, {Key: "g", Disabled: true}},
			Parts: []workspace.Part{
				{Key: "p", Value: "{{v}}"},
				{Key: "file", Value: "{{dir}}/a.txt", File: true},
				{Key: "gone", Disabled: true},
			},
		},
	}
	out, missing := Resolve(req, workspace.Auth{Type: "none"}, resolver(map[string]string{
		"base": "http://x", "v": "1", "h": "Id", "dir": "/tmp",
	}))
	if missing != nil {
		t.Fatalf("missing = %v", missing)
	}
	if out.URL != "http://x/items" {
		t.Errorf("URL = %q", out.URL)
	}
	if want := []workspace.KV{{Key: "q", Value: "1"}}; !reflect.DeepEqual(out.Params, want) {
		t.Errorf("Params = %v", out.Params)
	}
	if want := []workspace.KV{{Key: "X-Id", Value: "1"}, {Key: "Content-Type", Value: "application/json"}}; !reflect.DeepEqual(out.Headers, want) {
		t.Errorf("Headers = %v", out.Headers)
	}
	if out.Body.Content != `{"v":"1"}` {
		t.Errorf("Content = %q", out.Body.Content)
	}
	if want := []workspace.KV{{Key: "f", Value: "1"}}; !reflect.DeepEqual(out.Body.Form, want) {
		t.Errorf("Form = %v", out.Body.Form)
	}
	wantParts := []workspace.Part{{Key: "p", Value: "1"}, {Key: "file", Value: "/tmp/a.txt", File: true}}
	if !reflect.DeepEqual(out.Body.Parts, wantParts) {
		t.Errorf("Parts = %v", out.Body.Parts)
	}
	if out.Auth.Type != "none" {
		t.Errorf("Auth = %v", out.Auth)
	}

	if req.Params[0].Value != "{{v}}" || len(req.Params) != 2 {
		t.Errorf("input mutated: %v", req.Params)
	}

	hr, err := Encode(t.Context(), out)
	if err != nil {
		t.Fatal(err)
	}
	if hr.URL.RawQuery != "q=1" {
		t.Errorf("RawQuery = %q", hr.URL.RawQuery)
	}
}

func TestResolveAuthKinds(t *testing.T) {
	r := resolver(map[string]string{"token": "abc", "user": "u", "pw": "p", "key": "val"})
	base := workspace.Request{Method: "GET", URL: "http://x"}

	cases := []struct {
		name       string
		auth       workspace.Auth
		header     string
		wantHeader []string
		wantParams []workspace.KV
	}{
		{"bearer", workspace.Auth{Type: "bearer", Token: "{{token}}"}, "Authorization", []string{"Bearer abc"}, nil},
		{"basic", workspace.Auth{Type: "basic", Username: "{{user}}", Password: "{{pw}}"}, "Authorization", []string{"Basic dTpw"}, nil},
		{"apikey query", workspace.Auth{Type: "apikey", Key: "X", Value: "{{key}}", In: "query"}, "X", nil, []workspace.KV{{Key: "X", Value: "val"}}},
		{"apikey header default", workspace.Auth{Type: "apikey", Key: "X", Value: "{{key}}"}, "X", []string{"val"}, nil},
		{"apikey header explicit", workspace.Auth{Type: "apikey", Key: "X", Value: "{{key}}", In: "header"}, "X", []string{"val"}, nil},
		{"none", workspace.Auth{Type: "none", Token: "abc"}, "Authorization", nil, nil},
		{"empty bearer", workspace.Auth{Type: "bearer"}, "Authorization", nil, nil},
		{"empty basic", workspace.Auth{Type: "basic"}, "Authorization", nil, nil},
		{"empty apikey", workspace.Auth{Type: "apikey", Key: "X"}, "X", nil, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := Resolve(base, tc.auth, r)
			if got := header(out, tc.header); !reflect.DeepEqual(got, tc.wantHeader) {
				t.Errorf("header %s = %v, want %v", tc.header, got, tc.wantHeader)
			}
			if !reflect.DeepEqual(out.Params, tc.wantParams) {
				t.Errorf("Params = %v, want %v", out.Params, tc.wantParams)
			}
			if out.Auth.Type != "none" {
				t.Errorf("out.Auth = %v", out.Auth)
			}
		})
	}
}

func TestResolveExplicitHeaderWinsOverAuth(t *testing.T) {
	r := resolver(nil)
	req := workspace.Request{URL: "http://x", Headers: []workspace.KV{{Key: "authorization", Value: "Custom 1"}}}
	out, _ := Resolve(req, workspace.Auth{Type: "bearer", Token: "abc"}, r)
	if want := []workspace.KV{{Key: "authorization", Value: "Custom 1"}}; !reflect.DeepEqual(out.Headers, want) {
		t.Errorf("Headers = %v", out.Headers)
	}

	req = workspace.Request{URL: "http://x", Headers: []workspace.KV{{Key: "X-Key", Value: "mine"}}}
	out, _ = Resolve(req, workspace.Auth{Type: "apikey", Key: "x-key", Value: "theirs"}, r)
	if want := []workspace.KV{{Key: "X-Key", Value: "mine"}}; !reflect.DeepEqual(out.Headers, want) {
		t.Errorf("Headers = %v", out.Headers)
	}

	req = workspace.Request{URL: "http://x", Headers: []workspace.KV{{Key: "Authorization", Value: "x", Disabled: true}}}
	out, _ = Resolve(req, workspace.Auth{Type: "bearer", Token: "abc"}, r)
	if want := []workspace.KV{{Key: "Authorization", Value: "Bearer abc"}}; !reflect.DeepEqual(out.Headers, want) {
		t.Errorf("Headers = %v", out.Headers)
	}
}

func TestEffectiveAuthInherit(t *testing.T) {
	root := &workspace.Folder{Auth: workspace.Auth{Type: "basic", Username: "root"}}
	mid := &workspace.Folder{Parent: root}
	inner := &workspace.Folder{Parent: mid, Auth: workspace.Auth{Type: "bearer", Token: "folder"}}
	chain := []*workspace.Folder{root, mid, inner}

	req := &workspace.Request{}
	if got := EffectiveAuth(req, chain); got != inner.Auth {
		t.Errorf("inherit = %v, want %v", got, inner.Auth)
	}
	if got := EffectiveAuth(req, []*workspace.Folder{root, mid}); got != root.Auth {
		t.Errorf("inherit outer = %v, want %v", got, root.Auth)
	}
	req.Auth = workspace.Auth{Type: "none"}
	if got := EffectiveAuth(req, chain); got.Type != "none" {
		t.Errorf("none override = %v", got)
	}
	if got := EffectiveAuth(&workspace.Request{}, nil); got != (workspace.Auth{Type: "none"}) {
		t.Errorf("default = %v", got)
	}
}

func TestResolveReportsMissing(t *testing.T) {
	req := workspace.Request{
		URL:     "http://x/{{trace_id}}",
		Headers: []workspace.KV{{Key: "X-Trace", Value: "{{trace_id}}"}},
	}
	_, missing := Resolve(req, workspace.Auth{Type: "bearer", Token: "{{trace_id}}"}, resolver(nil))
	if want := []string{"trace_id"}; !reflect.DeepEqual(missing, want) {
		t.Errorf("missing = %v, want %v", missing, want)
	}

	req = workspace.Request{
		URL:    "http://x/{{a}}",
		Params: []workspace.KV{{Key: "{{b}}", Value: "{{a}}"}, {Key: "{{off}}", Disabled: true}},
		Body:   workspace.Body{Type: "multipart", Parts: []workspace.Part{{Key: "f", Value: "{{c}}", File: true}}},
	}
	_, missing = Resolve(req, workspace.Auth{Type: "basic", Username: "{{d}}"}, resolver(nil))
	if want := []string{"a", "b", "d", "c"}; !reflect.DeepEqual(missing, want) {
		t.Errorf("missing = %v, want %v", missing, want)
	}
}
