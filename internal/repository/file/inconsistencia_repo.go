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

// SubstituirPorData remove as inconsistências existentes para a data e grava as novas,
// refletindo sempre o último processamento daquela data.
func (r *InconsistenciaRepo) SubstituirPorData(_ context.Context, data time.Time, itens []model.InconsistenciaProcessamento) error {
	all, err := r.st.load()
	if err != nil {
		return err
	}
	dataStr := data.Format("2006-01-02")

	maxID := int64(0)
	var mantidos []model.InconsistenciaProcessamento
	for _, i := range all {
		if i.ID > maxID {
			maxID = i.ID
		}
		if i.DataLoteContabil.Format("2006-01-02") != dataStr {
			mantidos = append(mantidos, i)
		}
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
