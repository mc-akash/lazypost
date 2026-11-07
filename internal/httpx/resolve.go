package httpx

import (
	"encoding/base64"
	"strings"

	"github.com/mc-akash/lazypost/internal/vars"
	"github.com/mc-akash/lazypost/internal/workspace"
)

const authorizationHeader = "Authorization"

func EffectiveAuth(req *workspace.Request, chain []*workspace.Folder) workspace.Auth {
	if req.Auth.Type != "" {
		return req.Auth
	}
	for i := len(chain) - 1; i >= 0; i-- {
		if chain[i] != nil && chain[i].Auth.Type != "" {
			return chain[i].Auth
		}
	}
	return workspace.Auth{Type: "none"}
}

func Resolve(req workspace.Request, auth workspace.Auth, r *vars.Resolver) (out workspace.Request, missing []string) {
	x := expander{r: r, seen: map[string]bool{}}
	out = workspace.Request{
		Name:   req.Name,
		Method: req.Method,
		URL:    x.expand(req.URL),
		Path:   req.Path,
		Parent: req.Parent,
	}
	out.Params = x.kvs(req.Params)
	out.Headers = x.kvs(req.Headers)
	out.Auth = workspace.Auth{Type: "none"}
	applyAuth(&out, x.auth(auth))
	out.Body = workspace.Body{
		Type:    req.Body.Type,
		Content: x.expand(req.Body.Content),
		Form:    x.kvs(req.Body.Form),
		Parts:   x.parts(req.Body.Parts),
	}
	if out.Body.Type == "json" {
		addHeader(&out, contentTypeHeader, "application/json")
	}
	return out, x.missing
}

func applyAuth(out *workspace.Request, a workspace.Auth) {
	switch a.Type {
	case "bearer":
		if a.Token != "" {
			addHeader(out, authorizationHeader, "Bearer "+a.Token)
		}
	case "basic":
		if a.Username != "" || a.Password != "" {
			cred := base64.StdEncoding.EncodeToString([]byte(a.Username + ":" + a.Password))
			addHeader(out, authorizationHeader, "Basic "+cred)
		}
	case "apikey":
		if a.Key == "" || a.Value == "" {
			return
		}
		if a.In == "query" {
			out.Params = append(out.Params, workspace.KV{Key: a.Key, Value: a.Value})
			return
		}
		addHeader(out, a.Key, a.Value)
	}
}

func addHeader(out *workspace.Request, key, value string) {
	for _, h := range out.Headers {
		if strings.EqualFold(h.Key, key) {
			return
		}
	}
	out.Headers = append(out.Headers, workspace.KV{Key: key, Value: value})
}

type expander struct {
	r       *vars.Resolver
	missing []string
	seen    map[string]bool
}

func (x *expander) expand(s string) string {
	out, missing := x.r.Expand(s)
	for _, name := range missing {
		if !x.seen[name] {
			x.seen[name] = true
			x.missing = append(x.missing, name)
		}
	}
	return out
}

func (x *expander) kvs(kvs []workspace.KV) []workspace.KV {
	var out []workspace.KV
	for _, kv := range kvs {
		if kv.Disabled {
			continue
		}
		out = append(out, workspace.KV{Key: x.expand(kv.Key), Value: x.expand(kv.Value)})
	}
	return out
}

func (x *expander) parts(parts []workspace.Part) []workspace.Part {
	var out []workspace.Part
	for _, p := range parts {
		if p.Disabled {
			continue
		}
		out = append(out, workspace.Part{Key: x.expand(p.Key), Value: x.expand(p.Value), File: p.File})
	}
	return out
}

func (x *expander) auth(a workspace.Auth) workspace.Auth {
	return workspace.Auth{
		Type:     a.Type,
		Token:    x.expand(a.Token),
		Username: x.expand(a.Username),
		Password: x.expand(a.Password),
		Key:      x.expand(a.Key),
		Value:    x.expand(a.Value),
		In:       a.In,
	}
}
