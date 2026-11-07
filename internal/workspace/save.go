package workspace

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/mc-akash/lazypost/internal/paths"
)

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

func Slug(name string) string {
	s := strings.Trim(nonSlug.ReplaceAllString(strings.ToLower(name), "-"), "-")
	if s == "" {
		return "request"
	}
	return s
}

func writeYAML(path string, v any, perm os.FileMode) error {
	data, err := yaml.Marshal(v)
	if err != nil {
		return fmt.Errorf("encode %s: %w", path, err)
	}
	return paths.WriteAtomic(path, data, perm)
}

func uniquePath(dir, slug, ext string) (string, error) {
	p := filepath.Join(dir, slug+ext)
	for i := 2; ; i++ {
		_, err := os.Stat(p)
		if errors.Is(err, os.ErrNotExist) {
			return p, nil
		}
		if err != nil {
			return "", err
		}
		p = filepath.Join(dir, fmt.Sprintf("%s-%d%s", slug, i, ext))
	}
}

func (w *Workspace) SaveRequest(r *Request) error {
	if err := writeYAML(r.Path, r, 0o644); err != nil {
		return err
	}
	r.ModTime = modTime(r.Path)
	return nil
}

func (w *Workspace) CreateRequest(parent *Folder, name string) (*Request, error) {
	path, err := uniquePath(parent.Path, Slug(name), yamlSuffix)
	if err != nil {
		return nil, err
	}
	r := &Request{
		Name:   name,
		Method: defaultMethod,
		URL:    "{{base_url}}/",
		Path:   path,
		Parent: parent,
	}
	if err := w.SaveRequest(r); err != nil {
		return nil, err
	}
	parent.Requests = append(parent.Requests, r)
	sort.Slice(parent.Requests, func(i, j int) bool {
		return filepath.Base(parent.Requests[i].Path) < filepath.Base(parent.Requests[j].Path)
	})
	return r, nil
}

func (w *Workspace) CreateFolder(parent *Folder, name string) (*Folder, error) {
	path, err := uniquePath(parent.Path, Slug(name), "")
	if err != nil {
		return nil, err
	}
	if err := os.Mkdir(path, 0o755); err != nil {
		return nil, err
	}
	f := &Folder{Name: filepath.Base(path), Path: path, Parent: parent}
	parent.Folders = append(parent.Folders, f)
	sort.Slice(parent.Folders, func(i, j int) bool { return parent.Folders[i].Name < parent.Folders[j].Name })
	return f, nil
}

func (w *Workspace) DeleteRequest(r *Request) error {
	if err := os.Remove(r.Path); err != nil {
		return err
	}
	if p := r.Parent; p != nil {
		p.Requests = removeItem(p.Requests, r)
	}
	return nil
}

func (w *Workspace) DeleteFolder(f *Folder) error {
	if f.Parent == nil {
		return errors.New("cannot delete the collections root")
	}
	if err := os.RemoveAll(f.Path); err != nil {
		return err
	}
	f.Parent.Folders = removeItem(f.Parent.Folders, f)
	return nil
}

func removeItem[T comparable](s []T, item T) []T {
	out := make([]T, 0, len(s))
	for _, v := range s {
		if v != item {
			out = append(out, v)
		}
	}
	return out
}

func (w *Workspace) envPaths(name string) (plain, secret string) {
	dir := filepath.Join(w.Root, "envs")
	return filepath.Join(dir, name+yamlSuffix), filepath.Join(dir, name+secretSuffix)
}

func (w *Workspace) CreateEnv(name string) (*Env, error) {
	if name == "" || name != filepath.Base(name) || strings.HasPrefix(name, ".") || strings.HasSuffix(name, ".secret") {
		return nil, fmt.Errorf("invalid environment name %q", name)
	}
	for _, e := range w.Envs {
		if e.Name == name {
			return nil, fmt.Errorf("environment %q already exists", name)
		}
	}
	e := &Env{Name: name, Vars: map[string]string{}, Secrets: map[string]string{}}
	if err := w.SaveEnv(e); err != nil {
		return nil, err
	}
	w.Envs = append(w.Envs, e)
	sort.Slice(w.Envs, func(i, j int) bool { return w.Envs[i].Name < w.Envs[j].Name })
	return e, nil
}

func (w *Workspace) SaveEnv(e *Env) error {
	plain, secret := w.envPaths(e.Name)
	for _, le := range w.LoadErrors {
		if le.Path == plain || le.Path == secret {
			rel, _ := filepath.Rel(w.Root, le.Path)
			return fmt.Errorf("fix %s first: it failed to load", filepath.ToSlash(rel))
		}
	}
	if len(e.Secrets) == 0 {
		if err := writeYAML(plain, nonNil(e.Vars), 0o644); err != nil {
			return err
		}
		return removeIfExists(secret)
	}
	if err := writeYAML(secret, e.Secrets, 0o600); err != nil {
		return err
	}
	return writeYAML(plain, nonNil(e.Vars), 0o644)
}

func (w *Workspace) DeleteEnv(e *Env) error {
	plain, secret := w.envPaths(e.Name)
	if err := removeIfExists(plain); err != nil {
		return err
	}
	if err := removeIfExists(secret); err != nil {
		return err
	}
	w.Envs = removeItem(w.Envs, e)
	return nil
}

func nonNil(m map[string]string) map[string]string {
	if m == nil {
		return map[string]string{}
	}
	return m
}

func removeIfExists(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
