package ui

import (
	"errors"
	"net/http"

	"github.com/mc-akash/lazypost/internal/history"
	"github.com/mc-akash/lazypost/internal/state"
	"github.com/mc-akash/lazypost/internal/workspace"
)

type ModelBuilder struct {
	deps modelDeps
}

func NewModelBuilder() *ModelBuilder {
	return &ModelBuilder{}
}

func (b *ModelBuilder) WithWorkspace(ws *workspace.Workspace) *ModelBuilder {
	b.deps.WS = ws
	return b
}

func (b *ModelBuilder) WithState(store *state.Store, sess state.Session) *ModelBuilder {
	b.deps.StateStore, b.deps.Session = store, sess
	return b
}

func (b *ModelBuilder) WithHistory(h *history.Store) *ModelBuilder {
	b.deps.History = h
	return b
}

func (b *ModelBuilder) WithClipboard(c Clipboard) *ModelBuilder {
	b.deps.Clip = c
	return b
}

func (b *ModelBuilder) WithClient(c *http.Client) *ModelBuilder {
	b.deps.Client = c
	return b
}

func (b *ModelBuilder) WithStartupStatus(s string) *ModelBuilder {
	b.deps.StartupStatus = s
	return b
}

func (b *ModelBuilder) Build() (Model, error) {
	switch {
	case b.deps.WS == nil:
		return Model{}, errors.New("model builder: workspace is required")
	case b.deps.StateStore == nil:
		return Model{}, errors.New("model builder: state store is required")
	case b.deps.Clip == nil:
		return Model{}, errors.New("model builder: clipboard is required")
	case b.deps.Client == nil:
		return Model{}, errors.New("model builder: http client is required")
	}
	return newModel(b.deps), nil
}
