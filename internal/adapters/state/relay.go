package state

import (
	"errors"
	"os"
	"path/filepath"
)

type Relay struct{ path string }

func NewRelay(dir string) (*Relay, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	return &Relay{path: filepath.Join(dir, "relay.json")}, nil
}

func (s *Relay) Load() ([]byte, error) {
	b, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	return b, err
}

func (s *Relay) Save(data []byte) error { return writeAtomic(s.path, data, 0600) }

func SaveRelayFile(path string, data []byte) error { return writeAtomic(path, data, 0600) }
