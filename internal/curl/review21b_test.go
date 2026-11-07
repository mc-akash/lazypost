package curl

import (
	"strings"
	"testing"

	"github.com/mc-akash/lazypost/internal/httpx"
	"github.com/mc-akash/lazypost/internal/vars"
	"github.com/mc-akash/lazypost/internal/workspace"
)

func exportResolved(req workspace.Request) string {
	out, _ := httpx.Resolve(req, workspace.Auth{Type: "none"}, vars.NewResolver())
	return Export(out)
}

func TestExportAddsJSONContentTypeLikeSend(t *testing.T) {
	req := workspace.Request{Method: "POST", URL: "https://a.io", Body: workspace.Body{Type: "json", Content: `{"a":1}`}}
	if got := exportResolved(req); !strings.Contains(got, `-H 'Content-Type: application/json'`) {
		t.Fatalf("export = %s", got)
	}

	req.Headers = []workspace.KV{{Key: "content-type", Value: "application/vnd.api+json"}}
	got := exportResolved(req)
	if strings.Count(strings.ToLower(got), "content-type") != 1 || !strings.Contains(got, "application/vnd.api+json") {
		t.Fatalf("explicit header must win: %s", got)
	}
}

func TestImportCombinedShortFlags(t *testing.T) {
	cases := []struct{ cmd, method string }{
		{"curl -sSL https://a.io/x", "GET"},
		{"curl -kLv https://a.io/x", "GET"},
		{"curl -sXPOST https://a.io/x", "POST"},
		{"curl -sX PUT https://a.io/x", "PUT"},
	}
	for _, c := range cases {
		got, err := Import(c.cmd)
		if err != nil || got.Method != c.method || got.URL != "https://a.io/x" {
			t.Errorf("%s: %+v, %v", c.cmd, got, err)
		}
	}
	if _, err := Import("curl -sSo out https://a.io/x"); err == nil || !strings.Contains(err.Error(), "-o") {
		t.Errorf("unsupported letter: err = %v", err)
	}
}

func TestImportRefusesDataFromFile(t *testing.T) {
	for _, flag := range []string{"-d", "--data", "--data-binary", "--data-ascii"} {
		_, err := Import("curl https://a.io " + flag + " @body.json")
		if err == nil || !strings.Contains(err.Error(), flag+" @body.json") {
			t.Errorf("%s: err = %v", flag, err)
		}
	}
	if _, err := Import("curl https://a.io -d@body.json"); err == nil || !strings.Contains(err.Error(), "-d") {
		t.Errorf("-d@file: err = %v", err)
	}
	got, err := Import("curl https://a.io --data-raw @literal")
	if err != nil || got.Body.Content != "@literal" {
		t.Errorf("--data-raw keeps @ literally: %+v, %v", got.Body, err)
	}
}
