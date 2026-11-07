package httpx

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mc-akash/lazypost/internal/paths"
	"github.com/mc-akash/lazypost/internal/workspace"
)

var maxBodyBytes int64 = 32 << 20

const maxRedirects = 10

const contentTypeHeader = "Content-Type"

func Encode(ctx context.Context, req workspace.Request) (*http.Request, error) {
	u, err := url.Parse(req.URL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL %q: %w", req.URL, err)
	}
	if u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("invalid URL %q: missing scheme or host", req.URL)
	}
	u.RawQuery = appendQuery(u.RawQuery, req.Params)

	body, contentType, err := encodeBody(req.Body)
	if err != nil {
		return nil, err
	}
	hr, err := http.NewRequestWithContext(ctx, strings.ToUpper(req.Method), u.String(), body)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	for _, h := range req.Headers {
		if strings.EqualFold(h.Key, "Host") {
			hr.Host = h.Value
			continue
		}
		hr.Header.Add(h.Key, h.Value)
	}

	if contentType != "" && (hr.Header.Get(contentTypeHeader) == "" || req.Body.Type == "multipart") {
		hr.Header.Set(contentTypeHeader, contentType)
	}
	return hr, nil
}

func appendQuery(raw string, params []workspace.KV) string {
	var b strings.Builder
	b.WriteString(raw)
	for _, p := range params {
		if b.Len() > 0 {
			b.WriteByte('&')
		}
		b.WriteString(url.QueryEscape(p.Key))
		b.WriteByte('=')
		b.WriteString(url.QueryEscape(p.Value))
	}
	return b.String()
}

func encodeBody(body workspace.Body) (io.Reader, string, error) {
	switch body.Type {
	case "json", "raw":
		return strings.NewReader(body.Content), "", nil
	case "form":
		v := url.Values{}
		for _, kv := range body.Form {
			v.Add(kv.Key, kv.Value)
		}
		return strings.NewReader(v.Encode()), "application/x-www-form-urlencoded", nil
	case "multipart":
		return encodeMultipart(body.Parts)
	}
	return nil, "", nil
}

func encodeMultipart(parts []workspace.Part) (io.Reader, string, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for _, p := range parts {
		if !p.File {
			if err := w.WriteField(p.Key, p.Value); err != nil {
				return nil, "", fmt.Errorf("multipart field %q: %w", p.Key, err)
			}
			continue
		}
		if err := writeFilePart(w, p); err != nil {
			return nil, "", err
		}
	}
	if err := w.Close(); err != nil {
		return nil, "", fmt.Errorf("multipart: %w", err)
	}
	return &buf, w.FormDataContentType(), nil
}

func writeFilePart(w *multipart.Writer, p workspace.Part) error {
	path, err := paths.ExpandHome(p.Value)
	if err != nil {
		return fmt.Errorf("multipart file %s: %w", p.Value, err)
	}
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("multipart file %s: %w", path, err)
	}
	defer f.Close()
	fw, err := w.CreateFormFile(p.Key, filepath.Base(path))
	if err != nil {
		return fmt.Errorf("multipart file %s: %w", path, err)
	}
	if _, err := io.Copy(fw, f); err != nil {
		return fmt.Errorf("multipart file %s: %w", path, err)
	}
	return nil
}

type Options struct {
	Timeout  time.Duration
	Insecure bool
}

func NewClient(o Options) *http.Client {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	if o.Insecure {
		tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // user opt-in
	}
	return &http.Client{
		Timeout:   o.Timeout,
		Transport: tr,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if req.URL.Host != via[0].URL.Host {
				return http.ErrUseLastResponse
			}
			if len(via) >= maxRedirects {
				return errors.New("stopped after 10 redirects")
			}
			return nil
		},
	}
}

type Response struct {
	Code      int
	Status    string
	Header    http.Header
	Body      []byte
	Truncated bool
	Size      int64
	Duration  time.Duration
}

func Send(ctx context.Context, c *http.Client, hr *http.Request) (*Response, error) {
	hr = hr.WithContext(ctx)
	start := time.Now()
	resp, err := c.Do(hr)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	size := int64(len(body))
	truncated := size > maxBodyBytes
	if truncated {
		body = body[:maxBodyBytes]
	}
	if resp.ContentLength >= 0 {
		size = resp.ContentLength
	}
	return &Response{
		Code:      resp.StatusCode,
		Status:    resp.Status,
		Header:    resp.Header,
		Body:      body,
		Truncated: truncated,
		Size:      size,
		Duration:  time.Since(start),
	}, nil
}
