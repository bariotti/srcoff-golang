package file

import (
	"context"
	"fmt"

	"srcoff/internal/model"
)

// VeiculoDeParaRepo implementa VeiculoDeParaRepository usando arquivo JSON.
type VeiculoDeParaRepo struct {
	st *store[model.VeiculoDePara]
}

func NewVeiculoDeParaRepo(dir string) *VeiculoDeParaRepo {
	return &VeiculoDeParaRepo{st: newStore[model.VeiculoDePara](dir, "veiculo_de_para.json")}
}

func (r *VeiculoDeParaRepo) Listar(_ context.Context) ([]model.VeiculoDePara, error) {
	itens, err := r.st.load()
	if err != nil {
		return nil, err
	}
	if itens == nil {
		itens = []model.VeiculoDePara{}
	}
	return itens, nil
}

func (r *VeiculoDeParaRepo) Criar(_ context.Context, v model.VeiculoDePara) (int64, error) {
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
	v.ID = maxID + 1
	itens = append(itens, v)
	return v.ID, r.st.save(itens)
}

func (r *VeiculoDeParaRepo) Excluir(_ context.Context, id int64) error {
	itens, err := r.st.load()
	if err != nil {
		return err
	}
	var filtrados []model.VeiculoDePara
	achou := false
	for _, i := range itens {
		if i.ID == id {
			achou = true
			continue
		}
		filtrados = append(filtrados, i)
	}
	if !achou {
		return fmt.Errorf("de/para de veículo %d não encontrado", id)
	}
	return r.st.save(filtrados)
}
