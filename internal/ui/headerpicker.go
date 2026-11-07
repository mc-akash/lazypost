package ui

import (
	"cmp"
	"slices"
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"

	"github.com/mc-akash/lazypost/internal/workspace"
)

const (
	pickOrTyped         = "pick (or use typed)"
	statusBadHeaderName = "invalid header name: no spaces, colons or control characters"
)

var builtinHeaders = []string{
	"Accept", "Accept-Encoding", "Accept-Language", "Authorization", "Cache-Control",
	"Content-Type", "Cookie", "If-Match", "If-None-Match", "If-Modified-Since", "Origin",
	"Referer", "User-Agent", "X-API-Key", "X-Request-ID", "X-Correlation-ID", "X-Forwarded-For",
}

var headerValueSuggestions = map[string][]string{
	"accept":          {"application/json", "*/*", "text/plain", "application/xml"},
	"content-type":    {"application/json", "application/x-www-form-urlencoded", "multipart/form-data", "text/plain"},
	"accept-encoding": {"gzip", "deflate", "br", "identity"},
	"cache-control":   {"no-cache", "no-store", "max-age=0"},
	"accept-language": {"en-US,en;q=0.9"},
}

func (m Model) usedHeaderNames() []string {
	counts := map[string]int{}
	var names []string
	var walk func(f *workspace.Folder)
	walk = func(f *workspace.Folder) {
		if f == nil {
			return
		}
		for _, r := range f.Requests {
			for _, h := range r.Headers {
				k := strings.ToLower(strings.TrimSpace(h.Key))
				if k == "" {
					continue
				}
				if counts[k] == 0 {
					names = append(names, strings.TrimSpace(h.Key))
				}
				counts[k]++
			}
		}
		for _, sub := range f.Folders {
			walk(sub)
		}
	}
	walk(m.ws().Collections)
	slices.SortStableFunc(names, func(a, b string) int {
		la, lb := strings.ToLower(a), strings.ToLower(b)
		return cmp.Or(cmp.Compare(counts[lb], counts[la]), cmp.Compare(la, lb))
	})
	return names
}

func headerNameItems(used []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(used)+len(builtinHeaders))
	for _, n := range append(slices.Clone(used), builtinHeaders...) {
		if k := strings.ToLower(n); !seen[k] {
			seen[k] = true
			out = append(out, n)
		}
	}
	return out
}

func (m Model) openHeaderPicker() Model {
	m, ok := m.requireEditable()
	if !ok {
		return m
	}
	items := headerNameItems(m.usedHeaderNames())
	p := newListPicker(dialogAddHeader, typeInput, items, "", pickOrTyped, Model.pickHeaderValue)
	p.onTyped = func(m Model, name string) Model {
		if !validHeaderName(name) {
			m.status = statusBadHeaderName
			return m
		}
		return m.pickHeaderValue(name)
	}
	m.overlay = p
	return m
}

func validHeaderName(name string) bool {
	return name != "" && !strings.ContainsFunc(name, func(r rune) bool {
		return r == ':' || unicode.IsSpace(r) || unicode.IsControl(r)
	})
}

func (m Model) pickHeaderValue(name string) Model {
	vals := headerValueSuggestions[strings.ToLower(name)]
	if len(vals) == 0 {
		return m.inputHeaderValue(name)
	}
	add := func(m Model, v string) Model {
		m.overlay = nil
		return m.appendHeader(name, v)
	}
	p := newListPicker(headerValueTitle(name), typeInput, slices.Clone(vals), "", pickOrTyped, add)
	p.onTyped = add
	m.overlay = p
	return m
}

func (m Model) inputHeaderValue(name string) Model {
	in := newInput(headerValueTitle(name), "", func(m Model, v string) (Model, tea.Cmd) {
		return m.appendHeader(name, strings.TrimSpace(v)), nil
	})
	m.overlay = &in
	return m
}

func (m Model) appendHeader(name, value string) Model {
	m = m.editRequest(func(r *workspace.Request) {
		r.Headers = append(r.Headers, workspace.KV{Key: name, Value: value})
	})
	m.request.row = m.reqRowCount() - 1
	return m
}

func headerValueTitle(name string) string { return name + " value" }
