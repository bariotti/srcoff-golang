package file

import (
	"context"
	"time"

	"srcoff/internal/model"
)

// ExecucaoRepo implementa ExecucaoRepository usando arquivo JSON.
type ExecucaoRepo struct {
	st *store[model.MovimentoExecucao]
}

func NewExecucaoRepo(dir string) *ExecucaoRepo {
	return &ExecucaoRepo{st: newStore[model.MovimentoExecucao](dir, "execucoes.json")}
}

// RegistrarExecucao remove a execução anterior daquele (data, produto, domínio) e
// grava a nova — reflete sempre o último processamento da combinação.
func (r *ExecucaoRepo) RegistrarExecucao(_ context.Context, e model.MovimentoExecucao) error {
	all, err := r.st.load()
	if err != nil {
		return err
	}
	dataStr := e.DataLote.Format("2006-01-02")
	maxID := int64(0)
	var mantidos []model.MovimentoExecucao
	for _, x := range all {
		if x.ID > maxID {
			maxID = x.ID
		}
		if x.DataLote.Format("2006-01-02") == dataStr && x.Produto == e.Produto && x.Dominio == e.Dominio {
			continue
		}
		mantidos = append(mantidos, x)
	}
	e.ID = maxID + 1
	if e.CriadoEm.IsZero() {
		e.CriadoEm = time.Now()
	}
	mantidos = append(mantidos, e)
	return r.st.save(mantidos)
}

func (r *ExecucaoRepo) ListarPorData(_ context.Context, data time.Time) ([]model.MovimentoExecucao, error) {
	all, err := r.st.load()
	if err != nil {
		return nil, err
	}
	dataStr := data.Format("2006-01-02")
	result := []model.MovimentoExecucao{}
	for _, x := range all {
		if x.DataLote.Format("2006-01-02") == dataStr {
			result = append(result, x)
		}
	}
	return result, nil
}
