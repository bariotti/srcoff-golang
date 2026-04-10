package file

import (
	"context"
	"time"

	"srcoff/internal/model"
)

// PosicaoCarteiraRepo implementa PosicaoCarteiraRepository usando arquivo JSON.
type PosicaoCarteiraRepo struct {
	st *store[model.PosicaoCarteira]
}

func NewPosicaoCarteiraRepo(dir string) *PosicaoCarteiraRepo {
	return &PosicaoCarteiraRepo{st: newStore[model.PosicaoCarteira](dir, "posicao_carteira.json")}
}

func (r *PosicaoCarteiraRepo) BuscarPorDataEVersaoMaxima(_ context.Context, data time.Time) ([]model.PosicaoCarteira, error) {
	all, err := r.st.load()
	if err != nil {
		return nil, err
	}
	dataStr := data.Format("2006-01-02")

	// Encontrar versão máxima para a data
	maxVersao := 0
	for _, p := range all {
		if p.DataPosicaoCarteira.Format("2006-01-02") == dataStr && p.CodigoVersaoConteudo > maxVersao {
			maxVersao = p.CodigoVersaoConteudo
		}
	}

	var result []model.PosicaoCarteira
	for _, p := range all {
		if p.DataPosicaoCarteira.Format("2006-01-02") == dataStr && p.CodigoVersaoConteudo == maxVersao {
			result = append(result, p)
		}
	}
	return result, nil
}
