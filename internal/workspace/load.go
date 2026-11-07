package workspace

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"
)

const (
	defaultMethod  = "GET"
	folderFileName = "_folder.yaml"
	secretSuffix   = ".secret.yaml"
	yamlSuffix     = ".yaml"
)

var errSettingsMoved = errors.New("timeout and insecure moved to ~/.config/lazypost/config.yaml")

type rootFile struct {
	Globals  map[string]string `yaml:"globals,omitempty"`
	Timeout  any               `yaml:"timeout,omitempty"`
	Insecure any               `yaml:"insecure,omitempty"`
}

type folderFile struct {
	Vars map[string]string `yaml:"vars,omitempty"`
	Auth Auth              `yaml:"auth,omitempty"`
}

func Load(root string) (*Workspace, error) {
	if _, err := Init(root); err != nil {
		return nil, err
	}
	w := &Workspace{Root: root}

	var rf rootFile
	rootPath := filepath.Join(root, "lazypost.yaml")
	if w.readYAML(rootPath, &rf) {
		w.Globals = rf.Globals
		if rf.Timeout != nil || rf.Insecure != nil {
			w.LoadErrors = append(w.LoadErrors, FileError{Path: rootPath, Err: errSettingsMoved})
		}
	}
	w.SharedSecrets = w.readMap(filepath.Join(root, "secret.yaml"))

	if err := w.loadEnvs(); err != nil {
		return nil, err
	}
	w.Collections = &Folder{Path: filepath.Join(root, "collections")}
	if err := w.loadFolder(w.Collections); err != nil {
		return nil, err
	}
	return w, nil
}

func (w *Workspace) readYAML(path string, out any) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			w.LoadErrors = append(w.LoadErrors, FileError{Path: path, Err: err})
		}
		return false
	}
	if err := yaml.Unmarshal(data, out); err != nil {
		w.LoadErrors = append(w.LoadErrors, FileError{Path: path, Err: err})
		return false
	}
	return true
}

func (w *Workspace) readMap(path string) map[string]string {
	m := map[string]string{}
	if !w.readYAML(path, &m) || m == nil {
		return map[string]string{}
	}
	return m
}

func (w *Workspace) loadEnvs() error {
	dir := filepath.Join(w.Root, "envs")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read envs: %w", err)
	}
	byName := map[string]*Env{}
	get := func(name string) *Env {
		e, ok := byName[name]
		if !ok {
			e = &Env{Name: name, Vars: map[string]string{}, Secrets: map[string]string{}}
			byName[name] = e
			w.Envs = append(w.Envs, e)
		}
		return e
	}
	for _, ent := range entries {
		n := ent.Name()
		if ent.IsDir() || !strings.HasSuffix(n, yamlSuffix) {
			continue
		}
		path := filepath.Join(dir, n)
		if name, ok := strings.CutSuffix(n, secretSuffix); ok {
			get(name).Secrets = w.readMap(path)
		} else {
			get(strings.TrimSuffix(n, yamlSuffix)).Vars = w.readMap(path)
		}
	}
	sort.Slice(w.Envs, func(i, j int) bool { return w.Envs[i].Name < w.Envs[j].Name })
	return nil
}

func (w *Workspace) loadFolder(f *Folder) error {
	entries, err := os.ReadDir(f.Path)
	if err != nil {
		return fmt.Errorf("read %s: %w", f.Path, err)
	}
	for _, ent := range entries {
		n := ent.Name()
		path := filepath.Join(f.Path, n)
		switch {
		case ent.IsDir():
			sub := &Folder{Name: n, Path: path, Parent: f}
			if err := w.loadFolder(sub); err != nil {
				w.LoadErrors = append(w.LoadErrors, FileError{Path: path, Err: err})
				continue
			}
			f.Folders = append(f.Folders, sub)
		case n == folderFileName:
			var ff folderFile
			if w.readYAML(path, &ff) {
				f.Vars, f.Auth = ff.Vars, ff.Auth
			}
		case strings.HasSuffix(n, yamlSuffix) && !strings.HasSuffix(n, secretSuffix):
			var r Request
			mod := modTime(path)
			if !w.readYAML(path, &r) {
				continue
			}
			if r.Method == "" {
				r.Method = defaultMethod
			}
			if !ValidMethod(r.Method) {
				w.LoadErrors = append(w.LoadErrors, FileError{Path: path, Err: fmt.Errorf("invalid method %q", r.Method)})
				continue
			}
			r.Path, r.Parent, r.ModTime = path, f, mod
			f.Requests = append(f.Requests, &r)
		}
	}
	return nil
}

func modTime(path string) (t time.Time) {
	if fi, err := os.Stat(path); err == nil {
		t = fi.ModTime()
	}
	return t
}

func (w *Workspace) FolderChain(r *Request) []*Folder {
	var chain []*Folder
	for f := r.Parent; f != nil; f = f.Parent {
		chain = append([]*Folder{f}, chain...)
	}
	return chain
}

func (w *Workspace) FindRequest(path string) *Request {
	var find func(f *Folder) *Request
	find = func(f *Folder) *Request {
		for _, r := range f.Requests {
			if r.Path == path {
				return r
			}
		}
		for _, sub := range f.Folders {
			if r := find(sub); r != nil {
				return r
			}
		}
		return nil
	}
	if w.Collections == nil {
		return nil
	}
	return find(w.Collections)
}
