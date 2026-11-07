package ui

import (
	"errors"
	"strings"
	"testing"

	"github.com/mc-akash/lazypost/internal/workspace"
)

func TestLoadErrorStatusHidesQuotedValues(t *testing.T) {
	ws := &workspace.Workspace{Root: "/w", LoadErrors: []workspace.FileError{{
		Path: "/w/secret.yaml",
		Err:  errors.New("yaml: unmarshal errors:\n  line 1: cannot unmarshal !!str `sk-SUPE...` into map[string]string"),
	}}}
	got := loadErrorsStatus(ws)
	if strings.Contains(got, "sk-SUPE") || !strings.Contains(got, "secret.yaml") || !strings.Contains(got, "cannot unmarshal") {
		t.Fatalf("status = %q", got)
	}
}
