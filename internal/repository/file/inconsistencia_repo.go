package file

import (
	"context"
	"time"

	"srcoff/internal/model"
)

// InconsistenciaRepo implementa InconsistenciaRepository usando arquivo JSON.
type InconsistenciaRepo struct {
	st *store[model.InconsistenciaProcessamento]
}

func NewInconsistenciaRepo(dir string) *InconsistenciaRepo {
	return &InconsistenciaRepo{st: newStore[model.InconsistenciaProcessamento](dir, "inconsistencias.json")}
}

// SubstituirPorEscopo remove as inconsistências das combinações (data, produto, domínio)
// informadas e grava as novas, refletindo o último processamento de cada combinação.
func (r *InconsistenciaRepo) SubstituirPorEscopo(_ context.Context, data time.Time, combos []model.ProdutoDominio, itens []model.InconsistenciaProcessamento) error {
	all, err := r.st.load()
	if err != nil {
		return err
	}
	dataStr := data.Format("2006-01-02")
	noEscopo := map[string]bool{}
	for _, c := range combos {
		noEscopo[c.Produto+"\x00"+c.Dominio] = true
	}

	maxID := int64(0)
	var mantidos []model.InconsistenciaProcessamento
	for _, i := range all {
		if i.ID > maxID {
			maxID = i.ID
		}
		mesmaData := i.DataLoteContabil.Format("2006-01-02") == dataStr
		if mesmaData && noEscopo[i.Produto+"\x00"+i.Dominio] {
			continue // será substituída
		}
		mantidos = append(mantidos, i)
	}
	for idx := range itens {
		maxID++
		itens[idx].ID = maxID
		if itens[idx].CriadoEm.IsZero() {
			itens[idx].CriadoEm = time.Now()
		}
		mantidos = append(mantidos, itens[idx])
	}
	return r.st.save(mantidos)
}

func (r *InconsistenciaRepo) ListarPorData(_ context.Context, data time.Time) ([]model.InconsistenciaProcessamento, error) {
	all, err := r.st.load()
	if err != nil {
		return nil, err
	}
	dataStr := data.Format("2006-01-02")
	result := []model.InconsistenciaProcessamento{}
	for _, i := range all {
		if i.DataLoteContabil.Format("2006-01-02") == dataStr {
			result = append(result, i)
		}
	}
	return result, nil
}
