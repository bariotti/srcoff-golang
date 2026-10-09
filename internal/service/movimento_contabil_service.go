package service

import (
	"context"
	"fmt"
	"log"
	"math"
	"os"
	"sort"
	"strings"
	"time"

	"srcoff/internal/evaluator"
	"srcoff/internal/model"
)

type posicaoCarteiraRepo interface {
	BuscarPorDataEVersaoMaxima(ctx context.Context, data time.Time) ([]model.PosicaoCarteira, error)
}

type regraContabilRepo interface {
	ListarRegrasAtivas(ctx context.Context) ([]model.RegraContabil, error)
}

type movimentoContabilRepo interface {
	BulkInsert(ctx context.Context, lancamentos []model.LancamentoContabil) error
	ObterProximaVersao(ctx context.Context, data time.Time) (int, error)
	ObterVersaoAtual(ctx context.Context, data time.Time) (int, error)
	BuscarPorDataEIndicador(ctx context.Context, data time.Time, indicadorReversao bool) ([]model.LancamentoContabil, error)
	ConsultarPaginado(ctx context.Context, data time.Time, pagina, tamanho int) (*model.PaginaLancamentos, error)
	ConsultarPaginadoFiltrado(ctx context.Context, dataInicio, dataFim time.Time, boleto string, versao int, versaoModo string, pagina, tamanho int) (*model.PaginaLancamentos, error)
	ConsultarPaginadoFiltradoSemCancelados(ctx context.Context, dataInicio, dataFim time.Time, boleto string, versao int, versaoModo string, pagina, tamanho int) (*model.PaginaLancamentos, error)
	ExcluirPorDataEVersao(ctx context.Context, data time.Time, versao int) error
}

// inconsistenciaRepoWriter persiste as inconsistências detectadas no processamento.
type inconsistenciaRepoWriter interface {
	SubstituirPorEscopo(ctx context.Context, data time.Time, combos []model.ProdutoDominio, itens []model.InconsistenciaProcessamento) error
}

// execucaoRepoWriter registra a execução do contábil por combinação e consulta as
// datas já executadas por produto/domínio (base da validação de D-1).
type execucaoRepoWriter interface {
	RegistrarExecucao(ctx context.Context, e model.MovimentoExecucao) error
	DatasExecutadas(ctx context.Context, produto, dominio string) ([]time.Time, error)
	// DatasComMovimento retorna as datas que efetivamente geraram lançamentos (>0) —
	// base correta da obrigatoriedade de D-1 e da escolha da data de estorno.
	DatasComMovimento(ctx context.Context, produto, dominio string) ([]time.Time, error)
}

// padraoFlagLookup consulta a configuração dos padrões de arquivo (ex: obrigatoriedade
// do movimento de D-1) por produto/domínio.
type padraoFlagLookup interface {
	Listar(ctx context.Context) ([]model.PadraoArquivo, error)
}

// MovimentoContabilService implementa a lógica de geração e consulta de movimentos contábeis.
type MovimentoContabilService struct {
	posicaoRepo        posicaoCarteiraRepo
	regraRepo          regraContabilRepo
	movimentoRepo      movimentoContabilRepo
	evaluator          evaluator.Evaluator
	inconsistenciaRepo inconsistenciaRepoWriter
	execucaoRepo       execucaoRepoWriter
	padraoRepo         padraoFlagLookup
}

// NewMovimentoContabilService cria uma nova instância do serviço com as dependências injetadas.
func NewMovimentoContabilService(
	posicaoRepo posicaoCarteiraRepo,
	regraRepo regraContabilRepo,
	movimentoRepo movimentoContabilRepo,
	eval evaluator.Evaluator,
) *MovimentoContabilService {
	return &MovimentoContabilService{
		posicaoRepo:   posicaoRepo,
		regraRepo:     regraRepo,
		movimentoRepo: movimentoRepo,
		evaluator:     eval,
	}
}

// ComInconsistenciaRepo injeta o repositório de inconsistências (opcional). Permite
// persistir as inconsistências detectadas em GerarMovimento sem alterar a assinatura
// do construtor usada nos testes.
func (s *MovimentoContabilService) ComInconsistenciaRepo(r inconsistenciaRepoWriter) *MovimentoContabilService {
	s.inconsistenciaRepo = r
	return s
}

// ComExecucaoRepo injeta o repositório de log de execução (opcional).
func (s *MovimentoContabilService) ComExecucaoRepo(r execucaoRepoWriter) *MovimentoContabilService {
	s.execucaoRepo = r
	return s
}

// ComPadraoRepo injeta o lookup dos padrões de arquivo (opcional) — usado para saber
// se a validação de obrigatoriedade do movimento de D-1 se aplica a cada combinação.
func (s *MovimentoContabilService) ComPadraoRepo(r padraoFlagLookup) *MovimentoContabilService {
	s.padraoRepo = r
	return s
}

// exigeMovimentoD1 informa se a combinação (produto, domínio) exige a validação de
// movimento de D-1 útil, conforme o padrão de arquivo cadastrado. Default: true
// (quando não há lookup de padrão ou nenhum padrão casa com a combinação).
func (s *MovimentoContabilService) exigeMovimentoD1(ctx context.Context, produto, dominio string) bool {
	if s.padraoRepo == nil {
		return true
	}
	padroes, err := s.padraoRepo.Listar(ctx)
	if err != nil {
		return true
	}
	for _, p := range padroes {
		if strings.TrimSpace(p.Produto) == produto && strings.TrimSpace(p.Dominio) == dominio {
			return p.ExigeMovimentoD1()
		}
	}
	return true
}

