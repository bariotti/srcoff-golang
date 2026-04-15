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
// Usa map[string]interface{} para preservar campos dinâmicos adicionados ao JSON.
type PosicaoCarteiraRepo struct {
	st *posicaoStore
}

func NewPosicaoCarteiraRepo(dir string) *PosicaoCarteiraRepo {
	return &PosicaoCarteiraRepo{st: newPosicaoStore(dir, "posicao_carteira.json")}
}

// mapToPosicao converte um mapa genérico para PosicaoCarteira, preservando todos os campos em Campos.
func mapToPosicao(m map[string]interface{}) model.PosicaoCarteira {
	p := model.PosicaoCarteira{}

	// Campos fixos
	if v, ok := m["id"]; ok {
		p.ID = toInt64Val(v)
	}
	if v, ok := m["codigo_versao_conteudo"]; ok {
		p.CodigoVersaoConteudo = int(toInt64Val(v))
	}
	if v, ok := m["codigo_identificador_boleto"]; ok {
		p.CodigoIdentificadorBoleto = toStrVal(v)
	}
	if v, ok := m["descricao_veiculo"]; ok {
		p.DescricaoVeiculo = toStrVal(v)
	}
	if v, ok := m["indicador_contraparte_afiliada"]; ok {
		p.IndicadorContraparteAfiliada = toBoolVal(v)
	}
	if v, ok := m["valor_mtm"]; ok {
		p.ValorMTM = toFloat64Val(v)
	}
	if v, ok := m["principal_remanescente"]; ok {
		p.PrincipalRemanescente = toFloat64Val(v)
	}
	if v, ok := m["moeda_principal_remanescente"]; ok {
		p.MoedaPrincipalRemanescente = toStrVal(v)
	}
	if v, ok := m["produto"]; ok {
		p.Produto = toStrVal(v)
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

	// Campos — inclui TODOS os campos do mapa, incluindo dinâmicos como agio
	campos := make(map[string]interface{}, len(m))
	for k, v := range m {
		// Normalizar números para float64 (JSON deserializa como float64 por padrão)
		switch val := v.(type) {
		case float64:
			campos[k] = val
		case bool:
			campos[k] = val
		case string:
			campos[k] = val
		case nil:
			campos[k] = float64(0)
		default:
			campos[k] = v
		}
	}
	// Garantir que data_posicao_carteira seja time.Time no mapa
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

	maxVersao := 0
	for _, m := range all {
		p := mapToPosicao(m)
		if p.DataPosicaoCarteira.Format("2006-01-02") == dataStr && p.CodigoVersaoConteudo > maxVersao {
			maxVersao = p.CodigoVersaoConteudo
		}
	}

	var result []model.PosicaoCarteira
	for _, m := range all {
		p := mapToPosicao(m)
		if p.DataPosicaoCarteira.Format("2006-01-02") == dataStr && p.CodigoVersaoConteudo == maxVersao {
			result = append(result, p)
		}
	}
	return result, nil
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

func (r *PosicaoCarteiraRepo) Inserir(_ context.Context, p model.PosicaoCarteira) (int64, error) {
	all, err := r.st.load()
	if err != nil {
		return 0, err
	}
	maxID := int64(0)
	for _, m := range all {
		if id := toInt64Val(m["id"]); id > maxID {
			maxID = id
		}
	}
	newID := maxID + 1

	m := map[string]interface{}{
		"id":                              float64(newID),
		"data_posicao_carteira":           p.DataPosicaoCarteira.Format("2006-01-02"),
		"codigo_versao_conteudo":          float64(p.CodigoVersaoConteudo),
		"codigo_identificador_boleto":     p.CodigoIdentificadorBoleto,
		"descricao_veiculo":               p.DescricaoVeiculo,
		"indicador_contraparte_afiliada":  p.IndicadorContraparteAfiliada,
		"valor_mtm":                       p.ValorMTM,
		"principal_remanescente":          p.PrincipalRemanescente,
		"moeda_principal_remanescente":    p.MoedaPrincipalRemanescente,
		"produto":                         p.Produto,
	}
	all = append(all, m)
	return newID, r.st.save(all)
}

func (r *PosicaoCarteiraRepo) Deletar(_ context.Context, id int64) error {
	all, err := r.st.load()
	if err != nil {
		return err
	}
	var filtered []map[string]interface{}
	for _, m := range all {
		if toInt64Val(m["id"]) != id {
			filtered = append(filtered, m)
		}
	}
	return r.st.save(filtered)
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

func toFloat64Val(v interface{}) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case int64:
		return float64(val)
	case int:
		return float64(val)
	}
	return 0
}

func toStrVal(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func toBoolVal(v interface{}) bool {
	if b, ok := v.(bool); ok {
		return b
	}
	return false
}
