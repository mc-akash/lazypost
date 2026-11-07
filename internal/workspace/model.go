package workspace

import "time"

type KV struct {
	Key      string `yaml:"key"`
	Value    string `yaml:"value"`
	Disabled bool   `yaml:"disabled,omitempty"`
}

type Part struct {
	Key      string `yaml:"key"`
	Value    string `yaml:"value"`
	File     bool   `yaml:"file,omitempty"`
	Disabled bool   `yaml:"disabled,omitempty"`
}

type BodyType string

type Body struct {
	Type    BodyType `yaml:"type,omitempty"`
	Content string   `yaml:"content,omitempty"`
	Form    []KV     `yaml:"form,omitempty"`
	Parts   []Part   `yaml:"parts,omitempty"`
}

type AuthType string

type Auth struct {
	Type     AuthType `yaml:"type,omitempty"`
	Token    string   `yaml:"token,omitempty"`
	Username string   `yaml:"username,omitempty"`
	Password string   `yaml:"password,omitempty"`
	Key      string   `yaml:"key,omitempty"`
	Value    string   `yaml:"value,omitempty"`
	In       string   `yaml:"in,omitempty"`
}

type Request struct {
	Name    string    `yaml:"name"`
	Method  string    `yaml:"method"`
	URL     string    `yaml:"url"`
	Params  []KV      `yaml:"params,omitempty"`
	Headers []KV      `yaml:"headers,omitempty"`
	Auth    Auth      `yaml:"auth,omitempty"`
	Body    Body      `yaml:"body,omitempty"`
	Path    string    `yaml:"-" json:"-"`
	Parent  *Folder   `yaml:"-" json:"-"`
	ModTime time.Time `yaml:"-" json:"-"`
}

type Folder struct {
	Name     string
	Path     string
	Vars     map[string]string
	Auth     Auth
	Parent   *Folder
	Folders  []*Folder
	Requests []*Request
}

type Env struct {
	Name    string
	Vars    map[string]string
	Secrets map[string]string
}

type FileError struct {
	Path string
	Err  error
}

type Workspace struct {
	Root          string
	Globals       map[string]string
	SharedSecrets map[string]string
	Envs          []*Env
	Collections   *Folder
	LoadErrors    []FileError
}