// dataEstornoPorCombo resolve a data de origem do estorno para uma combinação:
//   - exige D-1 (padrão): o dia útil anterior à data de processamento;
//   - não exige: a maior data anterior à de processamento com contábil executado.
//
// Retorna ok=false quando não há data de origem (nenhuma execução anterior).
func (s *MovimentoContabilService) dataEstornoPorCombo(ctx context.Context, data time.Time, produto, dominio string, exigeD1 bool) (time.Time, bool) {
	if exigeD1 {
		return DiaUtilAnterior(data), true
	}
	if s.execucaoRepo == nil {
		return time.Time{}, false
	}
	// Maior dia útil anterior COM movimento contábil (lançamentos > 0).
	datas, err := s.execucaoRepo.DatasComMovimento(ctx, produto, dominio)
	if err != nil {
		return time.Time{}, false
	}
	var melhor time.Time
	achou := false
	for _, d := range datas {
		if d.Before(data) && (!achou || d.After(melhor)) {
			melhor = d
			achou = true
		}
	}
	return melhor, achou
}

// GerarMovimento processa toda a data (todas as combinações). Mantido para
// compatibilidade; delega para GerarMovimentoEscopo sem filtro.
func (s *MovimentoContabilService) GerarMovimento(ctx context.Context, data time.Time) ([]model.ProdutoDominio, error) {
	return s.GerarMovimentoEscopo(ctx, data, "", "")
}

