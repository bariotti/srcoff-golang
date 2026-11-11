package service

import (
	"context"
	"fmt"
	"log"
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

// regraVersaoRepo é a dependência OPCIONAL do histórico de versões (RF-133). Quando nil
// (backends file/sqlserver) o versionamento fica desligado e o cadastro funciona normalmente.
type regraVersaoRepo interface {
	Carregar(ctx context.Context, idRegra int64) (model.RegraContabil, bool, error)
	UltimaVersao(ctx context.Context, idRegra int64) (model.RegraVersao, bool, error)
	Snapshot(ctx context.Context, idRegra int64, regra model.RegraContabil, resumo, origem string) (int, error)
	Listar(ctx context.Context, idRegra int64) ([]model.RegraVersao, error)
	Obter(ctx context.Context, idRegra int64, versao int) (model.RegraVersao, bool, error)
	ResumoTodos(ctx context.Context) ([]model.RegraVersao, error)
}

type RegraContabilService struct {
	repo       regraContabilRepoFull
	versaoRepo regraVersaoRepo   // opcional (sqlite-first)
	auditoria  *AuditoriaService // opcional: trilha de auditoria (RF-136)
}

func NewRegraContabilService(repo regraContabilRepoFull) *RegraContabilService {
	return &RegraContabilService{repo: repo}
}

// ComVersaoRepo liga o histórico de versões do cadastro (RF-133). Sem ele a feature fica off.
func (s *RegraContabilService) ComVersaoRepo(r regraVersaoRepo) *RegraContabilService {
	s.versaoRepo = r
	return s
}

// ComAuditoria liga a trilha de auditoria global (RF-136) ao cadastro.
func (s *RegraContabilService) ComAuditoria(a *AuditoriaService) *RegraContabilService {
	s.auditoria = a
	return s
}

// HistoricoDisponivel informa se o versionamento de regras está ligado neste backend.
func (s *RegraContabilService) HistoricoDisponivel() bool { return s.versaoRepo != nil }

// capturarVersao grava um snapshot da regra APÓS uma mutação, com resumo derivado do diff
// contra a última versão. Best-effort: a mutação já foi persistida, então um erro aqui só é
// logado (não falha a requisição). Não cria versão quando nada mudou de fato.
func (s *RegraContabilService) capturarVersao(ctx context.Context, idRegra int64, origem string) {
	s.capturarVersaoComResumo(ctx, idRegra, "", origem)
}

// capturarVersaoComResumo é como capturarVersao, mas permite forçar o resumo (ex.: restauração).
// Com resumo vazio, o resumo é calculado pelo diff contra a última versão.
func (s *RegraContabilService) capturarVersaoComResumo(ctx context.Context, idRegra int64, resumo, origem string) {
	if s.versaoRepo == nil || idRegra == 0 {
		return
	}
	atual, ok, err := s.versaoRepo.Carregar(ctx, idRegra)
	if err != nil {
		log.Printf("[regra-versao] falha ao carregar regra %d para snapshot: %v", idRegra, err)
		return
	}
	if !ok {
		return
	}
	var anterior *model.RegraContabil
	if ult, ok, err := s.versaoRepo.UltimaVersao(ctx, idRegra); err == nil && ok {
		a := ult.Regra
		anterior = &a
	}
	if resumo == "" {
		resumo = model.ResumoAlteracao(anterior, atual)
		// Nada mudou de fato em relação à última versão → não cria ruído no histórico.
		if anterior != nil && resumo == "Sem alterações" {
			return
		}
	}
	if _, err := s.versaoRepo.Snapshot(ctx, idRegra, atual, resumo, origem); err != nil {
		log.Printf("[regra-versao] falha ao gravar snapshot da regra %d: %v", idRegra, err)
	}
}

// idRegraDaCondicao descobre a regra-mãe de uma condição (pelas regras visíveis). Usado para
// saber qual regra versionar quando a mutação é sobre uma condição. 0 se não encontrada.
func (s *RegraContabilService) idRegraDaCondicao(ctx context.Context, idCondicao int64) int64 {
	regras, err := s.repo.ListarRegras(ctx)
	if err != nil {
		return 0
	}
	for _, rg := range regras {
		for _, c := range rg.Condicoes {
			if c.ID == idCondicao {
				return rg.ID
			}
		}
	}
	return 0
}

// ListarRegras devolve as regras visíveis na UI (ativa + rascunho; sem excluídas) — RF-125.
func (s *RegraContabilService) ListarRegras(ctx context.Context) ([]model.RegraContabil, error) {
	return s.repo.ListarRegras(ctx)
}

// RegrasAtivas devolve as regras ATIVAS com as condições ATIVAS embutidas (a "porta" da
// geração). Usada pelo catálogo de regras (RF-134), relatório legível p/ contábil/auditoria.
func (s *RegraContabilService) RegrasAtivas(ctx context.Context) ([]model.RegraContabil, error) {
	return s.repo.ListarRegrasAtivas(ctx)
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
	novoID, err := s.repo.CriarRegra(ctx, regra)
	if err != nil {
		return 0, err
	}
	s.capturarVersao(ctx, novoID, "sistema")
	s.auditoria.Log(ctx, model.AudCadastro, "regra.criada", fmt.Sprintf("regra #%d", novoID), "sistema", regra.Descricao,
		map[string]interface{}{"id": novoID, "descricao": regra.Descricao, "produto": regra.CodigoProdutoCorporativo, "dominio": regra.Dominio})
	return novoID, nil
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
	s.capturarVersao(ctx, novoID, "sistema")
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
				novoID, err := s.repo.CriarCondicao(ctx, c)
				if err != nil {
					return 0, err
				}
				s.capturarVersao(ctx, rg.ID, "sistema")
				return novoID, nil
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
	if err := s.repo.EditarRegra(ctx, regra); err != nil {
		return err
	}
	s.capturarVersao(ctx, regra.ID, "sistema")
	s.auditoria.Log(ctx, model.AudCadastro, "regra.editada", fmt.Sprintf("regra #%d", regra.ID), "sistema", regra.Descricao,
		map[string]interface{}{"id": regra.ID, "descricao": regra.Descricao})
	return nil
}

func (s *RegraContabilService) ExcluirRegra(ctx context.Context, id int64) error {
	if id == 0 {
		return fmt.Errorf("id da regra é obrigatório")
	}
	if err := s.repo.ExcluirRegra(ctx, id); err != nil {
		return err
	}
	s.capturarVersao(ctx, id, "sistema")
	s.auditoria.Log(ctx, model.AudCadastro, "regra.excluida", fmt.Sprintf("regra #%d", id), "sistema", "exclusão lógica da regra",
		map[string]interface{}{"id": id})
	return nil
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
	novoID, err := s.repo.CriarCondicao(ctx, condicao)
	if err != nil {
		return 0, err
	}
	s.capturarVersao(ctx, condicao.IDRegra, "sistema")
	s.auditoria.Log(ctx, model.AudCadastro, "condicao.criada", fmt.Sprintf("condição #%d (regra #%d)", novoID, condicao.IDRegra), "sistema", condicao.Condicao,
		map[string]interface{}{"id": novoID, "id_regra": condicao.IDRegra, "condicao": condicao.Condicao, "conta_debito": condicao.ContaDebito, "conta_credito": condicao.ContaCredito})
	return novoID, nil
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
	// Descobre a regra-mãe ANTES da mutação (a condição ainda está visível).
	idRegra := condicao.IDRegra
	if s.versaoRepo != nil && idRegra == 0 {
		idRegra = s.idRegraDaCondicao(ctx, condicao.ID)
	}
	if err := s.repo.EditarCondicao(ctx, condicao); err != nil {
		return err
	}
	s.capturarVersao(ctx, idRegra, "sistema")
	s.auditoria.Log(ctx, model.AudCadastro, "condicao.editada", fmt.Sprintf("condição #%d (regra #%d)", condicao.ID, idRegra), "sistema", condicao.Condicao,
		map[string]interface{}{"id": condicao.ID, "id_regra": idRegra, "condicao": condicao.Condicao})
	return nil
}

func (s *RegraContabilService) ExcluirCondicao(ctx context.Context, id int64) error {
	if id == 0 {
		return fmt.Errorf("id da condição é obrigatório")
	}
	// Descobre a regra-mãe ANTES de excluir (depois a condição some das visíveis).
	var idRegra int64
	if s.versaoRepo != nil {
		idRegra = s.idRegraDaCondicao(ctx, id)
	}
	if err := s.repo.ExcluirCondicao(ctx, id); err != nil {
		return err
	}
	s.capturarVersao(ctx, idRegra, "sistema")
	s.auditoria.Log(ctx, model.AudCadastro, "condicao.excluida", fmt.Sprintf("condição #%d (regra #%d)", id, idRegra), "sistema", "exclusão lógica da condição",
		map[string]interface{}{"id": id, "id_regra": idRegra})
	return nil
}

// ── Histórico de versões do cadastro (RF-133) ───────────────────────────────

var errHistoricoOff = fmt.Errorf("histórico de versões indisponível neste backend")

// Historico devolve as versões de uma regra (mais recente primeiro).
func (s *RegraContabilService) Historico(ctx context.Context, idRegra int64) ([]model.RegraVersao, error) {
	if s.versaoRepo == nil {
		return nil, errHistoricoOff
	}
	if idRegra == 0 {
		return nil, fmt.Errorf("id da regra é obrigatório")
	}
	return s.versaoRepo.Listar(ctx, idRegra)
}

// ResumoHistorico devolve a última versão de CADA regra (sem o snapshot pesado), para a lista
// de regras exibir um selo "vN · alterada em ...". Map vazio quando a feature está off.
func (s *RegraContabilService) ResumoHistorico(ctx context.Context) (map[int64]model.RegraVersao, error) {
	out := map[int64]model.RegraVersao{}
	if s.versaoRepo == nil {
		return out, nil
	}
	ultimas, err := s.versaoRepo.ResumoTodos(ctx)
	if err != nil {
		return nil, err
	}
	for _, v := range ultimas {
		out[v.IDRegra] = v
	}
	return out, nil
}

// DiffVersoes compara duas versões da regra. versaoA/versaoB <= 0 assumem, respectivamente, a
// versão imediatamente anterior e a mais recente ("vigente × anterior").
func (s *RegraContabilService) DiffVersoes(ctx context.Context, idRegra int64, versaoA, versaoB int) (model.DiffRegraVersoes, error) {
	if s.versaoRepo == nil {
		return model.DiffRegraVersoes{}, errHistoricoOff
	}
	versoes, err := s.versaoRepo.Listar(ctx, idRegra)
	if err != nil {
		return model.DiffRegraVersoes{}, err
	}
	if len(versoes) == 0 {
		return model.DiffRegraVersoes{IDRegra: idRegra, SemAlteracoes: true}, nil
	}
	// versoes vem em ordem decrescente de número.
	maxV := versoes[0].Versao
	if versaoB <= 0 {
		versaoB = maxV
	}
	if versaoA <= 0 {
		// versão imediatamente anterior a B dentre as existentes.
		versaoA = versaoB
		for _, v := range versoes {
			if v.Versao < versaoB {
				versaoA = v.Versao
				break
			}
		}
	}
	va, okA, err := s.versaoRepo.Obter(ctx, idRegra, versaoA)
	if err != nil {
		return model.DiffRegraVersoes{}, err
	}
	vb, okB, err := s.versaoRepo.Obter(ctx, idRegra, versaoB)
	if err != nil {
		return model.DiffRegraVersoes{}, err
	}
	if !okA || !okB {
		return model.DiffRegraVersoes{}, fmt.Errorf("versão não encontrada")
	}
	d := model.DiffRegras(va.Regra, vb.Regra)
	d.IDRegra = idRegra
	d.VersaoA = versaoA
	d.VersaoB = versaoB
	return d, nil
}

// RestaurarVersao torna uma versão antiga o estado vigente da regra (RF-133). O snapshot vira a
// verdade: os campos da regra e das condições são sobrescritos, condições excluídas depois são
// ressuscitadas e condições criadas depois são excluídas. É append-only: a restauração gera uma
// nova versão ("Revertido para vN"), preservando todo o histórico.
func (s *RegraContabilService) RestaurarVersao(ctx context.Context, idRegra int64, versao int) error {
	if s.versaoRepo == nil {
		return errHistoricoOff
	}
	if idRegra == 0 || versao <= 0 {
		return fmt.Errorf("id da regra e versão são obrigatórios")
	}
	alvo, ok, err := s.versaoRepo.Obter(ctx, idRegra, versao)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("versão %d não encontrada para a regra %d", versao, idRegra)
	}
	snap := alvo.Regra
	snap.ID = idRegra

	// 1) Restaura os campos da regra (com status, para refazer ativo/status daquela versão).
	snap.Status = model.NormalizaStatus(snap.Status, snap.Ativo)
	snap.Ativo = model.AtivoDoStatus(snap.Status)
	if err := s.repo.EditarRegra(ctx, snap); err != nil {
		return err
	}

	// 2) Reconcilia as condições. As do snapshot são restauradas (ressuscitando as que haviam
	//    sido excluídas, pois a exclusão é lógica e a linha permanece); as criadas depois são
	//    excluídas. Usa o REPO direto para não gerar uma versão por condição.
	noSnapshot := map[int64]bool{}
	for _, c := range snap.Condicoes {
		noSnapshot[c.ID] = true
		c.IDRegra = idRegra
		c.Status = model.NormalizaStatus(c.Status, c.Ativo)
		c.Ativo = model.AtivoDoStatus(c.Status)
		if c.ID != 0 {
			if err := s.repo.EditarCondicao(ctx, c); err != nil {
				return err
			}
		} else {
			if _, err := s.repo.CriarCondicao(ctx, c); err != nil {
				return err
			}
		}
	}
	atual, _, err := s.versaoRepo.Carregar(ctx, idRegra)
	if err == nil {
		for _, c := range atual.Condicoes {
			if !noSnapshot[c.ID] {
				if err := s.repo.ExcluirCondicao(ctx, c.ID); err != nil {
					return err
				}
			}
		}
	}

	// 3) Anexa a nova versão que registra a restauração.
	s.capturarVersaoComResumo(ctx, idRegra, fmt.Sprintf("Revertido para v%d", versao), "sistema")
	s.auditoria.Log(ctx, model.AudCadastro, "regra.restaurada", fmt.Sprintf("regra #%d", idRegra), "sistema", fmt.Sprintf("revertida para a versão v%d", versao),
		map[string]interface{}{"id_regra": idRegra, "versao": versao})
	return nil
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
