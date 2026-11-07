package httpx

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mc-akash/lazypost/internal/workspace"
)

type echo struct {
	method string
	url    string
	header http.Header
	body   string
}

func echoServer(t *testing.T) (*httptest.Server, <-chan echo) {
	t.Helper()
	ch := make(chan echo, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		ch <- echo{r.Method, r.URL.String(), r.Header.Clone(), string(b)}
	}))
	t.Cleanup(srv.Close)
	return srv, ch
}

func sendTo(t *testing.T, req workspace.Request) *http.Request {
	t.Helper()
	hr, err := Encode(t.Context(), req)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	return hr
}

func resolveNone(req workspace.Request) workspace.Request {
	out, _ := Resolve(req, workspace.Auth{Type: "none"}, resolver(nil))
	return out
}

func TestEncodeJSONSetsContentType(t *testing.T) {
	srv, ch := echoServer(t)
	c := NewClient(Options{Timeout: 5 * time.Second})

	req := workspace.Request{
		Method:  "POST",
		URL:     srv.URL + "/p?a=1",
		Params:  []workspace.KV{{Key: "b", Value: "x y"}},
		Headers: []workspace.KV{{Key: "X-Test", Value: "1"}},
		Body:    workspace.Body{Type: "json", Content: `{"a":1}`},
	}
	if _, err := Send(t.Context(), c, sendTo(t, resolveNone(req))); err != nil {
		t.Fatal(err)
	}
	got := <-ch
	if got.method != "POST" || got.url != "/p?a=1&b=x+y" {
		t.Errorf("method/url = %s %s", got.method, got.url)
	}
	if ct := got.header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q", ct)
	}
	if got.header.Get("X-Test") != "1" || got.body != `{"a":1}` {
		t.Errorf("header/body = %v %q", got.header, got.body)
	}

	req.Headers = []workspace.KV{{Key: "content-type", Value: "application/vnd.api+json"}}
	if _, err := Send(t.Context(), c, sendTo(t, resolveNone(req))); err != nil {
		t.Fatal(err)
	}
	got = <-ch
	if ct := got.header.Values("Content-Type"); len(ct) != 1 || ct[0] != "application/vnd.api+json" {
		t.Errorf("Content-Type = %v", ct)
	}
}

func TestEncodeRawAndNone(t *testing.T) {
	srv, ch := echoServer(t)
	c := NewClient(Options{})
	req := workspace.Request{URL: srv.URL, Body: workspace.Body{Type: "raw", Content: "plain text"}}
	if _, err := Send(t.Context(), c, sendTo(t, req)); err != nil {
		t.Fatal(err)
	}
	got := <-ch
	if got.method != "GET" || got.body != "plain text" || got.header.Get("Content-Type") != "" {
		t.Errorf("raw = %+v", got)
	}

	req = workspace.Request{Method: "DELETE", URL: srv.URL, Body: workspace.Body{Type: "none", Content: "ignored"}}
	if _, err := Send(t.Context(), c, sendTo(t, req)); err != nil {
		t.Fatal(err)
	}
	if got = <-ch; got.method != "DELETE" || got.body != "" {
		t.Errorf("none = %+v", got)
	}
}

func TestEncodeInvalidURL(t *testing.T) {
	for _, u := range []string{"://bad", "", "no-scheme-path", "http://"} {
		if _, err := Encode(t.Context(), workspace.Request{URL: u}); err == nil {
			t.Errorf("Encode(%q) = nil error", u)
		}
	}
}

func TestEncodeForm(t *testing.T) {
	var form map[string][]string
	var ct string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ct = r.Header.Get("Content-Type")
		if err := r.ParseForm(); err != nil {
			t.Error(err)
		}
		form = r.PostForm
	}))
	defer srv.Close()

	req := workspace.Request{
		Method: "POST", URL: srv.URL,
		Body: workspace.Body{Type: "form", Form: []workspace.KV{{Key: "a", Value: "1"}, {Key: "b", Value: "2"}}},
	}
	if _, err := Send(t.Context(), NewClient(Options{}), sendTo(t, req)); err != nil {
		t.Fatal(err)
	}
	if ct != "application/x-www-form-urlencoded" {
		t.Errorf("Content-Type = %q", ct)
	}
	if len(form) != 2 || form["a"][0] != "1" || form["b"][0] != "2" {
		t.Errorf("form = %v", form)
	}
}

func TestEncodeMultipartFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "upload.txt")
	if err := os.WriteFile(path, []byte("file content"), 0o600); err != nil {
		t.Fatal(err)
	}
	type got struct{ field, file, filename string }
	ch := make(chan got, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Error(err)
			ch <- got{}
			return
		}
		f, fh, err := r.FormFile("file")
		if err != nil {
			t.Error(err)
			ch <- got{}
			return
		}
		defer f.Close()
		b, _ := io.ReadAll(f)
		ch <- got{r.FormValue("name"), string(b), fh.Filename}
	}))
	defer srv.Close()

	req := workspace.Request{
		Method: "POST", URL: srv.URL,
		Body: workspace.Body{Type: "multipart", Parts: []workspace.Part{
			{Key: "name", Value: "v"},
			{Key: "file", Value: path, File: true},
		}},
	}
	if _, err := Send(t.Context(), NewClient(Options{}), sendTo(t, req)); err != nil {
		t.Fatal(err)
	}
	if g := <-ch; g != (got{"v", "file content", "upload.txt"}) {
		t.Errorf("got %+v", g)
	}

	missing := filepath.Join(dir, "nope.bin")
	req.Body.Parts = []workspace.Part{{Key: "file", Value: missing, File: true}}
	_, err := Encode(t.Context(), req)
	if err == nil || !strings.Contains(err.Error(), missing) {
		t.Errorf("err = %v, want path %s", err, missing)
	}
}

func TestSendResponseFields(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "sid", Value: "42"})
		w.Header().Set("X-Reply", "yes")
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, "created")
	}))
	defer srv.Close()

	resp, err := Send(t.Context(), NewClient(Options{}), sendTo(t, workspace.Request{URL: srv.URL}))
	if err != nil {
		t.Fatal(err)
	}
	if resp.Code != 201 || resp.Status != "201 Created" {
		t.Errorf("code/status = %d %q", resp.Code, resp.Status)
	}
	if resp.Header.Get("X-Reply") != "yes" {
		t.Errorf("Header = %v", resp.Header)
	}
	if resp.Header.Get("Set-Cookie") != "sid=42" {
		t.Errorf("Set-Cookie = %q", resp.Header.Get("Set-Cookie"))
	}
	if string(resp.Body) != "created" || resp.Size != 7 || resp.Truncated {
		t.Errorf("Body = %q size %d trunc %v", resp.Body, resp.Size, resp.Truncated)
	}
	if resp.Duration <= 0 {
		t.Errorf("Duration = %v", resp.Duration)
	}
}

func setMaxBody(t *testing.T, n int64) {
	t.Helper()
	old := maxBodyBytes
	maxBodyBytes = n
	t.Cleanup(func() { maxBodyBytes = old })
}

func TestSendTruncatesBody(t *testing.T) {
	setMaxBody(t, 10)
	body := strings.Repeat("a", 100)

	cases := []struct {
		name     string
		handler  http.HandlerFunc
		wantSize func(int64) bool
	}{
		{"content-length", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Length", "100")
			_, _ = io.WriteString(w, body)
		}, func(n int64) bool { return n == 100 }},
		{"chunked", func(w http.ResponseWriter, r *http.Request) {
			for i := 0; i < 10; i++ {
				_, _ = io.WriteString(w, body[:10])
				w.(http.Flusher).Flush()
			}
		}, func(n int64) bool { return n >= 10 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(tc.handler)
			defer srv.Close()
			resp, err := Send(t.Context(), NewClient(Options{}), sendTo(t, workspace.Request{URL: srv.URL}))
			if err != nil {
				t.Fatal(err)
			}
			if string(resp.Body) != body[:10] || !resp.Truncated || !tc.wantSize(resp.Size) {
				t.Errorf("body %q trunc %v size %d", resp.Body, resp.Truncated, resp.Size)
			}
		})
	}
}