// GerarMovimentoEscopo processa a posição de carteira para a data, restrita ao
// Produto e/ou Domínio informados (vazio = sem filtro naquela dimensão). Avalia as
// regras ativas do escopo, gera lançamentos e estornos de D-1 (também do escopo),
// grava produto/domínio em cada lançamento e registra a execução por combinação.
// Retorna a lista de combinações (produto, domínio) BLOQUEADAS pela obrigatoriedade de
// movimento de D-1 útil (não processadas); as demais são processadas normalmente.
//
// Spec: núcleo de geração — RN-100..RN-160 (docs/especificacao.md §5).
//   RN-100 dia útil · RN-101 posição vigente · RN-110 aplicação de regras/inconsistências ·
//   RN-120 incremental · RN-130/131 estorno · RN-140 obrigatoriedade D-1 ·
//   RN-150 versionamento por combo · RN-160 persistência/execução.
func (s *MovimentoContabilService) GerarMovimentoEscopo(ctx context.Context, data time.Time, produtoFiltro, dominioFiltro string) ([]model.ProdutoDominio, error) {
	// 0. O contábil só pode ser processado em dias úteis.
	if !EhDiaUtil(data) {
		return nil, fmt.Errorf("data %s não é dia útil (%s); o contábil só pode ser processado em dias úteis", data.Format("2006-01-02"), DescricaoDiasNaoUteis)
	}

	// 1. Buscar posição com versão máxima para a data
	todasPosicoes, err := s.posicaoRepo.BuscarPorDataEVersaoMaxima(ctx, data)
	if err != nil {
		return nil, fmt.Errorf("erro ao buscar posicao_carteira: %w", err)
	}

	// Filtrar posições pelo escopo (produto/domínio).
	var posicoes []model.PosicaoCarteira
	for _, p := range todasPosicoes {
		prod := campoStrModel(p, "produto")
		dom := campoStrModel(p, "dominio")
		if produtoFiltro != "" && prod != produtoFiltro {
			continue
		}
		if dominioFiltro != "" && dom != dominioFiltro {
			continue
		}
		posicoes = append(posicoes, p)
	}

	if len(posicoes) == 0 {
		return nil, fmt.Errorf("nenhum registro de posicao_carteira encontrado para a data %s no escopo informado", data.Format("2006-01-02"))
	}

	// 2. Carregar todas as regras e condições ativas
	regras, err := s.regraRepo.ListarRegrasAtivas(ctx)
	if err != nil {
		return nil, fmt.Errorf("erro ao carregar regras contábeis: %w", err)
	}

	// Memoiza a obrigatoriedade de D-1 por combinação: exigeMovimentoD1 relê todos os
	// padrões a cada chamada e é consultado em vários pontos (base incremental e bloqueio
	// de D-1) para as mesmas combinações — o cache evita a releitura repetida.
	exigeD1Memo := map[string]bool{}
	exigeD1De := func(produto, dominio string) bool {
		k := produto + "\x00" + dominio
		if v, ok := exigeD1Memo[k]; ok {
			return v
		}
		v := s.exigeMovimentoD1(ctx, produto, dominio)
		exigeD1Memo[k] = v
		return v
	}

	// 2b. Base do cálculo INCREMENTAL (Spec: RN-120, docs/especificacao.md §5.4).
	// Para regras incrementais, o valor lançado em D0 é
	// | |valor_D0| - |valor_D-N| | por (produto, domínio, boleto, regra, contas), onde D-N é a
	// MESMA data-base do estorno (D-1 útil quando o padrão exige D-1; senão o último dia com
	// movimento antes de D0). O valor_D-N vem de reler a posição de D-N e reavaliar campo_valor.
	baseIncremental := map[string]float64{}
	temIncremental := false
	for _, reg := range regras {
		if reg.EhIncremental() {
			temIncremental = true
			break
		}
	}
	if temIncremental {
		combosD0 := map[string]model.ProdutoDominio{}
		for _, p := range posicoes {
			prod := campoStrModel(p, "produto")
			dom := campoStrModel(p, "dominio")
			combosD0[prod+"\x00"+dom] = model.ProdutoDominio{Produto: prod, Dominio: dom}
		}
		posDNCache := map[string][]model.PosicaoCarteira{}
		for _, c := range combosD0 {
			exigeD1 := exigeD1De(c.Produto, c.Dominio)
			dn, ok := s.dataEstornoPorCombo(ctx, data, c.Produto, c.Dominio, exigeD1)
			if !ok {
				continue // sem D-N → primeira ocorrência: base 0 (lança o valor cheio)
			}
			dnStr := dn.Format("2006-01-02")
			posDN, cached := posDNCache[dnStr]
			if !cached {
				posDN, err = s.posicaoRepo.BuscarPorDataEVersaoMaxima(ctx, dn)
				if err != nil {
					return nil, fmt.Errorf("erro ao buscar posição de D-N para incremental: %w", err)
				}
				posDNCache[dnStr] = posDN
			}
			for _, p := range posDN {
				if campoStrModel(p, "produto") != c.Produto || campoStrModel(p, "dominio") != c.Dominio {
					continue
				}
				s.acumularBaseIncremental(p, regras, baseIncremental)
			}
		}
	}

	// 3. Gerar lançamentos de D em memória
	var lancamentos []model.LancamentoContabil
	var inconsistencias []model.InconsistenciaProcessamento
	combosSet := map[string]model.ProdutoDominio{}
	for _, posicao := range posicoes {
		env := evaluator.PosicaoToEnv(posicao)
		produto := evaluator.CampoString(env, "produto")
		dominio := evaluator.CampoString(env, "dominio")
		combosSet[produto+"\x00"+dominio] = model.ProdutoDominio{Produto: produto, Dominio: dominio}
		for _, regra := range regras {
			// Só aplica a regra se Produto E Domínio da posição coincidirem com a regra.
			if !regraAplicaAPosicao(regra, env) {
				continue
			}
			boleto := boletoDaPosicao(env, regra.Condicoes)
			// Pré-condição da regra (opcional): avaliada uma vez por posição/regra.
			// Se referenciar um campo ausente na posição, registra inconsistência e
			// NÃO processa nenhuma condição desta regra (não gera lançamento).
			if strings.TrimSpace(regra.PreCondicao) != "" {
				if faltantes := camposFaltantes(env, regra.PreCondicao); len(faltantes) > 0 {
					inconsistencias = append(inconsistencias, novaInconsistencia(
						data, boleto, produto, dominio, regra, model.InconsistenciaPreCondicao, regra.PreCondicao, faltantes))
					continue
				}
				ok, err := s.evaluator.EvaluateCondition(regra.PreCondicao, env)
				if err != nil {
					evaluator.LogEvalError(data, boleto, regra.PreCondicao, err)
					continue
				}
				if !ok {
					continue
				}
			}
			for _, condicao := range regra.Condicoes {
				if !condicao.Ativo {
					continue
				}
				// Condição referenciando campo ausente → inconsistência, sem lançamento.
				if faltantes := camposFaltantes(env, condicao.Condicao); len(faltantes) > 0 {
					inconsistencias = append(inconsistencias, novaInconsistencia(
						data, boleto, produto, dominio, regra, model.InconsistenciaCondicao, condicao.Condicao, faltantes))
					continue
				}
				ok, err := s.evaluator.EvaluateCondition(condicao.Condicao, env)
				if err != nil {
					evaluator.LogEvalError(data, boleto, condicao.Condicao, err)
					continue
				}
				if !ok {
					continue
				}
				// Condição satisfeita: campo_valor referenciando campo ausente → inconsistência.
				if faltantes := camposFaltantes(env, condicao.CampoValor); len(faltantes) > 0 {
					inconsistencias = append(inconsistencias, novaInconsistencia(
						data, boleto, produto, dominio, regra, model.InconsistenciaCampoValor, condicao.CampoValor, faltantes))
					continue
				}
				valor, err := s.evaluator.EvaluateValue(condicao.CampoValor, env)
				if err != nil {
					evaluator.LogEvalError(data, boleto, condicao.CampoValor, err)
					continue
				}
				moeda := evaluator.CampoString(env, condicao.CampoMoeda)
				boletoLanc := evaluator.CampoString(env, campoBoletoOuPadrao(condicao.CampoBoleto))
				// Incremental: o valor lançado é | |valor_D0| - |valor_D-N| | para a mesma
				// (produto, domínio, boleto, regra, contas). Sem base (1ª ocorrência) → valor cheio.
				if regra.EhIncremental() {
					k := chaveIncremental(produto, dominio, boletoLanc, regra.ID, condicao.ContaDebito, condicao.ContaCredito)
					valor = math.Abs(math.Abs(valor) - math.Abs(baseIncremental[k]))
				}
				lancamentos = append(lancamentos, model.LancamentoContabil{
					DataLoteContabil:          data,
					CodigoIdentificadorBoleto: boletoLanc,
					ValorLancamentoContabil:   valor,
					MoedaLancamentoContabil:   moeda,
					ContaDebito:               condicao.ContaDebito,
					ContaCredito:              condicao.ContaCredito,
					Produto:                   produto,
					Dominio:                   dominio,
					IndicadorReversao:         false,
					DescricaoRegraContabil:    regra.Descricao,
					DescricaoCondicaoContabil: condicao.Condicao,
					IDRegraContabil:           regra.ID,
				})
			}
		}
	}

	// 3b. Obrigatoriedade de movimento de D-1 útil, por combinação. Baseia-se no MOVIMENTO
	// CONTÁBIL REAL (dias com lançamentos > 0), não no mero log de execução — uma rodada que
	// gerou 0 lançamentos (ex.: barrada por inconsistências) não conta como "tem contábil".
	// Regra (padrão exige D-1):
	//   - existe contábil em algum dia útil ANTERIOR à data? Não → executa.
	//   - Sim → existe contábil no dia útil anterior (D-1)? Não → barra; Sim → executa.
	bloqueados := map[string]bool{}
	var bloqueios []model.ProdutoDominio
	if s.execucaoRepo != nil {
		dataStr := data.Format("2006-01-02")
		dUtilAnterior := DiaUtilAnterior(data).Format("2006-01-02")
		for _, c := range combosSet {
			if !exigeD1De(c.Produto, c.Dominio) {
				continue
			}
			datas, err := s.execucaoRepo.DatasComMovimento(ctx, c.Produto, c.Dominio)
			if err != nil {
				return nil, fmt.Errorf("erro ao verificar obrigatoriedade de D-1: %w", err)
			}
			temAnterior := false // existe contábil em data estritamente anterior?
			temD1 := false       // existe contábil exatamente no dia útil anterior?
			for _, d := range datas {
				ds := d.Format("2006-01-02")
				if ds < dataStr {
					temAnterior = true
				}
				if ds == dUtilAnterior {
					temD1 = true
				}
			}
			// Só bloqueia quando já há contábil anterior e falta o D-1 útil.
			if temAnterior && !temD1 {
				bloqueados[c.Produto+"\x00"+c.Dominio] = true
				bloqueios = append(bloqueios, c)
			}
		}
	}
	// Remove as combinações bloqueadas do processamento (lançamentos, inconsistências e combos).
	if len(bloqueados) > 0 {
		for k := range bloqueados {
			delete(combosSet, k)
		}
		lancFiltrados := lancamentos[:0]
		for _, l := range lancamentos {
			if !bloqueados[l.Produto+"\x00"+l.Dominio] {
				lancFiltrados = append(lancFiltrados, l)
			}
		}
		lancamentos = lancFiltrados
		incFiltradas := inconsistencias[:0]
		for _, in := range inconsistencias {
			if !bloqueados[in.Produto+"\x00"+in.Dominio] {
				incFiltradas = append(incFiltradas, in)
			}
		}
		inconsistencias = incFiltradas
		for _, b := range bloqueios {
			log.Printf("[movimento] bloqueio D-1: %s/%s não processado em %s (falta movimento do dia útil anterior)", b.Produto, b.Dominio, data.Format("2006-01-02"))
		}
	}

	// Combinações (produto, domínio) efetivamente processadas (sem as bloqueadas).
	var combos []model.ProdutoDominio
	for _, c := range combosSet {
		combos = append(combos, c)
	}

	// Persistir inconsistências detectadas — substitui apenas as combinações do escopo.
	if s.inconsistenciaRepo != nil {
		if err := s.inconsistenciaRepo.SubstituirPorEscopo(ctx, data, combos, inconsistencias); err != nil {
			log.Printf("[movimento] falha ao persistir inconsistências para %s: %v", data.Format("2006-01-02"), err)
		}
	}
	if len(inconsistencias) > 0 {
		log.Printf("[movimento] %d inconsistência(s) detectada(s) para %s (lançamentos não gerados)", len(inconsistencias), data.Format("2006-01-02"))
	}

	// 4. Gerar estornos por combinação. A data de origem depende da obrigatoriedade de
	// D-1 do padrão: exige D-1 → dia útil anterior; não exige → maior data anterior com
	// movimento. Combinações sem data de origem (nenhuma execução anterior) não estornam.
	var estornos []model.LancamentoContabil
	if len(combosSet) > 0 {
		regraGeraEstorno := make(map[int64]bool, len(regras))
		for _, reg := range regras {
			regraGeraEstorno[reg.ID] = reg.EhReverte()
		}
		// Resolve a data de origem do estorno por combinação e agrupa as datas necessárias.
		dataOrigemCombo := map[string]time.Time{}
		datasNecessarias := map[string]time.Time{}
		for _, c := range combosSet {
			exigeD1 := exigeD1De(c.Produto, c.Dominio)
			dOrig, ok := s.dataEstornoPorCombo(ctx, data, c.Produto, c.Dominio, exigeD1)
			if !ok {
				continue
			}
			dataOrigemCombo[c.Produto+"\x00"+c.Dominio] = dOrig
			datasNecessarias[dOrig.Format("2006-01-02")] = dOrig
		}
		// Busca os lançamentos normais vigentes de cada data de origem (cache por data).
		lancPorData := map[string][]model.LancamentoContabil{}
		for ds, d := range datasNecessarias {
			ls, err := s.movimentoRepo.BuscarPorDataEIndicador(ctx, d, false)
			if err != nil {
				return nil, fmt.Errorf("erro ao buscar lançamentos para estorno: %w", err)
			}
			lancPorData[ds] = ls
		}
		// Gera os estornos por combinação, invertendo as contas.
		for comboKey, dOrig := range dataOrigemCombo {
			for _, l1 := range lancPorData[dOrig.Format("2006-01-02")] {
				if l1.Produto+"\x00"+l1.Dominio != comboKey {
					continue
				}
				if ge, found := regraGeraEstorno[l1.IDRegraContabil]; found && !ge {
					continue
				}
				estornos = append(estornos, model.LancamentoContabil{
					DataLoteContabil:          data,
					CodigoIdentificadorBoleto: l1.CodigoIdentificadorBoleto,
					ValorLancamentoContabil:   l1.ValorLancamentoContabil,
					MoedaLancamentoContabil:   l1.MoedaLancamentoContabil,
					ContaDebito:               l1.ContaCredito,
					ContaCredito:              l1.ContaDebito,
					Produto:                   l1.Produto,
					Dominio:                   l1.Dominio,
					IndicadorReversao:         true,
					DescricaoRegraContabil:    l1.DescricaoRegraContabil,
					DescricaoCondicaoContabil: l1.DescricaoCondicaoContabil,
					IDRegraContabil:           l1.IDRegraContabil,
				})
			}
		}
	}

	// 5. Calcular a próxima versão POR combinação (produto, domínio) e aplicá-la a
	// lançamentos e estornos. Cada combinação versiona de forma independente, então
	// a primeira execução de um novo produto/domínio na data começa na versão 1,
	// mesmo que outras combinações já tenham versões maiores.
	maxVersaoCombo := map[string]int{}
	for _, ind := range []bool{false, true} {
		vigentes, err := s.movimentoRepo.BuscarPorDataEIndicador(ctx, data, ind)
		if err != nil {
			return nil, fmt.Errorf("erro ao obter versão vigente: %w", err)
		}
		for _, l := range vigentes {
			k := l.Produto + "\x00" + l.Dominio
			if l.CodigoVersaoConteudo > maxVersaoCombo[k] {
				maxVersaoCombo[k] = l.CodigoVersaoConteudo
			}
		}
	}
	versaoPorCombo := map[string]int{}
	for _, c := range combosSet {
		k := c.Produto + "\x00" + c.Dominio
		versaoPorCombo[k] = maxVersaoCombo[k] + 1
	}
	for i := range lancamentos {
		lancamentos[i].CodigoVersaoConteudo = versaoPorCombo[lancamentos[i].Produto+"\x00"+lancamentos[i].Dominio]
	}
	for i := range estornos {
		estornos[i].CodigoVersaoConteudo = versaoPorCombo[estornos[i].Produto+"\x00"+estornos[i].Dominio]
	}

	// 6. Persistir movimento + estornos em um único BulkInsert
	todos := append(lancamentos, estornos...)
	if err := s.movimentoRepo.BulkInsert(ctx, todos); err != nil {
		return nil, fmt.Errorf("erro ao persistir lançamentos: %w", err)
	}

	// Contagem por combinação (produto, domínio), usada no registro de execução e no log.
	qtdLanc := map[string]int{}
	qtdEst := map[string]int{}
	for _, l := range lancamentos {
		qtdLanc[l.Produto+"\x00"+l.Dominio]++
	}
	for _, e := range estornos {
		qtdEst[e.Produto+"\x00"+e.Dominio]++
	}

	// 7. Registrar a execução por combinação (data, produto, domínio) com a contagem
	// VISÍVEL — a mesma exibida na consulta de movimento (versão vigente, excluindo os
	// pares lançamento+estorno de saldo zero). Assim o "Processados × Pendentes" bate
	// com a quantidade da consulta para o mesmo Produto/Domínio/Data/Versão.
	if s.execucaoRepo != nil {
		qtdLancVis := map[string]int{}
		qtdEstVis := map[string]int{}
		if visiveis, err := s.movimentoRepo.ConsultarPaginadoFiltradoSemCancelados(ctx, data, data, "", 0, "vigente", 1, 999999); err == nil {
			for _, l := range visiveis.Lancamentos {
				k := l.Produto + "\x00" + l.Dominio
				if l.IndicadorReversao {
					qtdEstVis[k]++
				} else {
					qtdLancVis[k]++
				}
			}
		} else {
			log.Printf("[movimento] falha ao contar lançamentos visíveis para execução: %v", err)
		}
		for _, c := range combosSet {
			k := c.Produto + "\x00" + c.Dominio
			if err := s.execucaoRepo.RegistrarExecucao(ctx, model.MovimentoExecucao{
				DataLote: data, Produto: c.Produto, Dominio: c.Dominio,
				QtdLancamentos: qtdLancVis[k], QtdEstornos: qtdEstVis[k],
				QtdMovimento: qtdLanc[k], // contagem bruta de movimento (não-estorno) gerado
			}); err != nil {
				log.Printf("[movimento] falha ao registrar execução %s/%s: %v", c.Produto, c.Dominio, err)
			}
		}
	}

	// Detalhamento por combinação (produto/domínio) efetivamente persistida, ordenado para log estável.
	detalhes := make([]string, 0, len(combosSet))
	for _, c := range combosSet {
		k := c.Produto + "\x00" + c.Dominio
		detalhes = append(detalhes, fmt.Sprintf("%s/%s v%d: %d lançamentos, %d estornos", c.Produto, c.Dominio, versaoPorCombo[k], qtdLanc[k], qtdEst[k]))
	}
	sort.Strings(detalhes)

	log.Printf("[movimento+estorno] persistidos %d lançamentos e %d estornos para %s (escopo produto=%q dominio=%q) [%s]",
		len(lancamentos), len(estornos), data.Format("2006-01-02"), produtoFiltro, dominioFiltro, strings.Join(detalhes, "; "))
	return bloqueios, nil
}

