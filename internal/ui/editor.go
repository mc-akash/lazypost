package ui

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/google/shlex"
	"go.yaml.in/yaml/v3"

	"github.com/mc-akash/lazypost/internal/workspace"
)

const (
	defaultEditor      = "vi"
	statusEmptyEdit    = "edit cancelled: empty file"
	tempRequestPattern = "lazypost-request-*.yaml"
	tempBodyPrefix     = "lazypost-body-*"
	tempFileMode       = 0o600
)

var errNotEditable = errors.New(statusNotEditable)

type editKind int

const (
	editRequest editKind = iota
	editBodyText
)

type editorDoneMsg struct {
	kind editKind
	path string
	err  error
}

func editorCommand(path string) (*exec.Cmd, error) {
	val := strings.TrimSpace(os.Getenv("VISUAL"))
	if val == "" {
		val = strings.TrimSpace(os.Getenv("EDITOR"))
	}
	if val == "" {
		val = defaultEditor
	}
	args, err := shlex.Split(val)
	if err != nil {
		return nil, fmt.Errorf("parse editor %q: %w", val, err)
	}
	if len(args) == 0 {
		return nil, fmt.Errorf("empty editor command %q", val)
	}
	return exec.Command(args[0], append(args[1:], path)...), nil
}

func writeTempFile(pattern string, data []byte) (string, error) {
	f, err := os.CreateTemp("", pattern)
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}
	name := f.Name()
	err = f.Chmod(tempFileMode)
	if err == nil {
		_, err = f.Write(data)
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(name)
		return "", fmt.Errorf("write temp file: %w", err)
	}
	return name, nil
}

func (m Model) requestTempFile() (string, error) {
	req, _, ok := m.editable()
	if !ok {
		return "", errNotEditable
	}
	data, err := yaml.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("encode request: %w", err)
	}
	return writeTempFile(tempRequestPattern, data)
}

func bodyFileExt(t workspace.BodyType) string {
	if t == "json" {
		return ".json"
	}
	return ".txt"
}

func (m Model) openEditor() (Model, tea.Cmd) {
	m, ok := m.requireEditable()
	if !ok {
		return m, nil
	}
	path, err := m.requestTempFile()
	if err != nil {
		m = m.fail("editor: " + err.Error())
		return m, nil
	}
	return m.execEditor(editRequest, path)
}

func (m Model) execEditor(kind editKind, path string) (Model, tea.Cmd) {
	cmd, err := editorCommand(path)
	if err != nil {
		os.Remove(path)
		m = m.fail("editor: " + err.Error())
		return m, nil
	}
	return m, tea.ExecProcess(cmd, func(err error) tea.Msg {
		return editorDoneMsg{kind: kind, path: path, err: err}
	})
}

func (m Model) handleEditorDone(msg editorDoneMsg) Model {
	defer os.Remove(msg.path)
	if msg.err != nil {
		m = m.fail("editor: " + msg.err.Error())
		return m
	}
	if msg.kind == editBodyText {
		return m.applyEditorText(msg.path)
	}
	return m.applyEditedRequest(msg.path)
}

func (m Model) applyEditedRequest(path string) Model {
	cur, file, ok := m.editable()
	if !ok {
		m.status = statusNotEditable
		return m
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return m.fail("editor: " + err.Error())
	}
	if len(bytes.TrimSpace(data)) == 0 {
		m.status = statusEmptyEdit
		return m
	}
	var req workspace.Request
	if err := yaml.Unmarshal(data, &req); err != nil {
		return m.fail("invalid YAML: " + err.Error())
	}
	if req.Method == "" {
		req.Method = "GET"
	}
	if !workspace.ValidMethod(req.Method) {
		return m.fail(fmt.Sprintf("invalid method %q", req.Method))
	}
	if req.Name == "" {
		req.Name = cur.Name
		if file != nil {
			req.Name = file.Name
		}
	}
	if sameRequest(req, cur) {
		return m
	}
	return m.storeEdit(file, req)
}

func sameRequest(a, b workspace.Request) bool {
	ya, errA := yaml.Marshal(a)
	yb, errB := yaml.Marshal(b)
	return errA == nil && errB == nil && bytes.Equal(ya, yb)
}

func (m Model) setBodyContent(content string) Model {
	cur, _, ok := m.editable()
	if !ok {
		m.status = statusNotEditable
		return m
	}
	if cur.Body.Content == content {
		return m
	}
	return m.editRequest(func(r *workspace.Request) {
		r.Body.Content = content
		if content != "" && (r.Body.Type == "" || r.Body.Type == "none") {

			r.Body.Type = "raw"
			if masked, _, _ := maskTemplates(strings.TrimSpace(content)); json.Valid([]byte(masked)) {
				r.Body.Type = "json"
			}
		}
	})
}
