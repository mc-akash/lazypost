package curl

import (
	"reflect"
	"strings"
	"testing"

	"github.com/mc-akash/lazypost/internal/workspace"
)

func TestExportJSON(t *testing.T) {
	req := workspace.Request{
		Method:  "POST",
		URL:     "https://a.io/u",
		Params:  []workspace.KV{{Key: "x", Value: "1"}},
		Headers: []workspace.KV{{Key: "Content-Type", Value: "application/json"}},
		Body:    workspace.Body{Type: "json", Content: `{"a":1}`},
	}
	want := `curl -X 'POST' 'https://a.io/u?x=1' -H 'Content-Type: application/json' --data-raw '{"a":1}'`
	if got := Export(req); got != want {
		t.Fatalf("Export:\n got %s\nwant %s", got, want)
	}
}

func TestExportGETOmitsMethod(t *testing.T) {
	got := Export(workspace.Request{Method: "GET", URL: "https://a.io"})
	if want := `curl 'https://a.io'`; got != want {
		t.Fatalf("got %s, want %s", got, want)
	}
}

func TestExportAppendsParamsToExistingQuery(t *testing.T) {
	req := workspace.Request{
		Method: "GET",
		URL:    "https://a.io/s?a=1",
		Params: []workspace.KV{{Key: "q", Value: "a b&c"}, {Key: "z", Value: "2"}},
	}
	if got, want := Export(req), `curl 'https://a.io/s?a=1&q=a+b%26c&z=2'`; got != want {
		t.Fatalf("got %s, want %s", got, want)
	}
}

func TestExportQuotesSingleQuote(t *testing.T) {
	req := workspace.Request{
		Method:  "GET",
		URL:     "https://a.io",
		Headers: []workspace.KV{{Key: "X", Value: "it's"}},
	}
	got := Export(req)
	if !strings.Contains(got, `-H 'X: it'\''s'`) {
		t.Fatalf("missing escaped header in %s", got)
	}
}

func TestExportForm(t *testing.T) {
	req := workspace.Request{
		Method: "POST",
		URL:    "https://a.io",
		Body: workspace.Body{Type: "form", Form: []workspace.KV{
			{Key: "a", Value: "1"}, {Key: "b", Value: "x y"},
		}},
	}
	want := `curl -X 'POST' 'https://a.io' --data-urlencode 'a=1' --data-urlencode 'b=x y'`
	if got := Export(req); got != want {
		t.Fatalf("got %s\nwant %s", got, want)
	}
}

func TestExportMultipart(t *testing.T) {
	req := workspace.Request{
		Method: "POST",
		URL:    "https://a.io/up",
		Body: workspace.Body{Type: "multipart", Parts: []workspace.Part{
			{Key: "name", Value: "bob"},
			{Key: "file", Value: "/tmp/a.png", File: true},
		}},
	}
	got := Export(req)
	for _, s := range []string{`--form-string 'name=bob'`, `-F 'file=@/tmp/a.png'`} {
		if !strings.Contains(got, s) {
			t.Fatalf("missing %s in %s", s, got)
		}
	}
}

func TestImportRoundTrip(t *testing.T) {
	reqs := []workspace.Request{
		{
			Method:  "POST",
			URL:     "https://a.io/u",
			Params:  []workspace.KV{{Key: "x", Value: "1"}, {Key: "q", Value: "a b"}},
			Headers: []workspace.KV{{Key: "Content-Type", Value: "application/json"}, {Key: "X", Value: "it's"}},
			Body:    workspace.Body{Type: "json", Content: `{"a":"it's"}`},
		},
		{
			Method: "PUT",
			URL:    "https://a.io/f",
			Body:   workspace.Body{Type: "form", Form: []workspace.KV{{Key: "a", Value: "1"}}},
		},
		{
			Method: "POST",
			URL:    "https://a.io/up",
			Body: workspace.Body{Type: "multipart", Parts: []workspace.Part{
				{Key: "n", Value: "v"}, {Key: "f", Value: "/tmp/a b.png", File: true},
				{Key: "at", Value: "@not-a-file"}, {Key: "lt", Value: "<not-a-file"},
			}},
		},
		{
			Method:  "POST",
			URL:     "https://a.io/raw",
			Headers: []workspace.KV{{Key: "Content-Type", Value: "text/plain"}},
			Body:    workspace.Body{Type: "raw", Content: "a=1&b=2"},
		},
		{Method: "GET", URL: "https://a.io/plain"},
	}
	for _, r := range reqs {
		got, err := Import(Export(r))
		if err != nil {
			t.Fatalf("Import(%s): %v", Export(r), err)
		}
		if got.Method != r.Method || got.URL != r.URL {
			t.Errorf("method/url: got %s %s, want %s %s", got.Method, got.URL, r.Method, r.URL)
		}
		if !reflect.DeepEqual(got.Params, r.Params) {
			t.Errorf("params: got %+v, want %+v", got.Params, r.Params)
		}
		if !reflect.DeepEqual(got.Headers, r.Headers) {
			t.Errorf("headers: got %+v, want %+v", got.Headers, r.Headers)
		}
		wantBody := r.Body
		if wantBody.Type == "" {
			wantBody.Type = "none"
		}
		if !reflect.DeepEqual(got.Body, wantBody) {
			t.Errorf("body: got %+v, want %+v", got.Body, wantBody)
		}
	}
}