// ConsultarLancamentos retorna os lançamentos paginados para a data informada.
func (s *MovimentoContabilService) ConsultarLancamentos(ctx context.Context, data time.Time, pagina, tamanho int) (*model.PaginaLancamentos, error) {
	return s.movimentoRepo.ConsultarPaginado(ctx, data, pagina, tamanho)
}

// ConsultarLancamentosFiltrado retorna lançamentos paginados por período, boleto e versão.
// Elimina lançamentos cujo saldo líquido (normal - reversão) é zero — usado pela página de consulta do frontend.
func (s *MovimentoContabilService) ConsultarLancamentosFiltrado(ctx context.Context, dataInicio, dataFim time.Time, boleto string, versao int, versaoModo string, pagina, tamanho int) (*model.PaginaLancamentos, error) {
	return s.movimentoRepo.ConsultarPaginadoFiltradoSemCancelados(ctx, dataInicio, dataFim, boleto, versao, versaoModo, pagina, tamanho)
}

// ConsultarLancamentosFiltradoEscopo funciona como ConsultarLancamentosFiltrado, mas
// permite restringir por produto e/ou domínio (ambos opcionais). Quando os dois estão
// vazios, delega diretamente ao repositório (paginação eficiente). Quando há filtro de
// escopo, busca todos os lançamentos que atendem aos demais critérios, filtra por
// produto/domínio e pagina o resultado — mantendo a contagem total correta.
func (s *MovimentoContabilService) ConsultarLancamentosFiltradoEscopo(ctx context.Context, dataInicio, dataFim time.Time, boleto, produto, dominio string, versao int, versaoModo string, pagina, tamanho int) (*model.PaginaLancamentos, error) {
	if produto == "" && dominio == "" {
		return s.movimentoRepo.ConsultarPaginadoFiltradoSemCancelados(ctx, dataInicio, dataFim, boleto, versao, versaoModo, pagina, tamanho)
	}

	completo, err := s.movimentoRepo.ConsultarPaginadoFiltradoSemCancelados(ctx, dataInicio, dataFim, boleto, versao, versaoModo, 1, 999999)
	if err != nil {
		return nil, err
	}

	filtrados := make([]model.LancamentoContabil, 0, len(completo.Lancamentos))
	for _, l := range completo.Lancamentos {
		if produto != "" && l.Produto != produto {
			continue
		}
		if dominio != "" && l.Dominio != dominio {
			continue
		}
		filtrados = append(filtrados, l)
	}

	total := len(filtrados)
	if tamanho <= 0 {
		tamanho = 100
	}
	if pagina <= 0 {
		pagina = 1
	}
	offset := (pagina - 1) * tamanho
	if offset >= total {
		return &model.PaginaLancamentos{Total: total, Pagina: pagina, Tamanho: tamanho, Lancamentos: []model.LancamentoContabil{}}, nil
	}
	end := offset + tamanho
	if end > total {
		end = total
	}
	return &model.PaginaLancamentos{Total: total, Pagina: pagina, Tamanho: tamanho, Lancamentos: filtrados[offset:end]}, nil
}

