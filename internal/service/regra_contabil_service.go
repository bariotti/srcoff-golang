package service

import (
	"context"
	"fmt"

	"srcoff/internal/model"
)

type regraContabilRepoFull interface {
	ListarRegrasAtivas(ctx context.Context) ([]model.RegraContabil, error)
	CriarRegra(ctx context.Context, regra model.RegraContabil) (int64, error)
	EditarRegra(ctx context.Context, regra model.RegraContabil) error
	ListarCondicoes(ctx context.Context, idRegra int64) ([]model.CondicaoRegra, error)
	CriarCondicao(ctx context.Context, condicao model.CondicaoRegra) (int64, error)
	EditarCondicao(ctx context.Context, condicao model.CondicaoRegra) error
	ExcluirCondicao(ctx context.Context, id int64) error
}

type RegraContabilService struct {
	repo regraContabilRepoFull
}

func NewRegraContabilService(repo regraContabilRepoFull) *RegraContabilService {
	return &RegraContabilService{repo: repo}
}

func (s *RegraContabilService) ListarRegras(ctx context.Context) ([]model.RegraContabil, error) {
	return s.repo.ListarRegrasAtivas(ctx)
}

func (s *RegraContabilService) CriarRegra(ctx context.Context, regra model.RegraContabil) (int64, error) {
	if regra.Descricao == "" {
		return 0, fmt.Errorf("campo obrigatório ausente: descricao")
	}
	if regra.CodigoProdutoCorporativo == "" {
		return 0, fmt.Errorf("campo obrigatório ausente: codigo_produto_corporativo")
	}
	return s.repo.CriarRegra(ctx, regra)
}

func (s *RegraContabilService) EditarRegra(ctx context.Context, regra model.RegraContabil) error {
	if regra.ID == 0 {
		return fmt.Errorf("id da regra é obrigatório")
	}
	if regra.Descricao == "" {
		return fmt.Errorf("campo obrigatório ausente: descricao")
	}
	return s.repo.EditarRegra(ctx, regra)
}

func (s *RegraContabilService) ListarCondicoes(ctx context.Context, idRegra int64) ([]model.CondicaoRegra, error) {
	if idRegra == 0 {
		return nil, fmt.Errorf("id da regra é obrigatório")
	}
	return s.repo.ListarCondicoes(ctx, idRegra)
}

func (s *RegraContabilService) CriarCondicao(ctx context.Context, condicao model.CondicaoRegra) (int64, error) {
	if condicao.Condicao == "" {
		return 0, fmt.Errorf("campo obrigatório ausente: condicao")
	}
	if condicao.ContaDebito == "" {
		return 0, fmt.Errorf("campo obrigatório ausente: conta_debito")
	}
	if condicao.ContaCredito == "" {
		return 0, fmt.Errorf("campo obrigatório ausente: conta_credito")
	}
	if condicao.CampoValor == "" {
		return 0, fmt.Errorf("campo obrigatório ausente: campo_valor")
	}
	if condicao.CampoMoeda == "" {
		return 0, fmt.Errorf("campo obrigatório ausente: campo_moeda")
	}
	return s.repo.CriarCondicao(ctx, condicao)
}

func (s *RegraContabilService) EditarCondicao(ctx context.Context, condicao model.CondicaoRegra) error {
	if condicao.ID == 0 {
		return fmt.Errorf("id da condição é obrigatório")
	}
	if condicao.Condicao == "" {
		return fmt.Errorf("campo obrigatório ausente: condicao")
	}
	if condicao.ContaDebito == "" {
		return fmt.Errorf("campo obrigatório ausente: conta_debito")
	}
	if condicao.ContaCredito == "" {
		return fmt.Errorf("campo obrigatório ausente: conta_credito")
	}
	if condicao.CampoValor == "" {
		return fmt.Errorf("campo obrigatório ausente: campo_valor")
	}
	if condicao.CampoMoeda == "" {
		return fmt.Errorf("campo obrigatório ausente: campo_moeda")
	}
	return s.repo.EditarCondicao(ctx, condicao)
}

func (s *RegraContabilService) ExcluirCondicao(ctx context.Context, id int64) error {
	if id == 0 {
		return fmt.Errorf("id da condição é obrigatório")
	}
	return s.repo.ExcluirCondicao(ctx, id)
}
