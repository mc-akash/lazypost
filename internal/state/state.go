package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/mc-akash/lazypost/internal/paths"
	"github.com/mc-akash/lazypost/internal/workspace"
)

type Draft struct {
	Request     workspace.Request `json:"request"`
	BaseModTime time.Time         `json:"base_mod_time"`
}

type Session struct {
	LastEnv     string             `json:"last_env"`
	LastRequest string             `json:"last_request"`
	Drafts      map[string]Draft   `json:"drafts"`
	Scratch     *workspace.Request `json:"scratch,omitempty"`
}

type Store struct {
	path string
}

func Open(path string) *Store {
	return &Store{path: path}
}

func (s *Store) Load(root string) (Session, error) {
	all, err := s.readAll()
	if err != nil {
		return Session{Drafts: map[string]Draft{}}, err
	}
	sess := all[root]
	if sess.Drafts == nil {
		sess.Drafts = map[string]Draft{}
	}
	return sess, nil
}

func (s *Store) Save(root string, sess Session) error {
	all, err := s.readAll()
	if errors.Is(err, errCorrupt) {
		if rerr := os.Rename(s.path, s.path+".corrupt"); rerr != nil {
			return fmt.Errorf("state: move aside corrupt file: %w", rerr)
		}
		all, err = nil, nil
	}
	if err != nil {
		return err
	}
	if all == nil {
		all = map[string]Session{}
	}
	all[root] = sess
	data, err := json.MarshalIndent(all, "", "  ")
	if err != nil {
		return fmt.Errorf("state: encode: %w", err)
	}
	return writeAtomic(s.path, data)
}

var errCorrupt = errors.New("corrupt state file")

func (s *Store) readAll() (map[string]Session, error) {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("state: read %s: %w", s.path, err)
	}
	var all map[string]Session
	if err := json.Unmarshal(data, &all); err != nil {
		return nil, fmt.Errorf("state: parse %s: %w: %w", s.path, errCorrupt, err)
	}
	return all, nil
}

func writeAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("state: mkdir %s: %w", dir, err)
	}
	if err := paths.WriteAtomic(path, data, 0o600); err != nil {
		return fmt.Errorf("state: %w", err)
	}
	return nil
}