// ExcluirMovimento exclui lançamentos de uma data e opcionalmente de uma versão específica.
func (s *MovimentoContabilService) ExcluirMovimento(ctx context.Context, data time.Time, versao int) error {
	return s.movimentoRepo.ExcluirPorDataEVersao(ctx, data, versao)
}

// GerarEstorno é o endpoint público — pode ser chamado manualmente pelo operador.
func (s *MovimentoContabilService) GerarEstorno(ctx context.Context, data time.Time) error {
	return s.gerarEstornoInterno(ctx, data)
}

// gerarEstornoInterno busca lançamentos do dia útil anterior (versão vigente) e gera estornos para D.
func (s *MovimentoContabilService) gerarEstornoInterno(ctx context.Context, data time.Time) error {
	dMenos1 := DiaUtilAnterior(data)

	// 1. Buscar lançamentos de D-1 (indicador_reversao=false)
	lancamentosD1, err := s.movimentoRepo.BuscarPorDataEIndicador(ctx, dMenos1, false)
	if err != nil {
		return fmt.Errorf("erro ao buscar lançamentos de D-1: %w", err)
	}

	log.Printf("[estorno] data=%s dMenos1=%s lancamentos_d1=%d", data.Format("2006-01-02"), dMenos1.Format("2006-01-02"), len(lancamentosD1))

	// 2. Se D-1 vazio, retornar erro de ausência
	if len(lancamentosD1) == 0 {
		return fmt.Errorf("nenhum lote contábil encontrado para D-1 (%s)", dMenos1.Format("2006-01-02"))
	}

	// 3. Buscar lançamentos de D (indicador_reversao=false) — mantido para referência futura
	_, err = s.movimentoRepo.BuscarPorDataEIndicador(ctx, data, false)
	if err != nil {
		return fmt.Errorf("erro ao buscar lançamentos de D: %w", err)
	}

	// 4. Estornar todos os lançamentos de D-1:
	//    - Regra 1: sempre estornar D-1 independente do valor de D0
	//    - Regra 2: estornar D-1 quando não há correspondente em D0
	//    Como a regra 1 engloba a regra 2, estornamos todos os lançamentos de D-1.
	var estornos []model.LancamentoContabil
	for _, l1 := range lancamentosD1 {
		estornos = append(estornos, model.LancamentoContabil{
			DataLoteContabil:          data,
			CodigoIdentificadorBoleto: l1.CodigoIdentificadorBoleto,
			ValorLancamentoContabil:   l1.ValorLancamentoContabil,
			MoedaLancamentoContabil:   l1.MoedaLancamentoContabil,
			ContaDebito:               l1.ContaCredito, // contas invertidas
			ContaCredito:              l1.ContaDebito,  // contas invertidas
			IndicadorReversao:         true,
			DescricaoRegraContabil:    l1.DescricaoRegraContabil,
			DescricaoCondicaoContabil: l1.DescricaoCondicaoContabil,
			IDRegraContabil:           l1.IDRegraContabil,
		})
	}

	if len(estornos) == 0 {
		log.Printf("[estorno] nenhum estorno gerado para data=%s", data.Format("2006-01-02"))
		return nil
	}

	log.Printf("[estorno] gerando %d estornos para data=%s", len(estornos), data.Format("2006-01-02"))

	// 6. Obter próxima versão para a data D — garante que reprocessamentos não sobrescrevem versões anteriores
	versao, err := s.movimentoRepo.ObterProximaVersao(ctx, data)
	if err != nil {
		return fmt.Errorf("erro ao obter próxima versão para estorno: %w", err)
	}
	for i := range estornos {
		estornos[i].CodigoVersaoConteudo = versao
	}

	// 7. Bulk insert dos estornos
	if err := s.movimentoRepo.BulkInsert(ctx, estornos); err != nil {
		return fmt.Errorf("erro ao persistir estornos: %w", err)
	}

	return nil
}

