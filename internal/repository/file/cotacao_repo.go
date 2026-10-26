package file

import (
	"context"
	"fmt"

	"srcoff/internal/model"
)

// CotacaoRepo implementa CotacaoRepository usando arquivo JSON.
type CotacaoRepo struct {
	st *store[model.Cotacao]
}

func NewCotacaoRepo(dir string) *CotacaoRepo {
	return &CotacaoRepo{st: newStore[model.Cotacao](dir, "cotacao.json")}
}

func (r *CotacaoRepo) Listar(_ context.Context) ([]model.Cotacao, error) {
	itens, err := r.st.load()
	if err != nil {
		return nil, err
	}
	if itens == nil {
		itens = []model.Cotacao{}
	}
	return itens, nil
}

func (r *CotacaoRepo) Criar(_ context.Context, c model.Cotacao) (int64, error) {
	itens, err := r.st.load()
	if err != nil {
		return 0, err
	}
	maxID := int64(0)
	for _, i := range itens {
		if i.ID > maxID {
			maxID = i.ID
		}
	}
	c.ID = maxID + 1
	itens = append(itens, c)
	return c.ID, r.st.save(itens)
}

func (r *CotacaoRepo) Excluir(_ context.Context, id int64) error {
	itens, err := r.st.load()
	if err != nil {
		return err
	}
	var filtrados []model.Cotacao
	achou := false
	for _, i := range itens {
		if i.ID == id {
			achou = true
			continue
		}
		filtrados = append(filtrados, i)
	}
	if !achou {
		return fmt.Errorf("cotação %d não encontrada", id)
	}
	return r.st.save(filtrados)
}
