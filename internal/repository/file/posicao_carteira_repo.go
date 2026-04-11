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

func (r *PosicaoCarteiraRepo) ListarPorData(_ context.Context, data time.Time) ([]model.PosicaoCarteira, error) {
	all, err := r.st.load()
	if err != nil {
		return nil, err
	}
	dataStr := data.Format("2006-01-02")
	var result []model.PosicaoCarteira
	for _, p := range all {
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
	for _, item := range all {
		if item.ID > maxID {
			maxID = item.ID
		}
	}
	p.ID = maxID + 1
	all = append(all, p)
	return p.ID, r.st.save(all)
}

func (r *PosicaoCarteiraRepo) Deletar(_ context.Context, id int64) error {
	all, err := r.st.load()
	if err != nil {
		return err
	}
	filtered := all[:0]
	for _, p := range all {
		if p.ID != id {
			filtered = append(filtered, p)
		}
	}
	return r.st.save(filtered)
}
