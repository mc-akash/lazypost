package ui

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"strings"
	"sync"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/mc-akash/lazypost/internal/history"
	"github.com/mc-akash/lazypost/internal/httpx"
	"github.com/mc-akash/lazypost/internal/vars"
	"github.com/mc-akash/lazypost/internal/workspace"
)

const (
	titleCancelled = "cancelled"
	titleError     = "ERR"
)

type sendResultMsg struct {
	seq  int
	path string
	resp *httpx.Response
	err  error
}

type historyErrMsg struct{ err error }

var historyMu sync.Mutex

func newSpinner() spinner.Model {
	return spinner.New(spinner.WithSpinner(spinner.MiniDot))
}

type sendPhase int

const (
	phaseSending sendPhase = iota
	phaseCancelled
	phaseDone
)

type sendResult struct {
	phase sendPhase
	seq   int
	resp  *httpx.Response
	err   error
}

type flight struct {
	seq         int
	key         string
	fromHistory bool
	cancel      context.CancelFunc
	entry       history.Entry
	secrets     []string
}

type selection struct {
	file    *workspace.Request
	tmpl    workspace.Request
	path    string
	entry   *history.Entry
	scratch bool
}

func (s selection) key() string {
	switch {
	case s.entry != nil:
		return histKey(*s.entry)
	case s.scratch:
		return scratchKey
	}
	return s.path
}

func (s selection) resultKey() string {
	if s.scratch {
		return scratchKey
	}
	return s.path
}

func histKey(e history.Entry) string {
	return "history\x00" + e.ID + "\x00" + e.Time.Format(time.RFC3339Nano)
}

func (m Model) selection() (selection, bool) {
	if m.scratchSelected() {
		return selection{tmpl: cloneRequest(*m.scratch), scratch: true}, true
	}
	return m.rowSelection()
}

func (m Model) rowSelection() (selection, bool) {
	switch m.lastLeft {
	case PanelCollections:
		if row, ok := m.collections.selected(m.ws().Collections); ok && row.req != nil {
			return selection{file: row.req, tmpl: m.effective(row.req), path: row.req.Path}, true
		}
	case PanelHistory:
		if e, ok := m.history.selected(); ok {
			var file *workspace.Request
			if m.ws().Collections != nil && e.RequestPath != "" {
				file = m.ws().FindRequest(e.RequestPath)
			}
			return selection{file: file, tmpl: e.Template, path: e.RequestPath, entry: &e}, true
		}
	}
	return selection{}, false
}

func (m Model) selectedRequest() workspace.Request {
	s, _ := m.selection()
	return s.tmpl
}

func (m Model) activeEnv() *workspace.Env {
	for _, e := range m.ws().Envs {
		if e.Name == m.envName {
			return e
		}
	}
	return nil
}

func (m Model) resolverFor(s selection) *vars.Resolver {
	return vars.ForRequest(m.ws(), m.activeEnv(), s.file)
}

func (m Model) resolve(sel selection) (workspace.Request, []string, *vars.Resolver) {
	var chain []*workspace.Folder
	if sel.file != nil {
		chain = m.ws().FolderChain(sel.file)
	}
	r := m.resolverFor(sel)
	resolved, missing := httpx.Resolve(sel.tmpl, httpx.EffectiveAuth(&sel.tmpl, chain), r)
	return resolved, missing, r
}

func (m Model) send() (Model, tea.Cmd) {
	sel, ok := m.selection()
	if !ok {
		return m, nil
	}
	idle := m.flight == nil
	m = m.cancelSend()

	resolved, missing, r := m.resolve(sel)
	tmpl, literals := maskLiteralCredentials(sel.tmpl)
	secrets := append(r.SecretValues(), literals...)

	m.seq++
	ctx, cancel := context.WithCancel(context.Background())
	m.flight = &flight{
		seq:         m.seq,
		key:         sel.key(),
		fromHistory: sel.entry != nil,
		cancel:      cancel,
		secrets:     secrets,
		entry: history.Entry{
			Time:        time.Now(),
			Env:         m.envName,
			Root:        m.ws().Root,
			RequestPath: sel.path,
			Template:    tmpl,
			Sent:        history.Redact(resolved, secrets),
		},
	}
	m.results = withResult(m.results, sel.key(), sendResult{phase: phaseSending, seq: m.seq})
	m.status = ""
	if len(missing) > 0 {
		m.status = "missing: " + strings.Join(missing, ", ")
	}
	cmd := sendCmd(ctx, m.deps.Client, m.seq, sel.resultKey(), resolved)
	if idle {

		m.spin = newSpinner()
		cmd = tea.Batch(cmd, m.spin.Tick)
	}
	return m, cmd
}

