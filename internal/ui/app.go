package ui

import (
	"fmt"
	"net/http"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/mc-akash/lazypost/internal/history"
	"github.com/mc-akash/lazypost/internal/state"
	"github.com/mc-akash/lazypost/internal/workspace"
)

type Clipboard interface {
	Read() (string, error)
	Write(string) error
}

type modelDeps struct {
	WS            *workspace.Workspace
	Session       state.Session
	StateStore    *state.Store
	History       *history.Store
	Clip          Clipboard
	Client        *http.Client
	StartupStatus string
}

type overlay interface {
	key(m Model, msg tea.KeyPressMsg) (Model, tea.Cmd)
	draw(m Model, screen string) string
	hints(m Model) []binding
}

type paster interface {
	paste(m Model, msg tea.PasteMsg) (Model, tea.Cmd)
}

type Model struct {
	deps      modelDeps
	width     int
	height    int
	focus     Panel
	lastLeft  Panel
	envsLast  bool
	lastMain  Panel
	lastKey   string
	prevFocus Panel
	envName   string
	status    string
	overlay   overlay

	environments environmentPanel
	collections  collectionPanel
	history      historyPanel
	request      requestPanel
	response     responsePanel
	variables    variablePanel

	seq     int
	flight  *flight
	results map[string]sendResult
	drafts  map[string]state.Draft
	stale   map[string]bool
	memo    *bodyMemo
	spin    spinner.Model

	scratch   *workspace.Request
	onScratch bool
}

func newModel(d modelDeps) Model {
	envName := pickActiveEnv(d.WS.Envs, d.Session.LastEnv)
	m := Model{
		deps:         d,
		focus:        PanelCollections,
		lastLeft:     PanelCollections,
		lastMain:     PanelRequest,
		memo:         &bodyMemo{},
		spin:         newSpinner(),
		envName:      envName,
		environments: newEnvironmentPanel(d.WS.Envs, envName),
		collections:  newCollectionPanel(),
	}
	m, restoreMsgs := m.restoreSession(d.Session)
	m.status = joinStatus(append([]string{d.StartupStatus, initialStatus(d)}, restoreMsgs...))
	if errs := startupErrors(d); errs != "" {
		m = m.showError(errs)
	}
	return m
}

func pickActiveEnv(envs []*workspace.Env, last string) string {
	for _, e := range envs {
		if e.Name == last {
			return last
		}
	}
	if len(envs) > 0 {
		return envs[0].Name
	}
	return ""
}

func initialStatus(d modelDeps) string {
	ws := d.WS
	switch {
	case len(ws.LoadErrors) > 0:
		return loadErrorsStatus(ws)
	case isEmptyFolder(ws.Collections):
		return "No requests yet — press " + km.New[0] + " to create one"
	}
	return ""
}

func startupErrors(d modelDeps) string {
	var msgs []string
	if len(d.WS.LoadErrors) > 0 {
		msgs = append(msgs, loadErrorsStatus(d.WS))
	}
	if d.StartupStatus != "" {
		msgs = append(msgs, d.StartupStatus)
	}
	return strings.Join(msgs, "\n")
}

func loadErrorsStatus(ws *workspace.Workspace) string {
	first := ws.LoadErrors[0]
	return fmt.Sprintf("%d file(s) failed to load: %s: %s", len(ws.LoadErrors), relPath(ws.Root, first.Path), hideQuoted(first.Err.Error()))
}

var quotedValue = regexp.MustCompile("`[^`]*`")

func joinStatus(msgs []string) string {
	var out []string
	for _, s := range msgs {
		if s != "" {
			out = append(out, s)
		}
	}
	return strings.Join(out, "; ")
}

func relPath(root, path string) string {
	if rel, err := filepath.Rel(root, path); err == nil && filepath.IsLocal(rel) {
		return rel
	}
	return path
}

func isEmptyFolder(f *workspace.Folder) bool {
	return f == nil || (len(f.Folders) == 0 && len(f.Requests) == 0)
}

func (m Model) ws() *workspace.Workspace { return m.deps.WS }

func (m Model) bodyEd() *bodyEditor {
	ed, _ := m.overlay.(*bodyEditor)
	return ed
}

func (m Model) Init() tea.Cmd {
	if m.deps.History == nil {
		return nil
	}
	return loadHistoryCmd(m.deps.History, m.ws().Root)
}

func (m Model) SaveSession() error {
	_, err := m.saveSession()
	return err
}

func (m Model) Focus() Panel { return m.focus }

func (m Model) Status() string { return m.status }

func (m Model) View() tea.View {
	v := tea.NewView(m.Render())
	v.AltScreen = true
	return v
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		if ed := m.bodyEd(); ed != nil {
			resized := ed.sized(msg.Width, msg.Height)
			m.overlay = &resized
		}
		return m, nil
	case sendResultMsg:
		return m.handleSendResult(msg)
	case spinner.TickMsg:
		return m.tickSpinner(msg)
	case historyLoadedMsg:
		return m.handleHistoryLoaded(msg), nil
	case historyErrMsg:
		m = m.fail("history: " + msg.err.Error())
		return m, nil
	case editorDoneMsg:
		return m.handleEditorDone(msg), nil
	case tea.KeyPressMsg:
		if is(msg.String(), km.ForceQuit) {
			return m.quit()
		}
		if m.overlay != nil {
			return m.overlay.key(m, msg)
		}
		return m.handleKey(msg.String())
	case tea.PasteMsg:
		if p, ok := m.overlay.(paster); ok {
			return p.paste(m, msg)
		}
	}
	return m, nil
}

