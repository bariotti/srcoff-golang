package file

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// ConfiguracaoRepo persiste configurações chave→valor (ex: pastas monitoradas)
// em JSON.
type ConfiguracaoRepo struct {
	mu   sync.Mutex
	path string
}

func NewConfiguracaoRepo(dir string) *ConfiguracaoRepo {
	if err := os.MkdirAll(dir, 0755); err != nil {
		panic("file store: não foi possível criar diretório " + dir + ": " + err.Error())
	}
	return &ConfiguracaoRepo{path: filepath.Join(dir, "configuracoes.json")}
}

func (r *ConfiguracaoRepo) load() (map[string]string, error) {
	data, err := os.ReadFile(r.path)
	if os.IsNotExist(err) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, err
	}
	m := map[string]string{}
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return m, nil
}

func (r *ConfiguracaoRepo) save(m map[string]string) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(r.path, data, 0644)
}

func (r *ConfiguracaoRepo) Obter(_ context.Context, chave string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	m, err := r.load()
	if err != nil {
		return "", err
	}
	return m[chave], nil
}

func (r *ConfiguracaoRepo) Definir(_ context.Context, chave, valor string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	m, err := r.load()
	if err != nil {
		return err
	}
	m[chave] = valor
	return r.save(m)
}

func (r *ConfiguracaoRepo) ListarTodas(_ context.Context) (map[string]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.load()
}