func maskLiteralCredentials(t workspace.Request) (workspace.Request, []string) {
	out, found := extractSecrets(t, "", &workspace.Env{})
	mask := func(got *string, was string) {
		if *got != was {
			*got = history.MaskCredentials(*got)
		}
	}
	for i := range out.Headers {
		mask(&out.Headers[i].Value, t.Headers[i].Value)
	}
	mask(&out.Auth.Password, t.Auth.Password)
	mask(&out.Auth.Token, t.Auth.Token)
	mask(&out.Auth.Value, t.Auth.Value)
	values := make([]string, len(found))
	for i, s := range found {
		values[i] = s.value
	}
	return out, values
}

func (m Model) tickSpinner(msg spinner.TickMsg) (Model, tea.Cmd) {
	if m.flight == nil {
		return m, nil
	}
	var cmd tea.Cmd
	m.spin, cmd = m.spin.Update(msg)
	return m, cmd
}

func sendCmd(ctx context.Context, c *http.Client, seq int, path string, req workspace.Request) tea.Cmd {
	return func() tea.Msg {
		hr, err := httpx.Encode(ctx, req)
		if err != nil {
			return sendResultMsg{seq: seq, path: path, err: err}
		}
		resp, err := httpx.Send(ctx, c, hr)
		return sendResultMsg{seq: seq, path: path, resp: resp, err: err}
	}
}

func (m Model) cancelSend() Model {
	if m.flight == nil {
		return m
	}
	m.flight.cancel()
	m.results = withResult(m.results, m.flight.key, sendResult{phase: phaseCancelled, seq: m.flight.seq})
	m.flight = nil
	return m
}

func (m Model) handleSendResult(msg sendResultMsg) (Model, tea.Cmd) {
	f := m.flight
	if f == nil || msg.seq != f.seq {
		return m, nil
	}
	f.cancel()
	m.flight = nil

	sendErr := redactErr(msg.err, f.secrets)
	res := sendResult{phase: phaseDone, seq: msg.seq, resp: msg.resp, err: sendErr}
	results := withResult(m.results, msg.path, res)
	if f.key != msg.path {
		delete(results, f.key)
	}
	if msg.path == "" {
		delete(results, msg.path)
	}

	e := completeEntry(f.entry, msg.resp, sendErr, f.secrets)
	if id, err := history.NewID(); err == nil {
		e.ID = id
	}
	if f.fromHistory {
		results[histKey(e)] = res
	}
	m.results = results
	m.history = m.history.add(e, f.fromHistory, m.historyMax())
	if m.deps.History == nil {
		return m, nil
	}
	return m, appendHistoryCmd(m.deps.History, e)
}

func redactErr(err error, secrets []string) error {
	if err == nil {
		return nil
	}
	return errors.New(history.RedactString(err.Error(), secrets))
}

func appendHistoryCmd(s *history.Store, e history.Entry) tea.Cmd {
	return func() tea.Msg {
		historyMu.Lock()
		defer historyMu.Unlock()
		if err := s.Append(e); err != nil {
			return historyErrMsg{err: err}
		}
		return nil
	}
}

func completeEntry(e history.Entry, resp *httpx.Response, err error, secrets []string) history.Entry {
	if err != nil {
		e.Err = err.Error()
		return e
	}
	e.Code = resp.Code
	e.Status = history.RedactString(resp.Status, secrets)
	e.Header = make(http.Header, len(resp.Header))
	for k, vs := range resp.Header {
		for _, v := range vs {
			e.Header[k] = append(e.Header[k], history.RedactString(v, secrets))
		}
	}
	var cut bool
	e.Body, cut = history.RedactBody(resp.Body, secrets)
	e.Truncated = resp.Truncated || cut
	e.DurationMS = resp.Duration.Milliseconds()
	return e
}

func withResult(results map[string]sendResult, key string, r sendResult) map[string]sendResult {
	out := make(map[string]sendResult, len(results)+1)
	maps.Copy(out, results)
	out[key] = r
	return out
}

func responseTitle(status string, d time.Duration, size int64, truncated bool) string {
	t := fmt.Sprintf("%s · %dms · %s", status, d.Milliseconds(), humanSize(size))
	if truncated {
		t += " (truncated)"
	}
	return t
}

func humanSize(n int64) string {
	const kb, mb = 1 << 10, 1 << 20
	switch {
	case n < kb:
		return fmt.Sprintf("%dB", n)
	case n < mb:
		return fmt.Sprintf("%.1fKB", float64(n)/kb)
	}
	return fmt.Sprintf("%.1fMB", float64(n)/mb)
}