// camposFaltantes retorna os campos referenciados na expressão que não existem no
// env da posição. Erros de sintaxe são ignorados aqui (tratados na avaliação).
func camposFaltantes(env map[string]interface{}, expressao string) []string {
	if strings.TrimSpace(expressao) == "" {
		return nil
	}
	campos, err := evaluator.CamposReferenciados(expressao)
	if err != nil {
		return nil
	}
	var faltantes []string
	for _, c := range campos {
		if _, ok := env[c]; !ok {
			faltantes = append(faltantes, c)
		}
	}
	return faltantes
}

// novaInconsistencia monta um registro de inconsistência com detalhe legível.
func novaInconsistencia(data time.Time, boleto, produto, dominio string, regra model.RegraContabil, tipo, expressao string, faltantes []string) model.InconsistenciaProcessamento {
	campos := strings.Join(faltantes, ", ")
	rotulos := map[string]string{
		model.InconsistenciaPreCondicao: "pré-condição",
		model.InconsistenciaCondicao:    "condição",
		model.InconsistenciaCampoValor:  "campo valor",
	}
	detalhe := fmt.Sprintf("A %s da regra %q referencia campo(s) inexistente(s) na posição: %s. Lançamento não gerado.",
		rotulos[tipo], regra.Descricao, campos)
	return model.InconsistenciaProcessamento{
		DataLoteContabil:          data,
		CodigoIdentificadorBoleto: boleto,
		Produto:                   produto,
		Dominio:                   dominio,
		IDRegraContabil:           regra.ID,
		DescricaoRegraContabil:    regra.Descricao,
		Tipo:                      tipo,
		Expressao:                 expressao,
		CamposFaltantes:           campos,
		Detalhe:                   detalhe,
		CriadoEm:                  time.Now(),
	}
}

