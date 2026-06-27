package file

import (
	"context"
	"fmt"

	"srcoff/internal/model"
)

type regraStore struct {
	Regras    []model.RegraContabil `json:"regras"`
	Condicoes []model.CondicaoRegra `json:"condicoes"`
	NextRegra int64                 `json:"next_regra"`
	NextCond  int64                 `json:"next_cond"`
}

// RegraContabilRepo implementa RegraContabilRepository usando arquivo JSON.
type RegraContabilRepo struct {
	st *store[regraStore]
}

func NewRegraContabilRepo(dir string) *RegraContabilRepo {
	return &RegraContabilRepo{st: newStore[regraStore](dir, "regras.json")}
}

func (r *RegraContabilRepo) load() (regraStore, error) {
	items, err := r.st.load()
	if err != nil {
		return regraStore{}, err
	}
	if len(items) == 0 {
		return regraStore{NextRegra: 1, NextCond: 1}, nil
	}
	return items[0], nil
}

func (r *RegraContabilRepo) save(s regraStore) error {
	return r.st.save([]regraStore{s})
}

func (r *RegraContabilRepo) ListarRegrasAtivas(_ context.Context) ([]model.RegraContabil, error) {
	s, err := r.load()
	if err != nil {
		return nil, err
	}
	var result []model.RegraContabil
	for _, reg := range s.Regras {
		if !reg.Ativo {
			continue
		}
		var condicoes []model.CondicaoRegra
		for _, c := range s.Condicoes {
			if c.IDRegra == reg.ID && c.Ativo {
				condicoes = append(condicoes, c)
			}
		}
		reg.Condicoes = condicoes
		result = append(result, reg)
	}
	return result, nil
}

func (r *RegraContabilRepo) CriarRegra(_ context.Context, regra model.RegraContabil) (int64, error) {
	s, err := r.load()
	if err != nil {
		return 0, err
	}
	regra.ID = s.NextRegra
	regra.Ativo = true
	if !regra.PostaReverte {
		regra.PostaReverte = false
	}
	s.NextRegra++
	s.Regras = append(s.Regras, regra)
	return regra.ID, r.save(s)
}

func (r *RegraContabilRepo) EditarRegra(_ context.Context, regra model.RegraContabil) error {
	s, err := r.load()
	if err != nil {
		return err
	}
	for i, reg := range s.Regras {
		if reg.ID == regra.ID {
			s.Regras[i].Descricao = regra.Descricao
			s.Regras[i].CodigoProdutoCorporativo = regra.CodigoProdutoCorporativo
			s.Regras[i].Ativo = regra.Ativo
			return r.save(s)
		}
	}
	return fmt.Errorf("regra %d não encontrada", regra.ID)
}

func (r *RegraContabilRepo) ListarCondicoes(_ context.Context, idRegra int64) ([]model.CondicaoRegra, error) {
	s, err := r.load()
	if err != nil {
		return nil, err
	}
	var result []model.CondicaoRegra
	for _, c := range s.Condicoes {
		if c.IDRegra == idRegra && c.Ativo {
			result = append(result, c)
		}
	}
	return result, nil
}

func (r *RegraContabilRepo) CriarCondicao(_ context.Context, condicao model.CondicaoRegra) (int64, error) {
	s, err := r.load()
	if err != nil {
		return 0, err
	}
	condicao.ID = s.NextCond
	condicao.Ativo = true
	s.NextCond++
	s.Condicoes = append(s.Condicoes, condicao)
	return condicao.ID, r.save(s)
}

func (r *RegraContabilRepo) EditarCondicao(_ context.Context, condicao model.CondicaoRegra) error {
	s, err := r.load()
	if err != nil {
		return err
	}
	for i, c := range s.Condicoes {
		if c.ID == condicao.ID {
			s.Condicoes[i].Condicao = condicao.Condicao
			s.Condicoes[i].ContaDebito = condicao.ContaDebito
			s.Condicoes[i].ContaCredito = condicao.ContaCredito
			s.Condicoes[i].CampoValor = condicao.CampoValor
			s.Condicoes[i].CampoMoeda = condicao.CampoMoeda
			s.Condicoes[i].Ativo = condicao.Ativo
			return r.save(s)
		}
	}
	return fmt.Errorf("condição %d não encontrada", condicao.ID)
}

func (r *RegraContabilRepo) ExcluirCondicao(_ context.Context, id int64) error {
	s, err := r.load()
	if err != nil {
		return err
	}
	for i, c := range s.Condicoes {
		if c.ID == id {
			s.Condicoes[i].Ativo = false
			return r.save(s)
		}
	}
	return fmt.Errorf("condição %d não encontrada", id)
}
