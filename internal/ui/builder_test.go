package ui

import (
	"strings"
	"testing"
)

func TestModelBuilderRequiresDeps(t *testing.T) {
	full := func(t *testing.T) *ModelBuilder {
		d := testDeps(t, fixtureWS(t))
		return NewModelBuilder().WithWorkspace(d.WS).WithState(d.StateStore, d.Session).
			WithHistory(d.History).WithClipboard(d.Clip).WithClient(d.Client)
	}
	if _, err := full(t).Build(); err != nil {
		t.Fatalf("full builder: %v", err)
	}
	for name, tc := range map[string]struct {
		unset func(*ModelBuilder)
		want  string
	}{
		"workspace": {func(b *ModelBuilder) { b.WithWorkspace(nil) }, "workspace"},
		"state":     {func(b *ModelBuilder) { b.WithState(nil, b.deps.Session) }, "state store"},
		"clipboard": {func(b *ModelBuilder) { b.WithClipboard(nil) }, "clipboard"},
		"client":    {func(b *ModelBuilder) { b.WithClient(nil) }, "http client"},
	} {
		b := full(t)
		tc.unset(b)
		if _, err := b.Build(); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
}