// campoBoletoPadrao é o nome do campo da posição usado como identificador do
// boleto quando a condição não parametriza um campo_boleto próprio. Configurável
// via CAMPO_BOLETO_PADRAO para manter compatibilidade com bases existentes.
func campoBoletoPadrao() string {
	if v := os.Getenv("CAMPO_BOLETO_PADRAO"); v != "" {
		return v
	}
	return "codigo_identificador_boleto"
}

// campoBoletoOuPadrao retorna o campo_boleto da condição ou o padrão do sistema.
func campoBoletoOuPadrao(campoBoleto string) string {
	if strings.TrimSpace(campoBoleto) != "" {
		return campoBoleto
	}
	return campoBoletoPadrao()
}

// boletoDaPosicao tenta descobrir um identificador de boleto para logs, usando o
// campo_boleto da primeira condição que o define, ou o padrão do sistema.
func boletoDaPosicao(env map[string]interface{}, condicoes []model.CondicaoRegra) string {
	for _, c := range condicoes {
		if b := evaluator.CampoString(env, campoBoletoOuPadrao(c.CampoBoleto)); b != "" {
			return b
		}
	}
	return evaluator.CampoString(env, campoBoletoPadrao())
}

// regraAplicaAPosicao decide se uma regra deve ser avaliada para a posição,
// comparando o produto da posição com a lista de produtos da regra (separada por
// vírgula, ex: "NDF,SWAP"). O produto da posição é informado no momento do upload
// e persistido no campo `produto` (ou no campo indicado por regra.CampoProduto).
//
// Regras:
//   - Regra SEM produto (codigo_produto_corporativo vazio) aplica-se a todas as posições.
//   - Regra COM produto aplica-se apenas às posições cujo produto coincide — assim,
//     ao processar uma posição de NDF, somente as regras de NDF são aplicadas.
func regraAplicaAPosicao(regra model.RegraContabil, env map[string]interface{}) bool {
	campoProd := strings.TrimSpace(regra.CampoProduto)
	if campoProd == "" {
		campoProd = "produto"
	}
	produtoPosicao := strings.TrimSpace(evaluator.CampoString(env, campoProd))
	dominioPosicao := strings.TrimSpace(evaluator.CampoString(env, "dominio"))
	return valorCasaLista(regra.CodigoProdutoCorporativo, produtoPosicao) &&
		valorCasaLista(regra.Dominio, dominioPosicao)
}

