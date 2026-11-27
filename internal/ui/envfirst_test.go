package ui

import (
	"slices"
	"strings"
	"testing"
)

func withoutEnvs(t *testing.T, m Model) Model {
	t.Helper()
	for _, e := range slices.Clone(m.ws().Envs) {
		if err := m.ws().DeleteEnv(e); err != nil {
			t.Fatal(err)
		}
	}
	m.envName = ""
	m.environments = newEnvironmentPanel(nil, "")
	return m
}

func expectInput(t *testing.T, m Model, title string) {
	t.Helper()
	if in := m.input(); in == nil || !strings.HasPrefix(in.title, title) {
		t.Fatalf("want %q dialog, overlay = %#v, status = %q", title, m.overlay, m.Status())
	}
}

func TestNoEnvsAddVarCreatesEnvFirst(t *testing.T) {
	m := press(withoutEnvs(t, newTestModel(t, fixtureWS(t))), "V", "a")
	expectInput(t, m, dialogNewEnv)
	m = submitInput(t, m, "dev")
	expectInput(t, m, dialogAddVar)
	if e := m.activeEnv(); e == nil || e.Name != "dev" {
		t.Fatalf("active env = %v", e)
	}
}

func TestNoEnvsEditVarCreatesEnvFirst(t *testing.T) {
	m := press(withoutEnvs(t, newTestModel(t, fixtureWS(t))), "V", "enter")
	expectInput(t, m, dialogNewEnv)
	expectInput(t, submitInput(t, m, "dev"), dialogEditPrefix)
}

func TestNoEnvsCancelCreateKeepsState(t *testing.T) {
	m := press(withoutEnvs(t, newTestModel(t, fixtureWS(t))), "V", "a", "esc")
	if m.overlay != nil || len(m.ws().Envs) != 0 {
		t.Fatalf("esc should cancel: overlay = %#v, envs = %d", m.overlay, len(m.ws().Envs))
	}
}

func TestWriteDraftPicksEnvThenSaves(t *testing.T) {
	m, _ := sendModel(t, fixtureWS(t), "http://example.invalid")
	m.envName = ""
	m = press(addHeader(t, press(m, "4"), "Authorization: Bearer tok-draft-9"), "w")
	if p := m.picker(); p == nil || p.title != envPickerTitle {
		t.Fatalf("env picker not shown, status = %q", m.Status())
	}
	m = press(m, "enter")
	if !strings.HasPrefix(m.Status(), "saved (1 secret → ") || len(m.drafts) != 0 {
		t.Fatalf("status = %q, drafts = %v", m.Status(), m.drafts)
	}
}

func TestWriteDraftNoEnvsCreatesEnvThenSaves(t *testing.T) {
	m, _ := sendModel(t, fixtureWS(t), "http://example.invalid")
	m = withoutEnvs(t, m)
	m = press(addHeader(t, press(m, "4"), "Authorization: Bearer tok-draft-9"), "w")
	expectInput(t, m, dialogNewEnv)
	if m = submitInput(t, m, "prod"); m.Status() != "saved (1 secret → prod)" {
		t.Fatalf("status = %q", m.Status())
	}
}