func TestSendStopsReadingEndlessStream(t *testing.T) {
	setMaxBody(t, 16)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		chunk := []byte(strings.Repeat("x", 1024))
		for r.Context().Err() == nil {
			if _, err := w.Write(chunk); err != nil {
				return
			}
			w.(http.Flusher).Flush()
		}
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	type result struct {
		resp *Response
		err  error
	}
	done := make(chan result, 1)
	go func() {
		resp, err := Send(ctx, NewClient(Options{}), sendTo(t, workspace.Request{URL: srv.URL}))
		done <- result{resp, err}
	}()
	select {
	case res := <-done:
		if res.err != nil {
			t.Fatal(res.err)
		}
		if len(res.resp.Body) != 16 || !res.resp.Truncated || res.resp.Size < 16 {
			t.Errorf("len %d trunc %v size %d", len(res.resp.Body), res.resp.Truncated, res.resp.Size)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Send kept reading an endless stream")
	}
}

func TestSendCancel(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	defer srv.Close()
	defer close(release)

	ctx, cancel := context.WithCancel(t.Context())
	hr, err := Encode(ctx, workspace.Request{URL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	time.AfterFunc(50*time.Millisecond, cancel)
	_, err = Send(ctx, NewClient(Options{}), hr)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}

func TestSendConnectionRefused(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	u := srv.URL
	srv.Close()
	resp, err := Send(t.Context(), NewClient(Options{Timeout: 2 * time.Second}), sendTo(t, workspace.Request{URL: u}))
	if err == nil || resp != nil {
		t.Errorf("resp %v err %v", resp, err)
	}
}

func TestRedirectLimit(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		http.Redirect(w, r, "/again", http.StatusFound)
	}))
	defer srv.Close()

	_, err := Send(t.Context(), NewClient(Options{}), sendTo(t, workspace.Request{URL: srv.URL}))
	if err == nil || !strings.Contains(err.Error(), "10 redirects") {
		t.Errorf("err = %v", err)
	}
	if n := hits.Load(); n != maxRedirects {
		t.Errorf("hits = %d, want %d", n, maxRedirects)
	}
}

func TestNewClientOptions(t *testing.T) {
	c := NewClient(Options{Timeout: 3 * time.Second, Insecure: true})
	if c.Timeout != 3*time.Second {
		t.Errorf("Timeout = %v", c.Timeout)
	}
	tr, ok := c.Transport.(*http.Transport)
	if !ok || tr.TLSClientConfig == nil || !tr.TLSClientConfig.InsecureSkipVerify {
		t.Errorf("transport not insecure: %#v", c.Transport)
	}

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	if _, err := Send(t.Context(), c, sendTo(t, workspace.Request{URL: srv.URL})); err != nil {
		t.Errorf("insecure TLS: %v", err)
	}
	if _, err := Send(t.Context(), NewClient(Options{}), sendTo(t, workspace.Request{URL: srv.URL})); err == nil {
		t.Error("secure client accepted self-signed cert")
	}
}

func TestEncodeHostHeaderSetsHost(t *testing.T) {
	hr := sendTo(t, workspace.Request{URL: "http://127.0.0.1/", Headers: []workspace.KV{{Key: "host", Value: "example.test"}}})
	if hr.Host != "example.test" || hr.Header.Get("Host") != "" {
		t.Errorf("Host = %q, header = %v", hr.Host, hr.Header)
	}
}

func TestCrossHostRedirectNotFollowed(t *testing.T) {
	var otherHits atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		otherHits.Add(1)
	}))
	defer other.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+"/x", http.StatusFound)
	}))
	defer srv.Close()

	resp, err := Send(t.Context(), NewClient(Options{}), sendTo(t, workspace.Request{URL: srv.URL}))
	if err != nil {
		t.Fatal(err)
	}
	if resp.Code != http.StatusFound || resp.Header.Get("Location") != other.URL+"/x" {
		t.Errorf("code = %d, location = %q", resp.Code, resp.Header.Get("Location"))
	}
	if n := otherHits.Load(); n != 0 {
		t.Errorf("other host hit %d times", n)
	}
}
