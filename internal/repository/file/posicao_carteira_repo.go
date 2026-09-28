package file

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"srcoff/internal/model"
)

// posicaoStore armazena posições como mapas genéricos para preservar campos dinâmicos.
type posicaoStore struct {
	mu   sync.Mutex
	path string
}

func newPosicaoStore(dir, name string) *posicaoStore {
	if err := os.MkdirAll(dir, 0755); err != nil {
		panic("file store: não foi possível criar diretório " + dir + ": " + err.Error())
	}
	return &posicaoStore{path: filepath.Join(dir, name)}
}

func (s *posicaoStore) load() ([]map[string]interface{}, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return []map[string]interface{}{}, nil
	}
	if err != nil {
		return nil, err
	}
	var items []map[string]interface{}
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, err
	}
	return items, nil
}

func (s *posicaoStore) save(items []map[string]interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0644)
}

// PosicaoCarteiraRepo implementa PosicaoCarteiraRepository usando arquivo JSON.
// Cada registro é um mapa flat que preserva os metadados do lote (id, data, versão)
// e todos os campos de negócio dinâmicos.
type PosicaoCarteiraRepo struct {
	st *posicaoStore
}

func NewPosicaoCarteiraRepo(dir string) *PosicaoCarteiraRepo {
	return &PosicaoCarteiraRepo{st: newPosicaoStore(dir, "posicao_carteira.json")}
}

// metaKeys são as chaves de metadados do lote, não expostas como campo de negócio
// mas mantidas em Campos para compatibilidade com expressões que as utilizem.
func mapToPosicao(m map[string]interface{}) model.PosicaoCarteira {
	p := model.PosicaoCarteira{}
	if v, ok := m["id"]; ok {
		p.ID = toInt64Val(v)
	}
	if v, ok := m["codigo_versao_conteudo"]; ok {
		p.CodigoVersaoConteudo = int(toInt64Val(v))
	}
	if v, ok := m["data_posicao_carteira"]; ok {
		if s, ok := v.(string); ok {
			if t, err := time.Parse(time.RFC3339, s); err == nil {
				p.DataPosicaoCarteira = t
			} else if t, err := time.Parse("2006-01-02", s); err == nil {
				p.DataPosicaoCarteira = t
			}
		}
	}

	// Campos — TODOS os campos do registro, incluindo dinâmicos vindos de upload.
	campos := make(map[string]interface{}, len(m))
	for k, v := range m {
		switch val := v.(type) {
		case nil:
			campos[k] = float64(0)
		default:
			campos[k] = val
		}
	}
	campos["data_posicao_carteira"] = p.DataPosicaoCarteira
	p.Campos = campos
	return p
}

func (r *PosicaoCarteiraRepo) BuscarPorDataEVersaoMaxima(_ context.Context, data time.Time) ([]model.PosicaoCarteira, error) {
	all, err := r.st.load()
	if err != nil {
		return nil, err
	}
	dataStr := data.Format("2006-01-02")

	// Snapshot vigente = para cada (produto, domínio), a maior versão daquela data.
	// Combinações distintas versionam de forma independente e coexistem no resultado.
	var doDia []model.PosicaoCarteira
	maxPorCombo := map[string]int{}
	for _, m := range all {
		p := mapToPosicao(m)
		if p.DataPosicaoCarteira.Format("2006-01-02") != dataStr {
			continue
		}
		doDia = append(doDia, p)
		k := comboKeyPos(p)
		if p.CodigoVersaoConteudo > maxPorCombo[k] {
			maxPorCombo[k] = p.CodigoVersaoConteudo
		}
	}

	var result []model.PosicaoCarteira
	for _, p := range doDia {
		if p.CodigoVersaoConteudo == maxPorCombo[comboKeyPos(p)] {
			result = append(result, p)
		}
	}
	return result, nil
}

// campoStrPos lê um campo string da posição.
func campoStrPos(p model.PosicaoCarteira, campo string) string {
	if v, ok := p.Campos[campo]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// comboKeyPos identifica a combinação (produto, domínio) da posição para versionamento.
func comboKeyPos(p model.PosicaoCarteira) string {
	return campoStrPos(p, "produto") + "\x00" + campoStrPos(p, "dominio")
}

func (r *PosicaoCarteiraRepo) ListarPorData(_ context.Context, data time.Time) ([]model.PosicaoCarteira, error) {
	all, err := r.st.load()
	if err != nil {
		return nil, err
	}
	dataStr := data.Format("2006-01-02")
	var result []model.PosicaoCarteira
	for _, m := range all {
		p := mapToPosicao(m)
		if p.DataPosicaoCarteira.Format("2006-01-02") == dataStr {
			result = append(result, p)
		}
	}
	return result, nil
}

func (r *PosicaoCarteiraRepo) ListarPorPeriodo(_ context.Context, dataInicio, dataFim time.Time) ([]model.PosicaoCarteira, error) {
	all, err := r.st.load()
	if err != nil {
		return nil, err
	}
	inicioStr := dataInicio.Format("2006-01-02")
	fimStr := dataFim.Format("2006-01-02")
	var result []model.PosicaoCarteira
	for _, m := range all {
		p := mapToPosicao(m)
		d := p.DataPosicaoCarteira.Format("2006-01-02")
		if d >= inicioStr && d <= fimStr {
			result = append(result, p)
		}
	}
	return result, nil
}

// ImportarLote persiste um conjunto de registros de posição para uma data/versão.
// Cada registro é gravado como um mapa flat com os metadados do lote somados aos
// campos dinâmicos do arquivo.
func (r *PosicaoCarteiraRepo) ImportarLote(_ context.Context, data time.Time, versao int, registros []map[string]interface{}) error {
	all, err := r.st.load()
	if err != nil {
		return err
	}
	maxID := int64(0)
	for _, m := range all {
		if id := toInt64Val(m["id"]); id > maxID {
			maxID = id
		}
	}
	dataStr := data.Format("2006-01-02")
	for _, reg := range registros {
		maxID++
		m := make(map[string]interface{}, len(reg)+3)
		for k, v := range reg {
			m[k] = v
		}
		m["id"] = float64(maxID)
		m["data_posicao_carteira"] = dataStr
		m["codigo_versao_conteudo"] = float64(versao)
		all = append(all, m)
	}
	return r.st.save(all)
}

// helpers de conversão de tipos JSON
func toInt64Val(v interface{}) int64 {
	switch val := v.(type) {
	case float64:
		return int64(val)
	case int64:
		return val
	case int:
		return int64(val)
	}
	return 0
}
