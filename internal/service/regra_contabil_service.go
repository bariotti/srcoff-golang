package service

import (
	"context"
	"fmt"
	"strings"

	"srcoff/internal/model"
)

type regraContabilRepoFull interface {
	ListarRegrasAtivas(ctx context.Context) ([]model.RegraContabil, error)
	CriarRegra(ctx context.Context, regra model.RegraContabil) (int64, error)
	EditarRegra(ctx context.Context, regra model.RegraContabil) error
	ExcluirRegra(ctx context.Context, id int64) error
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
	// Produto é opcional: uma regra sem produto se aplica a todas as posições.
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

func (s *RegraContabilService) ExcluirRegra(ctx context.Context, id int64) error {
	if id == 0 {
		return fmt.Errorf("id da regra é obrigatório")
	}
	return s.repo.ExcluirRegra(ctx, id)
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
	if err := validarConfigMoedaBase(&condicao); err != nil {
		return 0, err
	}
	return s.repo.CriarCondicao(ctx, condicao)
}

// validarConfigMoedaBase valida e normaliza a configuração obrigatória da moeda base do
// veículo na condição (RF-036/RN-182). Quando a base vem da posição, exige as colunas de
// valor e moeda base (e ignora a data de conversão); caso contrário, exige a coluna da data
// base de conversão (e ignora as outras duas).
func validarConfigMoedaBase(c *model.CondicaoRegra) error {
	c.CampoValorMoedaBase = strings.TrimSpace(c.CampoValorMoedaBase)
	c.CampoMoedaBase = strings.TrimSpace(c.CampoMoedaBase)
	c.CampoDataConversao = strings.TrimSpace(c.CampoDataConversao)
	if c.BaseNaPosicao {
		if c.CampoValorMoedaBase == "" || c.CampoMoedaBase == "" {
			return fmt.Errorf("informe as colunas do valor e da moeda base do veículo (base vem da posição)")
		}
		c.CampoDataConversao = "" // não usado quando vem da posição
	} else {
		if c.CampoDataConversao == "" {
			return fmt.Errorf("informe a coluna da data base para conversão da moeda base do veículo")
		}
		c.CampoValorMoedaBase = ""
		c.CampoMoedaBase = ""
	}
	return nil
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
	if err := validarConfigMoedaBase(&condicao); err != nil {
		return err
	}
	return s.repo.EditarCondicao(ctx, condicao)
}

func (s *RegraContabilService) ExcluirCondicao(ctx context.Context, id int64) error {
	if id == 0 {
		return fmt.Errorf("id da condição é obrigatório")
	}
	return s.repo.ExcluirCondicao(ctx, id)
}
