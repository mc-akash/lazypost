package httpx

import (
	"testing"

	"github.com/mc-akash/lazypost/internal/workspace"
)

func TestEncodeUppercasesMethod(t *testing.T) {
	hr, err := Encode(t.Context(), workspace.Request{Method: "post", URL: "https://a.io"})
	if err != nil || hr.Method != "POST" {
		t.Fatalf("method = %v, %v", hr, err)
	}
}

func TestResolveAddsJSONContentTypeUnlessExplicit(t *testing.T) {
	req := workspace.Request{Method: "POST", URL: "https://a.io", Body: workspace.Body{Type: "json"}}
	if got := header(resolveNone(req), "Content-Type"); len(got) != 1 || got[0] != "application/json" {
		t.Fatalf("Content-Type = %v", got)
	}
	req.Headers = []workspace.KV{{Key: "content-type", Value: "text/plain"}}
	out := resolveNone(req)
	if len(out.Headers) != 1 || out.Headers[0].Value != "text/plain" {
		t.Fatalf("explicit header must win: %v", out.Headers)
	}
}
