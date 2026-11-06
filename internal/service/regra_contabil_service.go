package service

import (
	"context"
	"fmt"
	"strings"

	"srcoff/internal/evaluator"
	"srcoff/internal/model"
)

type regraContabilRepoFull interface {
	ListarRegrasAtivas(ctx context.Context) ([]model.RegraContabil, error)
	ListarRegras(ctx context.Context) ([]model.RegraContabil, error)
	CriarRegra(ctx context.Context, regra model.RegraContabil) (int64, error)
	EditarRegra(ctx context.Context, regra model.RegraContabil) error
	ExcluirRegra(ctx context.Context, id int64) error
	ListarCondicoes(ctx context.Context, idRegra int64) ([]model.CondicaoRegra, error)
	ListarCondicoesVisiveis(ctx context.Context, idRegra int64) ([]model.CondicaoRegra, error)
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

// ListarRegras devolve as regras visíveis na UI (ativa + rascunho; sem excluídas) — RF-125.
func (s *RegraContabilService) ListarRegras(ctx context.Context) ([]model.RegraContabil, error) {
	return s.repo.ListarRegras(ctx)
}

// normalizarCamposRegra normaliza os nomes de coluna e as expressões de uma regra (e suas
// condições) para o mesmo formato da importação da posição (snake_case sem acento), para que
// casem com as colunas normalizadas da posição. Expressões vão por NormalizarExpressao (que
// normaliza só os identificadores); campos que são um nome de coluna único vão por
// NormalizarNome. Usada no cadastro (Criar/Editar) e na simulação (WhatIf).
func normalizarCamposRegra(r *model.RegraContabil) {
	r.CampoProduto = evaluator.NormalizarNome(r.CampoProduto)
	r.PreCondicao = evaluator.NormalizarExpressao(r.PreCondicao)
	for i := range r.Condicoes {
		normalizarCamposCondicao(&r.Condicoes[i])
	}
}

func normalizarCamposCondicao(c *model.CondicaoRegra) {
	c.Condicao = evaluator.NormalizarExpressao(c.Condicao)
	c.CampoValor = evaluator.NormalizarExpressao(c.CampoValor)
	c.CampoValorMoedaBase = evaluator.NormalizarExpressao(c.CampoValorMoedaBase)
	c.CampoMoeda = evaluator.NormalizarNome(c.CampoMoeda)
	c.CampoBoleto = evaluator.NormalizarNome(c.CampoBoleto)
	c.CampoMoedaBase = evaluator.NormalizarNome(c.CampoMoedaBase)
	c.CampoDataConversao = evaluator.NormalizarNome(c.CampoDataConversao)
}

func (s *RegraContabilService) CriarRegra(ctx context.Context, regra model.RegraContabil) (int64, error) {
	if regra.Descricao == "" {
		return 0, fmt.Errorf("campo obrigatório ausente: descricao")
	}
	normalizarCamposRegra(&regra)
	// Produto é opcional: uma regra sem produto se aplica a todas as posições.
	return s.repo.CriarRegra(ctx, regra)
}

// ClonarRegra duplica uma regra e todas as suas condições, criando uma nova regra com a
// descrição sufixada " (cópia)". Spec: RF-071.
func (s *RegraContabilService) ClonarRegra(ctx context.Context, id int64) (int64, error) {
	if id == 0 {
		return 0, fmt.Errorf("id da regra é obrigatório")
	}
	regras, err := s.repo.ListarRegras(ctx)
	if err != nil {
		return 0, err
	}
	var orig *model.RegraContabil
	for i := range regras {
		if regras[i].ID == id {
			orig = &regras[i]
			break
		}
	}
	if orig == nil {
		return 0, fmt.Errorf("regra %d não encontrada", id)
	}
	nova := *orig
	nova.ID = 0
	nova.Condicoes = nil
	nova.Descricao = strings.TrimSpace(orig.Descricao) + " (cópia)"
	novoID, err := s.repo.CriarRegra(ctx, nova)
	if err != nil {
		return 0, err
	}
	conds, err := s.repo.ListarCondicoesVisiveis(ctx, id)
	if err != nil {
		return novoID, err
	}
	for _, c := range conds {
		c.ID = 0
		c.IDRegra = novoID
		if _, err := s.repo.CriarCondicao(ctx, c); err != nil {
			return novoID, err
		}
	}
	return novoID, nil
}

// ClonarCondicao duplica uma condição dentro da mesma regra (localizada pelo id). Spec: RF-071.
func (s *RegraContabilService) ClonarCondicao(ctx context.Context, idCondicao int64) (int64, error) {
	if idCondicao == 0 {
		return 0, fmt.Errorf("id da condição é obrigatório")
	}
	regras, err := s.repo.ListarRegras(ctx)
	if err != nil {
		return 0, err
	}
	for _, rg := range regras {
		conds, err := s.repo.ListarCondicoesVisiveis(ctx, rg.ID)
		if err != nil {
			return 0, err
		}
		for _, c := range conds {
			if c.ID == idCondicao {
				c.ID = 0
				c.IDRegra = rg.ID
				return s.repo.CriarCondicao(ctx, c)
			}
		}
	}
	return 0, fmt.Errorf("condição %d não encontrada", idCondicao)
}

func (s *RegraContabilService) EditarRegra(ctx context.Context, regra model.RegraContabil) error {
	if regra.ID == 0 {
		return fmt.Errorf("id da regra é obrigatório")
	}
	if regra.Descricao == "" {
		return fmt.Errorf("campo obrigatório ausente: descricao")
	}
	normalizarCamposRegra(&regra)
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
	// UI: inclui condições em rascunho (sem excluídas). A geração usa repo.ListarCondicoes.
	return s.repo.ListarCondicoesVisiveis(ctx, idRegra)
}

func (s *RegraContabilService) CriarCondicao(ctx context.Context, condicao model.CondicaoRegra) (int64, error) {
	normalizarCamposCondicao(&condicao)
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
	normalizarCamposCondicao(&condicao)
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

// Lint analisa estaticamente as regras/condições ATIVAS e devolve achados (erro/aviso):
// regra sem condição, expressão (pré-condição/condição/campo valor) que não compila,
// conta de débito == crédito e condições duplicadas no mesmo escopo. Spec: RF-129.
func (s *RegraContabilService) Lint(ctx context.Context) (*model.LintResultado, error) {
	regras, err := s.repo.ListarRegrasAtivas(ctx)
	if err != nil {
		return nil, err
	}
	res := &model.LintResultado{Achados: []model.LintAchado{}}
	add := func(sev, tipo string, idRegra int64, regra string, idCond int64, det string) {
		res.Achados = append(res.Achados, model.LintAchado{Severidade: sev, Tipo: tipo, IDRegra: idRegra, Regra: regra, IDCondicao: idCond, Detalhe: det})
	}
	compila := func(expr string) error {
		_, err := evaluator.CamposReferenciados(expr)
		return err
	}
	vistos := map[string]string{} // chave de escopo+condição+contas → origem (para detectar duplicatas)
	for _, r := range regras {
		if pc := strings.TrimSpace(r.PreCondicao); pc != "" {
			if err := compila(pc); err != nil {
				add("erro", "pre_condicao_invalida", r.ID, r.Descricao, 0, "pré-condição não compila: "+err.Error())
			}
		}
		if len(r.Condicoes) == 0 {
			add("aviso", "sem_condicao", r.ID, r.Descricao, 0, "regra ativa sem condição ativa — não gera lançamentos")
		}
		for _, c := range r.Condicoes {
			if strings.TrimSpace(c.Condicao) != "" {
				if err := compila(c.Condicao); err != nil {
					add("erro", "condicao_invalida", r.ID, r.Descricao, c.ID, "condição não compila: "+err.Error())
				}
			}
			if strings.TrimSpace(c.CampoValor) != "" {
				if err := compila(c.CampoValor); err != nil {
					add("erro", "campo_valor_invalido", r.ID, r.Descricao, c.ID, "campo valor não compila: "+err.Error())
				}
			}
			if c.ContaDebito != "" && c.ContaDebito == c.ContaCredito {
				add("erro", "contas_iguais", r.ID, r.Descricao, c.ID, "conta de débito e crédito iguais ("+c.ContaDebito+")")
			}
			chave := strings.ToLower(r.CodigoProdutoCorporativo + "|" + r.Dominio + "|" + strings.TrimSpace(c.Condicao) + "|" + c.ContaDebito + "|" + c.ContaCredito)
			if origem, ok := vistos[chave]; ok {
				add("aviso", "duplicada", r.ID, r.Descricao, c.ID, "condição duplicada de "+origem+" (mesmo produto/domínio, expressão e contas)")
			} else {
				vistos[chave] = fmt.Sprintf("regra #%d", r.ID)
			}
		}
	}
	res.Total = len(res.Achados)
	return res, nil
}
