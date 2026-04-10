// Package file implementa os repositórios usando arquivos JSON como backend de persistência.
package file

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// store é um helper genérico para ler/escrever um slice de T em um arquivo JSON.
type store[T any] struct {
	mu   sync.Mutex
	path string
}

func newStore[T any](dir, name string) *store[T] {
	if err := os.MkdirAll(dir, 0755); err != nil {
		panic("file store: não foi possível criar diretório " + dir + ": " + err.Error())
	}
	return &store[T]{path: filepath.Join(dir, name)}
}

func (s *store[T]) load() ([]T, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return []T{}, nil
	}
	if err != nil {
		return nil, err
	}
	var items []T
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, err
	}
	return items, nil
}

func (s *store[T]) save(items []T) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0644)
}
