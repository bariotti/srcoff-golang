package file

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// parametrizacaoStore persiste as opções parametrizáveis por categoria em JSON,
// no formato {"produto": ["NDF","SWAP"], "dominio": [...]}.
type parametrizacaoStore struct {
	mu   sync.Mutex
	path string
}

// ParametrizacaoRepo implementa ParametrizacaoRepository usando arquivo JSON.
type ParametrizacaoRepo struct {
	st *parametrizacaoStore
}

// opcoesPadrao são criadas na primeira execução para que os combos não iniciem vazios.
var opcoesPadrao = map[string][]string{
	"produto": {"NDF", "SWAP", "FXO"},
	"dominio": {"Posição", "Liquidação"},
}

func NewParametrizacaoRepo(dir string) *ParametrizacaoRepo {
	if err := os.MkdirAll(dir, 0755); err != nil {
		panic("file store: não foi possível criar diretório " + dir + ": " + err.Error())
	}
	st := &parametrizacaoStore{path: filepath.Join(dir, "parametrizacoes.json")}
	// Semeia opções padrão se o arquivo ainda não existe.
	if _, err := os.Stat(st.path); os.IsNotExist(err) {
		_ = st.save(opcoesPadrao)
	} else if m, err := st.load(); err == nil {
		// Instalação existente: acrescenta categorias padrão que ainda não existam.
		alterado := false
		for cat, vals := range opcoesPadrao {
			if _, ok := m[cat]; !ok {
				m[cat] = vals
				alterado = true
			}
		}
		if alterado {
			_ = st.save(m)
		}
	}
	return &ParametrizacaoRepo{st: st}
}

func (s *parametrizacaoStore) load() (map[string][]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return map[string][]string{}, nil
	}
	if err != nil {
		return nil, err
	}
	m := map[string][]string{}
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return m, nil
}

func (s *parametrizacaoStore) save(m map[string][]string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0644)
}

func (r *ParametrizacaoRepo) ListarOpcoes(_ context.Context, categoria string) ([]string, error) {
	m, err := r.st.load()
	if err != nil {
		return nil, err
	}
	opcoes := m[categoria]
	if opcoes == nil {
		opcoes = []string{}
	}
	return opcoes, nil
}

func (r *ParametrizacaoRepo) AdicionarOpcao(_ context.Context, categoria, valor string) error {
	m, err := r.st.load()
	if err != nil {
		return err
	}
	for _, o := range m[categoria] {
		if o == valor {
			return nil // já existe, idempotente
		}
	}
	m[categoria] = append(m[categoria], valor)
	return r.st.save(m)
}

func (r *ParametrizacaoRepo) RemoverOpcao(_ context.Context, categoria, valor string) error {
	m, err := r.st.load()
	if err != nil {
		return err
	}
	var filtrada []string
	for _, o := range m[categoria] {
		if o != valor {
			filtrada = append(filtrada, o)
		}
	}
	m[categoria] = filtrada
	return r.st.save(m)
}
