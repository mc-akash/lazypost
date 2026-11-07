package ui

import (
	"strings"
	"testing"

	"github.com/mc-akash/lazypost/internal/workspace"
)

func jenkinsModel(t *testing.T) Model {
	t.Helper()
	ws := fixtureWS(t)
	mustEnv(t, ws, "jenkins")
	loaded, err := workspace.Load(ws.Root)
	if err != nil {
		t.Fatal(err)
	}
	return newTestModel(t, loaded)
}

func TestEnvPickerSlashFilterPicksJenkins(t *testing.T) {
	m := typeText(press(jenkinsModel(t), "E", "/"), "jen")
	if !strings.Contains(m.plainRender(), "/ jen") {
		t.Fatalf("filter line missing:\n%s", m.plainRender())
	}
	if got := strings.Join(m.picker().matches(), ","); got != "jenkins" {
		t.Fatalf("matches = %q", got)
	}
	m = press(m, "enter")
	if m.picker() != nil || m.activeEnv().Name != "jenkins" {
		t.Fatalf("picker %v active %q", m.picker(), m.activeEnv().Name)
	}
}

func TestEnvPickerJKMoveWithoutFiltering(t *testing.T) {
	m := press(jenkinsModel(t), "E", "k", "k", "j")
	if m.picker().filter != "" || m.picker().filtering {
		t.Fatalf("j/k filtered: %q", m.picker().filter)
	}
	if m = press(m, "enter"); m.activeEnv().Name != "jenkins" {
		t.Fatalf("active = %q, want jenkins", m.activeEnv().Name)
	}
}

func TestEnvPickerFilterTypesNavLetters(t *testing.T) {
	m := press(jenkinsModel(t), "E", "/")
	m = press(m, "j", "k", "G", "g")
	if m.picker().filter != "jkGg" {
		t.Fatalf("filter = %q, want jkGg", m.picker().filter)
	}
	if m = press(m, "backspace"); m.picker().filter != "jkG" {
		t.Fatalf("backspace: filter %q", m.picker().filter)
	}
}

func TestEnvPickerEscLeavesFilterThenCloses(t *testing.T) {
	m := press(jenkinsModel(t), "E", "/", "e", "esc")
	if m.picker() == nil || m.picker().filtering {
		t.Fatal("first esc should only leave filter mode")
	}
	if got := strings.Join(m.picker().matches(), ","); got != "dev,jenkins" {
		t.Fatalf("filtered list not kept: %q", got)
	}
	if m = press(m, "j"); m.picker().filter != "e" || m.picker().cursor != 1 {
		t.Fatalf("j after esc: filter %q cursor %d", m.picker().filter, m.picker().cursor)
	}
	m = press(m, "esc")
	if m.picker() != nil || m.activeEnv().Name != "staging" {
		t.Fatalf("second esc: picker %v active %q", m.picker(), m.activeEnv().Name)
	}
}

func TestEnvPickerHintsFollowFilterMode(t *testing.T) {
	m := press(jenkinsModel(t), "E")
	if line := hintLine(m.hints()); !strings.Contains(line, "filter: /") {
		t.Fatalf("idle hints %q", line)
	}
	m = press(m, "/")
	if line := hintLine(m.hints()); !strings.Contains(line, "stop filtering: esc") {
		t.Fatalf("filter hints %q", line)
	}
}
