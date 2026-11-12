package history

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/mc-akash/lazypost/internal/paths"
	"github.com/mc-akash/lazypost/internal/workspace"
)

const DefaultMax = 10

const MaxStoredBody = 32 << 10

const (
	fileMode     = 0o600
	redactMask   = "••••"
	minSecretLen = 3
)

type Entry struct {
	ID          string            `json:"id"`
	Time        time.Time         `json:"time"`
	Env         string            `json:"env"`
	Root        string            `json:"root,omitempty"`
	RequestPath string            `json:"request_path"`
	Template    workspace.Request `json:"template"`
	Sent        workspace.Request `json:"sent"`
	Code        int               `json:"code"`
	Status      string            `json:"status"`
	Header      http.Header       `json:"header,omitempty"`
	Body        []byte            `json:"body,omitempty"`
	Truncated   bool              `json:"truncated,omitempty"`
	DurationMS  int64             `json:"duration_ms"`
	Err         string            `json:"err,omitempty"`
}

type Store struct {
	path string
	max  int
}

func Open(path string) *Store {
	return &Store{path: path, max: DefaultMax}
}

func (s *Store) WithMax(n int) *Store {
	return &Store{path: s.path, max: n}
}

func (s *Store) Max() int { return s.max }

func (s *Store) Append(e Entry) error {
	if e.ID == "" {
		id, err := NewID()
		if err != nil {
			return err
		}
		e.ID = id
	}
	if len(e.Body) > MaxStoredBody {
		e.Body = e.Body[:MaxStoredBody:MaxStoredBody]
		e.Truncated = true
	}
	line, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("encode history entry: %w", err)
	}
	line = append(line, '\n')

	lines, err := s.readLines()
	if err != nil {
		return err
	}
	if kept := dropOldest(lines, e.Root, s.max-1); len(kept) < len(lines) {
		return s.rewrite(append(kept, line))
	}
	return s.appendLine(line)
}

func dropOldest(lines [][]byte, root string, keep int) [][]byte {
	kept := make([][]byte, 0, len(lines))
	for _, l := range slices.Backward(lines) {
		var e struct {
			Root string `json:"root"`
		}
		if json.Unmarshal(l, &e) == nil && e.Root == root {
			if keep <= 0 {
				continue
			}
			keep--
		}
		kept = append(kept, l)
	}
	slices.Reverse(kept)
	return kept
}

func (s *Store) Load() ([]Entry, error) {
	lines, err := s.readLines()
	if err != nil {
		return nil, err
	}
	entries := make([]Entry, 0, len(lines))
	for i := len(lines) - 1; i >= 0; i-- {
		var e Entry
		if json.Unmarshal(lines[i], &e) == nil {
			entries = append(entries, e)
		}
	}
	return entries, nil
}

func (s *Store) readLines() ([][]byte, error) {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read history: %w", err)
	}
	var lines [][]byte
	for _, l := range bytes.Split(data, []byte{'\n'}) {
		if len(bytes.TrimSpace(l)) == 0 {
			continue
		}
		lines = append(lines, append(slices.Clone(l), '\n'))
	}
	return lines, nil
}

func (s *Store) appendLine(line []byte) error {
	f, err := os.OpenFile(s.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, fileMode)
	if err != nil {
		return fmt.Errorf("open history: %w", err)
	}
	if _, err := f.Write(line); err != nil {
		f.Close()
		return fmt.Errorf("write history: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close history: %w", err)
	}
	return nil
}

func (s *Store) rewrite(lines [][]byte) error {
	if err := paths.WriteAtomic(s.path, bytes.Join(lines, nil), fileMode); err != nil {
		return fmt.Errorf("rewrite history: %w", err)
	}
	return nil
}

func NewID() (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generate history id: %w", err)
	}
	return hex.EncodeToString(b[:]), nil
}

func redactor(secrets []string) (replace func(string) string, longest int) {
	var olds []string
	for _, s := range secrets {
		if len(s) < minSecretLen {
			continue
		}
		olds = append(olds, s, url.QueryEscape(s), url.PathEscape(s))
	}
	if len(olds) == 0 {
		return func(s string) string { return s }, 0
	}
	slices.SortStableFunc(olds, func(a, b string) int { return len(b) - len(a) })
	pairs := make([]string, 0, 2*len(olds))
	for _, o := range olds {
		pairs = append(pairs, o, redactMask)
	}
	return strings.NewReplacer(pairs...).Replace, len(olds[0])
}

func RedactString(s string, secrets []string) string {
	r, _ := redactor(secrets)
	return r(s)
}

func RedactBody(body []byte, secrets []string) (out []byte, truncated bool) {
	r, longest := redactor(secrets)
	window := MaxStoredBody + longest
	cut := len(body) > window
	redacted := r(string(body[:min(len(body), window)]))
	keep := len(redacted)
	if cut {
		// a secret cut by the window sits in the last `longest` bytes, wherever earlier masks moved it
		keep -= longest
	}
	keep = max(0, min(keep, MaxStoredBody))
	return []byte(redacted[:keep]), cut || keep < len(redacted)
}

func Redact(req workspace.Request, secrets []string) workspace.Request {
	r, _ := redactor(secrets)

	out := req
	out.URL = r(req.URL)
	out.Params = redactKVs(req.Params, r)
	out.Headers = redactKVs(req.Headers, r)
	for i, h := range out.Headers {
		if isAuthHeader(h.Key) {
			out.Headers[i].Value = MaskCredentials(h.Value)
		}
	}
	out.Auth.Token = r(req.Auth.Token)
	out.Auth.Username = r(req.Auth.Username)
	out.Auth.Password = r(req.Auth.Password)
	out.Auth.Key = r(req.Auth.Key)
	out.Auth.Value = r(req.Auth.Value)
	out.Body.Content = r(req.Body.Content)
	out.Body.Form = redactKVs(req.Body.Form, r)
	if req.Body.Parts != nil {
		out.Body.Parts = make([]workspace.Part, len(req.Body.Parts))
		for i, p := range req.Body.Parts {
			p.Key, p.Value = r(p.Key), r(p.Value)
			out.Body.Parts[i] = p
		}
	}
	return out
}

func redactKVs(kvs []workspace.KV, r func(string) string) []workspace.KV {
	if kvs == nil {
		return nil
	}
	out := make([]workspace.KV, len(kvs))
	for i, kv := range kvs {
		kv.Key, kv.Value = r(kv.Key), r(kv.Value)
		out[i] = kv
	}
	return out
}

func isAuthHeader(name string) bool {
	return strings.EqualFold(name, "Authorization") || strings.EqualFold(name, "Proxy-Authorization")
}

func MaskCredentials(v string) string {
	scheme, _, hasCreds := strings.Cut(strings.TrimSpace(v), " ")
	if !hasCreds {
		return redactMask
	}
	return scheme + " " + redactMask
}