func (m Model) handleKey(key string) (tea.Model, tea.Cmd) {
	if next, cmd, ok := dispatch(m, key, m.panelBindings()); ok {
		return next, cmd
	}
	next, cmd, _ := dispatch(m, key, m.globalBindings())
	return next, cmd
}

func (m Model) globalBindings() []binding {
	scratchHint := "new unsaved request"
	if m.focus == PanelResponse || m.focus == PanelVars {
		scratchHint = ""
	}
	return []binding{
		{keys: keys(&km.JumpToPanel), run: func(m Model) (Model, tea.Cmd) {
			return m.setFocus(Panel(slices.Index(km.JumpToPanel, m.lastKey) + 1)), nil
		}, help: "jump to panel"},
		{keys: keys(&km.Send), run: Model.send},
		{keys: keys(&km.MainView), run: noCmd(Model.focusMain), help: "focus [4]/[5] (variables table from [1])"},
		{keys: keys(&km.PrevSidePanel, &km.NextSidePanel), run: func(m Model) (Model, tea.Cmd) {
			return m.setFocus(m.focus.cycleColumn(direction(m.lastKey, km.NextSidePanel))), nil
		}, help: "previous / next panel in the same column"},
		{keys: keys(&km.NextPanel, &km.PrevPanel), run: func(m Model) (Model, tea.Cmd) {
			return m.setFocus(cycle(m.focus, direction(m.lastKey, km.NextPanel))), nil
		}, help: "next / previous panel"},
		{keys: listMoves, run: func(m Model) (Model, tea.Cmd) {
			return m.moveCursor(direction(m.lastKey, km.NextItem)), nil
		}, help: "move down / up"},
		{keys: keys(&km.GotoTop, &km.GotoBottom), run: func(m Model) (Model, tea.Cmd) {
			return m.jumpEdge(is(m.lastKey, km.GotoBottom)), nil
		}, help: "top / bottom"},
		{keys: keys(&km.NewUnsaved), run: noCmd(Model.openScratch), help: "new unsaved request", hint: scratchHint},
		{keys: keys(&km.SwitchEnv), run: noCmd(Model.openEnvPicker), help: "switch environment (filter with search)"},
		{keys: keys(&km.Vars), run: noCmd(Model.jumpToVars), help: "variables of the active environment"},
		{keys: keys(&km.Reload), run: noCmd(Model.reload), help: "reload workspace from disk"},
		{keys: keys(esc), run: noCmd(Model.handleEsc), help: "cancel the running send"},
		{keys: keys(&km.Help), run: func(m Model) (Model, tea.Cmd) {
			m.overlay = &helpScreen{}
			return m, nil
		}, help: "help", hint: "keybinds"},
		{keys: keys(&km.Quit, &km.ForceQuit), run: Model.quit, help: "quit (drafts are kept)"},
	}
}

func (m Model) panelBindings() []binding {
	var bs []binding
	switch m.focus {
	case PanelEnvs:
		bs = m.envBindings()
	case PanelCollections:
		bs = m.collectionsBindings()
	case PanelHistory:
		bs = m.historyBindings()
	case PanelVars:
		return m.varsBindings()
	case PanelRequest:
		bs = m.requestBindings()
	case PanelResponse:
		bs = m.responseBindings()
	}
	if m.flight != nil {
		return append([]binding{{keys: keys(esc), run: noCmd(Model.cancelSend), hint: "cancel"}}, bs...)
	}
	return bs
}

func (m Model) closeOverlay() Model {
	m.overlay = nil
	return m
}

func (m Model) setFocus(p Panel) Model {
	m.focus = p
	switch {
	case p == PanelEnvs:
		m.envsLast = true
	case p.isSide():
		m.lastLeft, m.envsLast = p, false
	case p.isMain():
		m.lastMain = p
	}
	return m
}

func cycle(p Panel, delta int) Panel {
	n := len(cyclePanels)
	for i, c := range cyclePanels {
		if c == p {
			return cyclePanels[((i+delta)%n+n)%n]
		}
	}
	return PanelEnvs
}

func (m Model) focusMain() Model {
	if m.envsLast {
		return m.focusVars()
	}
	return m.setFocus(m.lastMain)
}

func (m Model) handleEsc() Model {
	switch {
	case m.flight != nil:
		return m.cancelSend()
	case m.focus.isMain() && m.envsLast:
		return m.setFocus(PanelEnvs)
	case m.focus.isMain():
		return m.setFocus(m.lastLeft)
	}
	return m
}

func (m Model) moveCursor(delta int) Model {
	switch m.focus {
	case PanelEnvs:
		m.environments = m.environments.move(delta, len(m.ws().Envs))
	case PanelCollections:
		m.collections = m.collections.move(delta, m.ws().Collections)
		m.onScratch = false
	case PanelHistory:
		m.history = m.history.move(delta)
		m.onScratch = false
	}
	return m
}