// valorCasaLista verifica se o valor está na lista separada por vírgula. Lista
// vazia casa com qualquer valor (regra sem restrição naquela dimensão). Lista
// preenchida exige valor não-vazio presente na lista.
func valorCasaLista(lista, valor string) bool {
	if strings.TrimSpace(lista) == "" {
		return true
	}
	if valor == "" {
		return false
	}
	for _, item := range strings.Split(lista, ",") {
		if strings.TrimSpace(item) == valor {
			return true
		}
	}
	return false
}

// chaveIncremental identifica a série incremental por (produto, domínio, boleto, regra, contas).
func chaveIncremental(produto, dominio, boleto string, idRegra int64, contaDeb, contaCred string) string {
	return produto + "\x00" + dominio + "\x00" + boleto + "\x00" + fmt.Sprintf("%d", idRegra) + "\x00" + contaDeb + "\x00" + contaCred
}

// acumularBaseIncremental avalia as regras incrementais sobre uma posição (de D-N) e acumula,
// por chave, o valor de campo_valor — a base (valor_D-N) usada no cálculo incremental de D0.
// Mesma lógica de avaliação da geração, porém só para regras incrementais e sem registrar
// inconsistências (campo ausente em D-N simplesmente não contribui para a base).
func (s *MovimentoContabilService) acumularBaseIncremental(posicao model.PosicaoCarteira, regras []model.RegraContabil, base map[string]float64) {
	env := evaluator.PosicaoToEnv(posicao)
	produto := evaluator.CampoString(env, "produto")
	dominio := evaluator.CampoString(env, "dominio")
	for _, regra := range regras {
		if !regra.EhIncremental() || !regraAplicaAPosicao(regra, env) {
			continue
		}
		if pc := strings.TrimSpace(regra.PreCondicao); pc != "" {
			if len(camposFaltantes(env, pc)) > 0 {
				continue
			}
			if ok, err := s.evaluator.EvaluateCondition(pc, env); err != nil || !ok {
				continue
			}
		}
		for _, c := range regra.Condicoes {
			if !c.Ativo {
				continue
			}
			if len(camposFaltantes(env, c.Condicao)) > 0 {
				continue
			}
			if ok, err := s.evaluator.EvaluateCondition(c.Condicao, env); err != nil || !ok {
				continue
			}
			if len(camposFaltantes(env, c.CampoValor)) > 0 {
				continue
			}
			valor, err := s.evaluator.EvaluateValue(c.CampoValor, env)
			if err != nil {
				continue
			}
			boleto := evaluator.CampoString(env, campoBoletoOuPadrao(c.CampoBoleto))
			base[chaveIncremental(produto, dominio, boleto, regra.ID, c.ContaDebito, c.ContaCredito)] += valor
		}
	}
}

// BulkInsertAjuste persiste lançamentos de ajuste gerados pela conciliação IA.
// Usa a próxima versão disponível para a data.
func (s *MovimentoContabilService) BulkInsertAjuste(ctx context.Context, lancamentos []model.LancamentoContabil) error {
	if len(lancamentos) == 0 {
		return nil
	}
	// Determinar versão pela data do primeiro lançamento
	versao, err := s.movimentoRepo.ObterProximaVersao(ctx, lancamentos[0].DataLoteContabil)
	if err != nil {
		return fmt.Errorf("erro ao obter versão: %w", err)
	}
	for i := range lancamentos {
		lancamentos[i].CodigoVersaoConteudo = versao
	}
	return s.movimentoRepo.BulkInsert(ctx, lancamentos)
}
