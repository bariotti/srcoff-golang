package file

import (
	"context"
	"fmt"

	"srcoff/internal/model"
)

// PadraoArquivoRepo implementa PadraoArquivoRepository usando arquivo JSON.
type PadraoArquivoRepo struct {
	st *store[model.PadraoArquivo]
}

func NewPadraoArquivoRepo(dir string) *PadraoArquivoRepo {
	return &PadraoArquivoRepo{st: newStore[model.PadraoArquivo](dir, "padroes_arquivo.json")}
}

func (r *PadraoArquivoRepo) Listar(_ context.Context) ([]model.PadraoArquivo, error) {
	itens, err := r.st.load()
	if err != nil {
		return nil, err
	}
	if itens == nil {
		itens = []model.PadraoArquivo{}
	}
	return itens, nil
}

func (r *PadraoArquivoRepo) Criar(_ context.Context, p model.PadraoArquivo) (int64, error) {
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
	p.ID = maxID + 1
	itens = append(itens, p)
	return p.ID, r.st.save(itens)
}

func (r *PadraoArquivoRepo) Excluir(_ context.Context, id int64) error {
	itens, err := r.st.load()
	if err != nil {
		return err
	}
	var filtrados []model.PadraoArquivo
	achou := false
	for _, i := range itens {
		if i.ID == id {
			achou = true
			continue
		}
		filtrados = append(filtrados, i)
	}
	if !achou {
		return fmt.Errorf("padrão %d não encontrado", id)
	}
	return r.st.save(filtrados)
}