func TestImportCases(t *testing.T) {
	tests := []struct {
		name string
		cmd  string
		want workspace.Request
	}{
		{
			name: "bare url",
			cmd:  "curl https://a.io",
			want: workspace.Request{Name: "GET /", Method: "GET", URL: "https://a.io", Body: workspace.Body{Type: "none"}},
		},
		{
			name: "without curl prefix",
			cmd:  "https://a.io/users/42?x=1&y=two%20words",
			want: workspace.Request{
				Name: "GET /users/42", Method: "GET", URL: "https://a.io/users/42",
				Params: []workspace.KV{{Key: "x", Value: "1"}, {Key: "y", Value: "two words"}},
				Body:   workspace.Body{Type: "none"},
			},
		},
		{
			name: "data implies POST raw",
			cmd:  "curl -d 'a=1' https://a.io",
			want: workspace.Request{
				Name: "POST /", Method: "POST", URL: "https://a.io",
				Headers: []workspace.KV{{Key: "Content-Type", Value: "application/x-www-form-urlencoded"}},
				Body:    workspace.Body{Type: "raw", Content: "a=1"},
			},
		},
		{
			name: "urlencoded content type stays raw",
			cmd:  "curl -H 'Content-Type: application/x-www-form-urlencoded' --data 'a=1' https://a.io",
			want: workspace.Request{
				Name: "POST /", Method: "POST", URL: "https://a.io",
				Headers: []workspace.KV{{Key: "Content-Type", Value: "application/x-www-form-urlencoded"}},
				Body:    workspace.Body{Type: "raw", Content: "a=1"},
			},
		},
		{
			name: "json detected from leading brace",
			cmd:  `curl https://a.io/x --data-binary '{"a":1}'`,
			want: workspace.Request{Name: "POST /x", Method: "POST", URL: "https://a.io/x", Body: workspace.Body{Type: "json", Content: `{"a":1}`}},
		},
		{
			name: "json detected from content type",
			cmd:  `curl -X PATCH -H 'content-type: application/vnd.api+json' --data-ascii 'null' --url https://a.io/x`,
			want: workspace.Request{
				Name: "PATCH /x", Method: "PATCH", URL: "https://a.io/x",
				Headers: []workspace.KV{{Key: "content-type", Value: "application/vnd.api+json"}},
				Body:    workspace.Body{Type: "json", Content: "null"},
			},
		},
		{
			name: "multiline continuations",
			cmd:  "curl --request put \\\n  --header 'A: b' \\\n  https://a.io/z",
			want: workspace.Request{
				Name: "PUT /z", Method: "PUT", URL: "https://a.io/z",
				Headers: []workspace.KV{{Key: "A", Value: "b"}},
				Body:    workspace.Body{Type: "none"},
			},
		},
		{
			name: "basic auth",
			cmd:  "curl -u bob:pw https://a.io",
			want: workspace.Request{
				Name: "GET /", Method: "GET", URL: "https://a.io",
				Auth: workspace.Auth{Type: "basic", Username: "bob", Password: "pw"},
				Body: workspace.Body{Type: "none"},
			},
		},
		{
			name: "multipart file",
			cmd:  "curl -F 'f=@x.png' --form 'n=v' https://a.io",
			want: workspace.Request{
				Name: "POST /", Method: "POST", URL: "https://a.io",
				Body: workspace.Body{Type: "multipart", Parts: []workspace.Part{
					{Key: "f", Value: "x.png", File: true}, {Key: "n", Value: "v"},
				}},
			},
		},
		{
			name: "data-urlencode form rows",
			cmd:  "curl --data-urlencode 'a=1' --data-urlencode 'b=x y' https://a.io",
			want: workspace.Request{
				Name: "POST /", Method: "POST", URL: "https://a.io",
				Body: workspace.Body{Type: "form", Form: []workspace.KV{{Key: "a", Value: "1"}, {Key: "b", Value: "x y"}}},
			},
		},
		{
			name: "ignored flags",
			cmd:  "curl -L -s --compressed -S -k -i -v --location --silent --insecure https://a.io",
			want: workspace.Request{Name: "GET /", Method: "GET", URL: "https://a.io", Body: workspace.Body{Type: "none"}},
		},
		{
			name: "chrome copy as curl",
			cmd:  `curl 'https://api.io/v1/users?page=2' -H 'accept: application/json' -b 'sid=abc' -H 'user-agent: Mozilla/5.0' --data-raw $'{"name":"O\'Neil"}' --compressed`,
			want: workspace.Request{
				Name: "POST /v1/users", Method: "POST", URL: "https://api.io/v1/users",
				Params: []workspace.KV{{Key: "page", Value: "2"}},
				Headers: []workspace.KV{
					{Key: "accept", Value: "application/json"},
					{Key: "Cookie", Value: "sid=abc"},
					{Key: "user-agent", Value: "Mozilla/5.0"},
				},
				Body: workspace.Body{Type: "json", Content: `{"name":"O'Neil"}`},
			},
		},
		{
			name: "cookie user-agent referer flags",
			cmd:  "curl --cookie 'a=1; b=2' -A 'agent/1' --user-agent=ua2 -e https://ref.io --referer r2 https://a.io",
			want: workspace.Request{
				Name: "GET /", Method: "GET", URL: "https://a.io",
				Headers: []workspace.KV{
					{Key: "Cookie", Value: "a=1; b=2"},
					{Key: "User-Agent", Value: "agent/1"},
					{Key: "User-Agent", Value: "ua2"},
					{Key: "Referer", Value: "https://ref.io"},
					{Key: "Referer", Value: "r2"},
				},
				Body: workspace.Body{Type: "none"},
			},
		},
		{
			name: "ansi-c escapes",
			cmd:  `curl https://a.io -d $'l1\nl2\t\r\\ \" \x41\u00e9 \q' -H $'X: it\'s'`,
			want: workspace.Request{
				Name: "POST /", Method: "POST", URL: "https://a.io",
				Headers: []workspace.KV{{Key: "X", Value: "it's"}, {Key: "Content-Type", Value: "application/x-www-form-urlencoded"}},
				Body:    workspace.Body{Type: "raw", Content: "l1\nl2\t\r\\ \" Aé \\q"},
			},
		},
		{
			name: "dollar quote inside quotes is literal",
			cmd:  `curl https://a.io -d '$'"'"'x' -H "A: $'b'"`,
			want: workspace.Request{
				Name: "POST /", Method: "POST", URL: "https://a.io",
				Headers: []workspace.KV{{Key: "A", Value: "$'b'"}, {Key: "Content-Type", Value: "application/x-www-form-urlencoded"}},
				Body:    workspace.Body{Type: "raw", Content: "$'x"},
			},
		},
		{
			name: "browser ignored flags",
			cmd:  "curl --http2 --http1.1 --http2-prior-knowledge --globoff -g --fail -f https://a.io",
			want: workspace.Request{Name: "GET /", Method: "GET", URL: "https://a.io", Body: workspace.Body{Type: "none"}},
		},
		{
			name: "explicit GET with data keeps GET",
			cmd:  "curl -X GET -d 'q' https://a.io",
			want: workspace.Request{
				Name: "GET /", Method: "GET", URL: "https://a.io",
				Headers: []workspace.KV{{Key: "Content-Type", Value: "application/x-www-form-urlencoded"}},
				Body:    workspace.Body{Type: "raw", Content: "q"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Import(tt.cmd)
			if err != nil {
				t.Fatalf("Import: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got  %+v\nwant %+v", got, tt.want)
			}
		})
	}
}

func TestImportErrors(t *testing.T) {
	tests := []struct {
		cmd     string
		mention string
	}{
		{cmd: "", mention: "URL"},
		{cmd: "curl -H 'a: b'", mention: "URL"},
		{cmd: "curl --proxy x https://a.io", mention: "--proxy"},
		{cmd: "curl -H", mention: "-H"},
		{cmd: "curl -H 'nocolon' https://a.io", mention: "nocolon"},
		{cmd: "curl https://a.io https://b.io", mention: "URL"},
		{cmd: "curl -d a -F b=c https://a.io", mention: "combine"},
		{cmd: "curl 'unterminated", mention: ""},
		{cmd: "curl -b cookies.txt https://a.io", mention: "-b"},
		{cmd: "curl --cookie jar https://a.io", mention: "--cookie"},
		{cmd: "curl -d $'abc https://a.io", mention: "$'"},
		{cmd: "curl -X 'GET;id' https://a.io", mention: "invalid method"},
		{cmd: "curl -X '' https://a.io", mention: "invalid method"},
	}
	for _, tt := range tests {
		_, err := Import(tt.cmd)
		if err == nil {
			t.Errorf("Import(%q): expected error", tt.cmd)
			continue
		}
		if !strings.Contains(err.Error(), tt.mention) {
			t.Errorf("Import(%q) error %q does not mention %q", tt.cmd, err, tt.mention)
		}
	}
}

func TestExportQuotesMethod(t *testing.T) {
	got := Export(workspace.Request{Method: "PROPFIND", URL: "https://a.io"})
	if want := `curl -X 'PROPFIND' 'https://a.io'`; got != want {
		t.Fatalf("got %s, want %s", got, want)
	}

	got = Export(workspace.Request{Method: `GET;$'\143'`, URL: "https://a.io"})
	if want := `curl -X 'GET;$'\''\143'\''' 'https://a.io'`; got != want {
		t.Fatalf("got %s, want %s", got, want)
	}
}
