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
	ConsultarPaginadoFiltrado(ctx context.Context, f model.FiltroMovimento) (*model.PaginaLancamentos, error)
	ConsultarPaginadoFiltradoSemCancelados(ctx context.Context, f model.FiltroMovimento) (*model.PaginaLancamentos, error)
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

// moedaBaseLookup resolve a moeda base de cada veículo (De/Para) — RF-036/RN-182.
type moedaBaseLookup interface {
	MapaVeiculoMoedaBase(ctx context.Context) (map[string]string, error)
}

// cotacaoLookup fornece as cotações indexadas por (data, moeda1, moeda2) — RF-037/RN-183.
type cotacaoLookup interface {
	MapaCotacoes(ctx context.Context) (map[string]model.Cotacao, error)
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
	saldoRepo          saldoContaWriter
	moedaBaseRepo      moedaBaseLookup
	cotacaoRepo        cotacaoLookup
}

// saldoContaWriter mantém e consulta a projeção materializada de saldo (RN-170).
// Opcional: quando ausente, a baixa do incremental_reverte usa a varredura do histórico
// (resultado idêntico) e nenhum saldo é mantido.
type saldoContaWriter interface {
	AplicarDeltas(ctx context.Context, deltas []model.SaldoConta) error
	SubstituirCombos(ctx context.Context, combos []model.ProdutoDominio, saldos []model.SaldoConta) error
	Listar(ctx context.Context, filtro model.SaldoFiltro) ([]model.SaldoConta, error)
	ListarPaginado(ctx context.Context, filtro model.SaldoFiltro, pagina, tamanho int) (*model.SaldoPagina, error)
	EstaVazio(ctx context.Context) (bool, error)
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

// ComMoedaBaseRepo injeta o lookup da moeda base do veículo (De/Para) — RF-036/RN-182.
func (s *MovimentoContabilService) ComMoedaBaseRepo(r moedaBaseLookup) *MovimentoContabilService {
	s.moedaBaseRepo = r
	return s
}

// ComCotacaoRepo injeta o lookup de cotações (conversão de moeda) — RF-037/RN-183.
func (s *MovimentoContabilService) ComCotacaoRepo(r cotacaoLookup) *MovimentoContabilService {
	s.cotacaoRepo = r
	return s
}

// ComSaldoRepo injeta a projeção de saldo (opcional, RN-170). Quando presente, a baixa
// do incremental_reverte lê o saldo materializado e o serviço o mantém a cada geração.
func (s *MovimentoContabilService) ComSaldoRepo(r saldoContaWriter) *MovimentoContabilService {
	s.saldoRepo = r
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
//
//	RN-100 dia útil · RN-101 posição vigente · RN-110 aplicação de regras/inconsistências ·
//	RN-120 incremental · RN-130/131 estorno · RN-140 obrigatoriedade D-1 ·
//	RN-150 versionamento por combo · RN-160 persistência/execução.
func (s *MovimentoContabilService) GerarMovimentoEscopo(ctx context.Context, data time.Time, produtoFiltro, dominioFiltro string) ([]model.ProdutoDominio, error) {
	// Log de início/fim/duração do processamento (Produto, Domínio e Data).
	escProd, escDom := produtoFiltro, dominioFiltro
	if escProd == "" {
		escProd = "todos"
	}
	if escDom == "" {
		escDom = "todos"
	}
	inicio := time.Now()
	log.Printf("[contabil] INICIO data=%s produto=%s dominio=%s", data.Format("2006-01-02"), escProd, escDom)
	defer func() {
		log.Printf("[contabil] FIM    data=%s produto=%s dominio=%s duracao=%s", data.Format("2006-01-02"), escProd, escDom, time.Since(inicio).Round(time.Millisecond))
	}()

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

	// Coluna do veículo da operação por combinação (produto, domínio): resolvida do padrão de
	// arquivo e normalizada (snake_case) para casar com as chaves da posição. Memoizada.
	// Vazia = padrão sem veículo (legado) → veículo do lançamento fica em branco. Spec: RF-034/RN-180.
	veiculoColMemo := map[string]string{}
	veiculoColunaDe := func(produto, dominio string) string {
		k := produto + "\x00" + dominio
		if v, ok := veiculoColMemo[k]; ok {
			return v
		}
		col := ""
		if s.padraoRepo != nil {
			if padroes, err := s.padraoRepo.Listar(ctx); err == nil {
				for _, p := range padroes {
					if strings.TrimSpace(p.Produto) == produto && strings.TrimSpace(p.Dominio) == dominio {
						col = evaluator.NormalizarNome(p.ColunaVeiculo)
						break
					}
				}
			}
		}
		veiculoColMemo[k] = col
		return col
	}

	// Layout de data do padrão por combinação (produto, domínio), para interpretar colunas de
	// data da posição (ex.: a data base de conversão) no mesmo formato cadastrado no padrão.
	formatoDataMemo := map[string]string{}
	formatoDataDe := func(produto, dominio string) string {
		k := produto + "\x00" + dominio
		if v, ok := formatoDataMemo[k]; ok {
			return v
		}
		lay := ""
		if s.padraoRepo != nil {
			if padroes, err := s.padraoRepo.Listar(ctx); err == nil {
				for _, p := range padroes {
					if strings.TrimSpace(p.Produto) == produto && strings.TrimSpace(p.Dominio) == dominio {
						lay = evaluator.LayoutData(p.FormatoData)
						break
					}
				}
			}
		}
		formatoDataMemo[k] = lay
		return lay
	}

	// Moeda base do veículo (De/Para — RF-036) e cotações (RF-037), carregadas uma vez.
	// Ausência das dependências = recursos não injetados; condições sem configuração de
	// moeda base simplesmente não convertem (campos ficam em branco).
	moedaBasePorVeiculo := map[string]string{}
	if s.moedaBaseRepo != nil {
		if m, err := s.moedaBaseRepo.MapaVeiculoMoedaBase(ctx); err == nil {
			moedaBasePorVeiculo = m
		}
	}
	cotacoes := map[string]model.Cotacao{}
	if s.cotacaoRepo != nil {
		if m, err := s.cotacaoRepo.MapaCotacoes(ctx); err == nil {
			cotacoes = m
		}
	}

	// 2b. Base do cálculo INCREMENTAL (Spec: RN-120, docs/especificacao.md §5.4).
	// Para regras incrementais, o valor lançado em D0 é
	// | |valor_D0| - |valor_D-N| | por (produto, domínio, boleto, regra, contas), onde D-N é a
	// MESMA data-base do estorno (D-1 útil quando o padrão exige D-1; senão o último dia com
	// movimento antes de D0). O valor_D-N vem de reler a posição de D-N e reavaliar campo_valor.
	baseIncremental := map[string]float64{}
	temIncremental := false
	for _, reg := range regras {
		if reg.UsaCalculoIncremental() {
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
		veiculo := evaluator.CampoString(env, veiculoColunaDe(produto, dominio))
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
				// campo_moeda referenciando campo ausente na posição → inconsistência, sem lançamento
				// (RN-110.5, item 1 — ex.: regra com moeda_mtm que não existe na posição).
				if faltantes := camposFaltantes(env, condicao.CampoMoeda); len(faltantes) > 0 {
					inconsistencias = append(inconsistencias, novaInconsistencia(
						data, boleto, produto, dominio, regra, model.InconsistenciaCampoMoeda, condicao.CampoMoeda, faltantes))
					continue
				}
				moeda := evaluator.CampoString(env, condicao.CampoMoeda)
				// campo_moeda existente mas em branco → inconsistência, sem lançamento (RN-110.5, item 2).
				if strings.TrimSpace(moeda) == "" {
					inc := novaInconsistencia(data, boleto, produto, dominio, regra, model.InconsistenciaCampoMoeda, condicao.CampoMoeda, nil)
					inc.Detalhe = fmt.Sprintf("A moeda da regra %q resultou em branco (campo %q vazio/ausente na posição). Lançamento não gerado.", regra.Descricao, condicao.CampoMoeda)
					inconsistencias = append(inconsistencias, inc)
					continue
				}
				boletoLanc := evaluator.CampoString(env, campoBoletoOuPadrao(condicao.CampoBoleto))
				// Incremental (Não Reverte e Reverte): o valor lançado é
				// | |valor_D0| - |valor_D-N| | para a mesma (produto, domínio, boleto, regra, contas).
				// Sem base (1ª ocorrência) → valor cheio.
				if regra.UsaCalculoIncremental() {
					k := chaveIncremental(produto, dominio, boletoLanc, regra.ID, condicao.ContaDebito, condicao.ContaCredito)
					valor = math.Abs(math.Abs(valor) - math.Abs(baseIncremental[k]))
				}
				// Valor na moeda base do veículo (RF-036/RN-182): da posição ou convertido.
				valorBase, moedaBase, incMB := s.resolverMoedaBaseVeiculo(
					data, boletoLanc, produto, dominio, regra, condicao, env, veiculo, valor, moeda, moedaBasePorVeiculo, cotacoes, formatoDataDe(produto, dominio))
				if incMB != nil {
					inconsistencias = append(inconsistencias, *incMB)
					continue
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
					Veiculo:                   veiculo,
					ValorMoedaBaseVeiculo:     valorBase,
					MoedaBaseVeiculo:          moedaBase,
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
					Veiculo:                   l1.Veiculo,
					ValorMoedaBaseVeiculo:     l1.ValorMoedaBaseVeiculo,
					MoedaBaseVeiculo:          l1.MoedaBaseVeiculo,
				})
			}
		}
	}

	// 4b. BAIXA por desaparecimento — tipo incremental_reverte (Spec: RN-121, §5.5).
	// Para boletos que tinham saldo em regras incremental_reverte e que NÃO existem mais na
	// posição de D0, gera estornos que ZERAM o saldo líquido postado (Σ lançamentos − Σ estornos),
	// lido do próprio movimento vigente até D-1 (ADR-007: usa o postado, não relê a posição).
	temIncRev := false
	for _, reg := range regras {
		if reg.EhIncrementalReverte() {
			temIncRev = true
			break
		}
	}
	if temIncRev {
		// Regras incremental_reverte e os campos de boleto que elas usam.
		regraIncRev := map[int64]bool{}
		cbSet := map[string]bool{}
		for _, reg := range regras {
			if !reg.EhIncrementalReverte() {
				continue
			}
			regraIncRev[reg.ID] = true
			for _, c := range reg.Condicoes {
				cbSet[campoBoletoOuPadrao(c.CampoBoleto)] = true
			}
		}
		// Boletos presentes na posição de D0, por combinação (RN-121.1 — existência na POSIÇÃO).
		presenteD0 := map[string]bool{} // chave: produto\x00dominio\x00boleto
		for _, p := range posicoes {
			env := evaluator.PosicaoToEnv(p)
			prod := evaluator.CampoString(env, "produto")
			dom := evaluator.CampoString(env, "dominio")
			for cb := range cbSet {
				if b := evaluator.CampoString(env, cb); b != "" {
					presenteD0[prod+"\x00"+dom+"\x00"+b] = true
				}
			}
		}
		// Saldo líquido postado até D-1 (movimento vigente, incluindo estornos), por chave incremental.
		type metaSaldo struct {
			produto, dominio, boleto, moeda string
			idRegra                         int64
			contaDeb, contaCred             string
			descRegra, descCond             string
			veiculo                         string
		}
		saldo := map[string]float64{}
		meta := map[string]metaSaldo{}
		if s.saldoRepo != nil {
			// Caminho MATERIALIZADO (RN-170/ADR-011): lê o saldo e subtrai a contribuição do
			// vigente atual de D0 (prevNet), obtendo o net <= D-1 — resultado idêntico à varredura.
			prevNet := map[string]float64{}
			for _, ind := range []bool{false, true} {
				ls, err := s.movimentoRepo.BuscarPorDataEIndicador(ctx, data, ind)
				if err != nil {
					return nil, fmt.Errorf("erro ao ler vigente de D0 para baixa incremental_reverte: %w", err)
				}
				for _, l := range ls {
					if !regraIncRev[l.IDRegraContabil] {
						continue
					}
					if l.IndicadorReversao {
						prevNet[chaveIncremental(l.Produto, l.Dominio, l.CodigoIdentificadorBoleto, l.IDRegraContabil, l.ContaCredito, l.ContaDebito)] -= l.ValorLancamentoContabil
					} else {
						prevNet[chaveIncremental(l.Produto, l.Dominio, l.CodigoIdentificadorBoleto, l.IDRegraContabil, l.ContaDebito, l.ContaCredito)] += l.ValorLancamentoContabil
					}
				}
			}
			var regrasIDs []int64
			for id := range regraIncRev {
				regrasIDs = append(regrasIDs, id)
			}
			saldos, err := s.saldoRepo.Listar(ctx, model.SaldoFiltro{RegrasIn: regrasIDs, IncluiZero: true})
			if err != nil {
				return nil, fmt.Errorf("erro ao ler saldo para baixa incremental_reverte: %w", err)
			}
			for _, sc := range saldos {
				k := chaveIncremental(sc.Produto, sc.Dominio, sc.CodigoIdentificadorBoleto, sc.IDRegraContabil, sc.ContaDebito, sc.ContaCredito)
				saldo[k] = sc.Saldo - prevNet[k]
				meta[k] = metaSaldo{sc.Produto, sc.Dominio, sc.CodigoIdentificadorBoleto, sc.MoedaLancamento, sc.IDRegraContabil, sc.ContaDebito, sc.ContaCredito, sc.DescricaoRegra, sc.DescricaoCondicao, ""}
			}
		} else {
			// Fallback (sem saldo materializado): varre o movimento vigente até D-1 e soma.
			hist, err := s.movimentoRepo.ConsultarPaginadoFiltrado(ctx, model.FiltroMovimento{DataInicio: time.Date(1900, 1, 1, 0, 0, 0, 0, time.UTC), DataFim: DiaUtilAnterior(data), VersaoModo: "vigente", Pagina: 1, Tamanho: 999999})
			if err != nil {
				return nil, fmt.Errorf("erro ao ler movimento para baixa incremental_reverte: %w", err)
			}
			for _, l := range hist.Lancamentos {
				if !regraIncRev[l.IDRegraContabil] {
					continue
				}
				if l.IndicadorReversao {
					k := chaveIncremental(l.Produto, l.Dominio, l.CodigoIdentificadorBoleto, l.IDRegraContabil, l.ContaCredito, l.ContaDebito)
					saldo[k] -= l.ValorLancamentoContabil
				} else {
					k := chaveIncremental(l.Produto, l.Dominio, l.CodigoIdentificadorBoleto, l.IDRegraContabil, l.ContaDebito, l.ContaCredito)
					saldo[k] += l.ValorLancamentoContabil
					if _, ok := meta[k]; !ok {
						meta[k] = metaSaldo{l.Produto, l.Dominio, l.CodigoIdentificadorBoleto, l.MoedaLancamentoContabil, l.IDRegraContabil, l.ContaDebito, l.ContaCredito, l.DescricaoRegraContabil, l.DescricaoCondicaoContabil, l.Veiculo}
					}
				}
			}
		}
		const epsSaldo = 1e-9
		for k, v := range saldo {
			if math.Abs(v) <= epsSaldo {
				continue // saldo zerado (ex.: já baixado) → nada a fazer (RN-121.4)
			}
			m, ok := meta[k]
			if !ok {
				continue
			}
			if produtoFiltro != "" && m.produto != produtoFiltro {
				continue
			}
			if dominioFiltro != "" && m.dominio != dominioFiltro {
				continue
			}
			if bloqueados[m.produto+"\x00"+m.dominio] {
				continue
			}
			if presenteD0[m.produto+"\x00"+m.dominio+"\x00"+m.boleto] {
				continue // boleto ainda existe na posição de D0 → não baixa
			}
			// Estorno de baixa: zera o saldo (contas invertidas em relação à orientação do saldo).
			deb, cred := m.contaCred, m.contaDeb
			if v < 0 {
				deb, cred = m.contaDeb, m.contaCred
			}
			estornos = append(estornos, model.LancamentoContabil{
				DataLoteContabil:          data,
				CodigoIdentificadorBoleto: m.boleto,
				ValorLancamentoContabil:   math.Abs(v),
				MoedaLancamentoContabil:   m.moeda,
				ContaDebito:               deb,
				ContaCredito:              cred,
				Produto:                   m.produto,
				Dominio:                   m.dominio,
				IndicadorReversao:         true,
				DescricaoRegraContabil:    m.descRegra,
				DescricaoCondicaoContabil: m.descCond,
				IDRegraContabil:           m.idRegra,
				Veiculo:                   m.veiculo,
			})
			// Garante o combo no versionamento/execução mesmo sem lançamentos de D0 (RN-121.5).
			ck := m.produto + "\x00" + m.dominio
			if _, ok := combosSet[ck]; !ok {
				combosSet[ck] = model.ProdutoDominio{Produto: m.produto, Dominio: m.dominio}
			}
		}
	}

	// 5. Calcular a próxima versão POR combinação (produto, domínio) e aplicá-la a
	// lançamentos e estornos. Cada combinação versiona de forma independente, então
	// a primeira execução de um novo produto/domínio na data começa na versão 1,
	// mesmo que outras combinações já tenham versões maiores.
	maxVersaoCombo := map[string]int{}
	// Para a manutenção do saldo (RN-170): net da versão vigente ANTERIOR por chave,
	// restrito aos combos que serão reescritos nesta geração.
	prevNetMaint := map[string]float64{}
	prevMetaMaint := map[string]model.SaldoConta{}
	// Linhas vigentes ANTES desta geração (ambos indicadores). Reusadas no passo 7 para a
	// contagem visível em memória — evita reler o dia do banco e preserva a contagem de combos
	// que não geraram linhas novas (cuja versão anterior permanece vigente).
	var vigenteAntes []model.LancamentoContabil
	for _, ind := range []bool{false, true} {
		vigentes, err := s.movimentoRepo.BuscarPorDataEIndicador(ctx, data, ind)
		if err != nil {
			return nil, fmt.Errorf("erro ao obter versão vigente: %w", err)
		}
		for _, l := range vigentes {
			vigenteAntes = append(vigenteAntes, l)
			k := l.Produto + "\x00" + l.Dominio
			if l.CodigoVersaoConteudo > maxVersaoCombo[k] {
				maxVersaoCombo[k] = l.CodigoVersaoConteudo
			}
			if s.saldoRepo != nil {
				if _, ok := combosSet[k]; ok {
					key, cd, cc := canonChaveLanc(l)
					sgn := 1.0
					if l.IndicadorReversao {
						sgn = -1
					}
					prevNetMaint[key] += sgn * l.ValorLancamentoContabil
					if _, ok := prevMetaMaint[key]; !ok {
						prevMetaMaint[key] = model.SaldoConta{Produto: l.Produto, Dominio: l.Dominio, CodigoIdentificadorBoleto: l.CodigoIdentificadorBoleto, IDRegraContabil: l.IDRegraContabil, ContaDebito: cd, ContaCredito: cc, MoedaLancamento: l.MoedaLancamentoContabil, DescricaoRegra: l.DescricaoRegraContabil, DescricaoCondicao: l.DescricaoCondicaoContabil}
					}
				}
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

	// 6b. Manutenção da projeção de saldo (RN-170/ADR-010): delta por chave =
	// net das linhas novas − net da versão vigente anterior (dos combos reescritos).
	// É aditivo e NÃO altera o movimento; falha aqui não aborta a geração (projeção reconstruível).
	if s.saldoRepo != nil {
		deltaByKey := map[string]float64{}
		metaByKey := map[string]model.SaldoConta{}
		for _, l := range todos {
			key, cd, cc := canonChaveLanc(l)
			sgn := 1.0
			if l.IndicadorReversao {
				sgn = -1
			}
			deltaByKey[key] += sgn * l.ValorLancamentoContabil
			metaByKey[key] = model.SaldoConta{Produto: l.Produto, Dominio: l.Dominio, CodigoIdentificadorBoleto: l.CodigoIdentificadorBoleto, IDRegraContabil: l.IDRegraContabil, ContaDebito: cd, ContaCredito: cc, MoedaLancamento: l.MoedaLancamentoContabil, DescricaoRegra: l.DescricaoRegraContabil, DescricaoCondicao: l.DescricaoCondicaoContabil}
		}
		for key, v := range prevNetMaint {
			deltaByKey[key] -= v
			if _, ok := metaByKey[key]; !ok {
				metaByKey[key] = prevMetaMaint[key]
			}
		}
		var deltas []model.SaldoConta
		for key, v := range deltaByKey {
			if v == 0 {
				continue
			}
			sc := metaByKey[key]
			sc.Saldo = v
			deltas = append(deltas, sc)
		}
		if err := s.saldoRepo.AplicarDeltas(ctx, deltas); err != nil {
			log.Printf("[saldo] falha ao aplicar deltas (projeção; reconstruível): %v", err)
		}
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
		// Contagem VISÍVEL (sem os pares lançamento+estorno de saldo zero) calculada EM MEMÓRIA,
		// em vez de reler o dia inteiro do banco com subquery correlacionada por linha (que era o
		// principal gargalo). O conjunto vigente final por combo é: as linhas recém-geradas quando o
		// combo foi reescrito; senão, as linhas vigentes anteriores (combo sem geração nova mantém o
		// estado anterior). A anulação de pares usa a MESMA chave canônica do backend (boleto + regra
		// + par de contas na orientação do lançamento normal), então o resultado é idêntico ao
		// ConsultarPaginadoFiltradoSemCancelados.
		type grpVis struct {
			boleto    string
			regra     int64
			deb, cred string
		}
		grupoDe := func(l model.LancamentoContabil) grpVis {
			deb, cred := l.ContaDebito, l.ContaCredito
			if l.IndicadorReversao {
				deb, cred = l.ContaCredito, l.ContaDebito
			}
			return grpVis{l.CodigoIdentificadorBoleto, l.IDRegraContabil, deb, cred}
		}
		combosComNovos := map[string]bool{}
		for _, l := range todos {
			combosComNovos[l.Produto+"\x00"+l.Dominio] = true
		}
		// Conjunto vigente final: linhas novas + vigentes anteriores dos combos NÃO reescritos.
		finais := make([]model.LancamentoContabil, 0, len(todos)+len(vigenteAntes))
		finais = append(finais, todos...)
		for _, l := range vigenteAntes {
			if combosComNovos[l.Produto+"\x00"+l.Dominio] {
				continue // combo reescrito → a versão nova substitui a anterior
			}
			finais = append(finais, l)
		}
		netGrupo := map[grpVis]float64{}
		for _, l := range finais {
			if l.IndicadorReversao {
				netGrupo[grupoDe(l)] -= l.ValorLancamentoContabil
			} else {
				netGrupo[grupoDe(l)] += l.ValorLancamentoContabil
			}
		}
		qtdLancVis := map[string]int{}
		qtdEstVis := map[string]int{}
		for _, l := range finais {
			if netGrupo[grupoDe(l)] == 0 {
				continue // par lançamento+estorno que se anula → não visível
			}
			k := l.Produto + "\x00" + l.Dominio
			if l.IndicadorReversao {
				qtdEstVis[k]++
			} else {
				qtdLancVis[k]++
			}
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

// PreviewExpressao avalia uma expressão (condição ou valor) contra uma linha de posição
// REAL da data informada (opcionalmente filtrando por produto/domínio/boleto), devolvendo o
// resultado, os campos referenciados e os ausentes. Permite testar a regra antes de ativá-la.
// Spec: RF-072.
func (s *MovimentoContabilService) PreviewExpressao(ctx context.Context, data time.Time, produto, dominio, boleto, campoBoleto, expressao, tipo string) (*model.PreviewExpressao, error) {
	posicoes, err := s.posicaoRepo.BuscarPorDataEVersaoMaxima(ctx, data)
	if err != nil {
		return nil, err
	}
	// O identificador do boleto é lido do campo informado na condição (campo_boleto), caindo
	// no padrão do sistema quando vazio — assim o filtro e o boleto exibido batem com a regra.
	campoBol := campoBoletoOuPadrao(campoBoleto)
	res := &model.PreviewExpressao{}
	var env map[string]interface{}
	for _, p := range posicoes {
		e := evaluator.PosicaoToEnv(p)
		if produto != "" && evaluator.CampoString(e, "produto") != produto {
			continue
		}
		if dominio != "" && evaluator.CampoString(e, "dominio") != dominio {
			continue
		}
		if boleto != "" && evaluator.CampoString(e, campoBol) != boleto {
			continue
		}
		env = e
		res.Boleto = evaluator.CampoString(e, campoBol)
		break
	}
	if env == nil {
		return res, nil // nenhuma posição casou → Encontrada=false
	}
	res.Encontrada = true
	res.CamposRef, _ = evaluator.CamposReferenciados(expressao)
	if faltantes := camposFaltantes(env, expressao); len(faltantes) > 0 {
		res.CamposFaltantes = faltantes
		res.Erro = "campos ausentes na posição: " + strings.Join(faltantes, ", ")
		return res, nil
	}
	if tipo == "valor" {
		v, err := s.evaluator.EvaluateValue(expressao, env)
		if err != nil {
			res.Erro = err.Error()
		} else {
			res.Resultado = v
		}
	} else {
		v, err := s.evaluator.EvaluateCondition(expressao, env)
		if err != nil {
			res.Erro = err.Error()
		} else {
			res.Resultado = v
		}
	}
	return res, nil
}

// SimularRegra avalia uma regra CANDIDATA (hipotética) sobre a posição de carteira da
// data/escopo e devolve os lançamentos que ela geraria, SEM persistir nada. Reaproveita a
// mesma lógica de avaliação da geração (pré-condição, condição, campo_valor, campo_moeda,
// campo_boleto, inconsistências por campo ausente). Não trata estorno, baixa, saldo nem
// conversão de moeda base — é uma projeção dos lançamentos NORMAIS. Spec: RF-122.
func (s *MovimentoContabilService) SimularRegra(ctx context.Context, in model.SimulacaoRegraInput) (*model.SimulacaoWhatIf, error) {
	data, err := time.Parse("2006-01-02", strings.TrimSpace(in.Data))
	if err != nil {
		return nil, fmt.Errorf("data inválida (use AAAA-MM-DD)")
	}
	limite := in.LimiteLinhas
	if limite <= 0 {
		limite = 500
	}
	if limite > 5000 {
		limite = 5000
	}
	// Ao menos uma condição com expressão.
	temCond := false
	for _, c := range in.Condicoes {
		if strings.TrimSpace(c.Condicao) != "" || strings.TrimSpace(c.CampoValor) != "" {
			temCond = true
			break
		}
	}
	if !temCond {
		return nil, fmt.Errorf("informe ao menos uma condição com expressão e campo_valor")
	}

	// Monta uma RegraContabil a partir da candidata para reusar a avaliação/boletoDaPosicao.
	// O casamento de produto/domínio vem do escopo (que já filtra as posições abaixo), então
	// a regra aqui não precisa de codigo_produto_corporativo/dominio.
	regra := model.RegraContabil{
		Descricao:      "(simulação)",
		PreCondicao:    in.PreCondicao,
		TipoLancamento: model.NormalizaTipoLancamento(in.TipoLancamento),
		Ativo:          true,
	}
	condicoes := make([]model.CondicaoRegra, 0, len(in.Condicoes))
	for _, c := range in.Condicoes {
		if strings.TrimSpace(c.Condicao) == "" && strings.TrimSpace(c.CampoValor) == "" {
			continue
		}
		condicoes = append(condicoes, model.CondicaoRegra{
			Condicao: c.Condicao, ContaDebito: c.ContaDebito, ContaCredito: c.ContaCredito,
			CampoValor: c.CampoValor, CampoMoeda: c.CampoMoeda, CampoBoleto: c.CampoBoleto, Ativo: true,
			BaseNaPosicao:       c.BaseNaPosicao,
			CampoValorMoedaBase: c.CampoValorMoedaBase,
			CampoMoedaBase:      c.CampoMoedaBase,
			CampoDataConversao:  c.CampoDataConversao,
		})
	}
	regra.Condicoes = condicoes

	posicoes, err := s.posicaoRepo.BuscarPorDataEVersaoMaxima(ctx, data)
	if err != nil {
		return nil, fmt.Errorf("erro ao buscar posicao_carteira: %w", err)
	}

	// Moeda base do veículo (RF-036/RN-182): carrega De/Para e cotações uma vez e resolve
	// o veículo/layout por combinação a partir do padrão — mesma lógica da geração — para
	// reproduzir na simulação o valor na moeda base do veículo.
	moedaBasePorVeiculo := map[string]string{}
	if s.moedaBaseRepo != nil {
		if m, err := s.moedaBaseRepo.MapaVeiculoMoedaBase(ctx); err == nil {
			moedaBasePorVeiculo = m
		}
	}
	cotacoes := map[string]model.Cotacao{}
	if s.cotacaoRepo != nil {
		if m, err := s.cotacaoRepo.MapaCotacoes(ctx); err == nil {
			cotacoes = m
		}
	}
	var padroes []model.PadraoArquivo
	if s.padraoRepo != nil {
		padroes, _ = s.padraoRepo.Listar(ctx)
	}
	padraoDe := func(produto, dominio string) (colVeiculo, layoutData string) {
		for _, p := range padroes {
			if strings.TrimSpace(p.Produto) == produto && strings.TrimSpace(p.Dominio) == dominio {
				return evaluator.NormalizarNome(p.ColunaVeiculo), evaluator.LayoutData(p.FormatoData)
			}
		}
		return "", ""
	}

	res := &model.SimulacaoWhatIf{}
	if regra.UsaCalculoIncremental() {
		res.Aviso = "Regra incremental: a simulação mostra o valor BRUTO de campo_valor em D0 (o valor diário real |D0 − D-N| depende do histórico persistido desta regra)."
	}

	boletoFiltro := strings.TrimSpace(in.Boleto)
	addInconsistencia := func(boleto, tipo, expr, detalhe string) {
		res.TotalInconsistencias++
		if len(res.Inconsistencias) < limite {
			res.Inconsistencias = append(res.Inconsistencias, model.SimulacaoInconsistencia{
				Boleto: boleto, Tipo: tipo, Expressao: expr, Detalhe: detalhe,
			})
		}
	}

	for _, posicao := range posicoes {
		env := evaluator.PosicaoToEnv(posicao)
		produto := evaluator.CampoString(env, "produto")
		dominio := evaluator.CampoString(env, "dominio")
		// Escopo: produto/domínio filtram as posições avaliadas.
		if in.Produto != "" && produto != strings.TrimSpace(in.Produto) {
			continue
		}
		if in.Dominio != "" && dominio != strings.TrimSpace(in.Dominio) {
			continue
		}
		boleto := boletoDaPosicao(env, regra.Condicoes)
		// Escopo opcional por boleto: avalia só o boleto informado (resolvido pelo campo_boleto da regra).
		if boletoFiltro != "" && boleto != boletoFiltro {
			continue
		}
		res.PosicoesAvaliadas++

		// Pré-condição (opcional): campo ausente → inconsistência; falsa → não gera.
		if strings.TrimSpace(regra.PreCondicao) != "" {
			if faltantes := camposFaltantes(env, regra.PreCondicao); len(faltantes) > 0 {
				addInconsistencia(boleto, "pré-condição", regra.PreCondicao,
					"referencia campo(s) inexistente(s) na posição: "+strings.Join(faltantes, ", "))
				continue
			}
			ok, err := s.evaluator.EvaluateCondition(regra.PreCondicao, env)
			if err != nil {
				addInconsistencia(boleto, "erro na pré-condição", regra.PreCondicao, err.Error())
				continue
			}
			if !ok {
				continue
			}
		}

		for _, condicao := range regra.Condicoes {
			if faltantes := camposFaltantes(env, condicao.Condicao); len(faltantes) > 0 {
				addInconsistencia(boleto, "condição", condicao.Condicao,
					"referencia campo(s) inexistente(s) na posição: "+strings.Join(faltantes, ", "))
				continue
			}
			if strings.TrimSpace(condicao.Condicao) != "" {
				ok, err := s.evaluator.EvaluateCondition(condicao.Condicao, env)
				if err != nil {
					addInconsistencia(boleto, "erro na condição", condicao.Condicao, err.Error())
					continue
				}
				if !ok {
					continue
				}
			}
			// Condição satisfeita (ou vazia = sempre): avalia campo_valor.
			if faltantes := camposFaltantes(env, condicao.CampoValor); len(faltantes) > 0 {
				addInconsistencia(boleto, "campo valor", condicao.CampoValor,
					"referencia campo(s) inexistente(s) na posição: "+strings.Join(faltantes, ", "))
				continue
			}
			valor, err := s.evaluator.EvaluateValue(condicao.CampoValor, env)
			if err != nil {
				addInconsistencia(boleto, "erro no campo valor", condicao.CampoValor, err.Error())
				continue
			}
			if faltantes := camposFaltantes(env, condicao.CampoMoeda); len(faltantes) > 0 {
				addInconsistencia(boleto, "campo moeda", condicao.CampoMoeda,
					"referencia campo(s) inexistente(s) na posição: "+strings.Join(faltantes, ", "))
				continue
			}
			moeda := evaluator.CampoString(env, condicao.CampoMoeda)
			if strings.TrimSpace(moeda) == "" {
				addInconsistencia(boleto, "campo moeda", condicao.CampoMoeda,
					"a moeda resultou em branco (campo vazio/ausente na posição)")
				continue
			}
			boletoLanc := evaluator.CampoString(env, campoBoletoOuPadrao(condicao.CampoBoleto))

			// Valor na moeda base do veículo (RF-036/RN-182), igual à geração: quando a condição
			// configura a base e ela não pode ser resolvida (campo ausente, veículo sem De/Para,
			// cotação faltante), vira inconsistência e NÃO gera lançamento — como na geração real.
			colVeiculo, layoutData := padraoDe(produto, dominio)
			veiculo := evaluator.CampoString(env, colVeiculo)
			valorBase, moedaBase, incMB := s.resolverMoedaBaseVeiculo(
				data, boletoLanc, produto, dominio, regra, condicao, env, veiculo, valor, moeda, moedaBasePorVeiculo, cotacoes, layoutData)
			if incMB != nil {
				det := incMB.Detalhe
				if det == "" && incMB.CamposFaltantes != "" {
					det = "campo(s) ausente(s) na posição: " + incMB.CamposFaltantes
				}
				addInconsistencia(boletoLanc, "moeda base", incMB.Expressao, det)
				continue
			}

			res.TotalLancamentos++
			if len(res.Lancamentos) < limite {
				res.Lancamentos = append(res.Lancamentos, model.SimulacaoLancamento{
					Boleto: boletoLanc, Valor: valor, Moeda: moeda,
					ValorMoedaBaseVeiculo: valorBase, MoedaBaseVeiculo: moedaBase,
					ContaDebito: condicao.ContaDebito, ContaCredito: condicao.ContaCredito,
					Produto: produto, Dominio: dominio, Condicao: condicao.Condicao,
				})
			}
		}
	}

	res.Truncado = res.TotalLancamentos > len(res.Lancamentos) || res.TotalInconsistencias > len(res.Inconsistencias)
	return res, nil
}

// Razonete monta o razonete (livro razão em T) por conta, a partir dos lançamentos
// vigentes do período. Cada lançamento contribui com um DÉBITO na conta_debito e um
// CRÉDITO na conta_credito (partida dobrada); estornos aparecem no lado oposto. Filtros
// opcionais: boleto, conta (débito OU crédito), produto, domínio. Spec: RF-070.
func (s *MovimentoContabilService) Razonete(ctx context.Context, dataInicio, dataFim time.Time, boleto, conta, produto, dominio, versaoModo string) ([]model.RazoneteConta, error) {
	if strings.TrimSpace(boleto) == "" {
		return nil, fmt.Errorf("informe o número do boleto")
	}
	if versaoModo == "" {
		versaoModo = "vigente"
	}
	pag, err := s.movimentoRepo.ConsultarPaginadoFiltrado(ctx, model.FiltroMovimento{
		DataInicio: dataInicio, DataFim: dataFim, Boleto: boleto, BoletoExato: true, Produto: produto, Dominio: dominio,
		VersaoModo: versaoModo, Pagina: 1, Tamanho: 1000000,
	})
	if err != nil {
		return nil, err
	}
	contaFiltro := strings.TrimSpace(conta)

	type chave struct{ conta, moeda string }
	grupos := map[chave]*model.RazoneteConta{}
	var ordem []chave
	add := func(c, moeda string, l model.LancamentoContabil, deb, cred float64) {
		if contaFiltro != "" && c != contaFiltro {
			return
		}
		k := chave{c, moeda}
		g, ok := grupos[k]
		if !ok {
			g = &model.RazoneteConta{Conta: c, Moeda: moeda}
			grupos[k] = g
			ordem = append(ordem, k)
		}
		g.Linhas = append(g.Linhas, model.RazoneteLinha{
			Data: l.DataLoteContabil.Format("2006-01-02"), Boleto: l.CodigoIdentificadorBoleto,
			Historico: l.DescricaoRegraContabil, Debito: deb, Credito: cred, Reversao: l.IndicadorReversao, Moeda: moeda,
		})
	}
	for _, l := range pag.Lancamentos {
		add(l.ContaDebito, l.MoedaLancamentoContabil, l, l.ValorLancamentoContabil, 0)
		add(l.ContaCredito, l.MoedaLancamentoContabil, l, 0, l.ValorLancamentoContabil)
	}

	out := make([]model.RazoneteConta, 0, len(ordem))
	for _, k := range ordem {
		g := grupos[k]
		sort.SliceStable(g.Linhas, func(i, j int) bool { return g.Linhas[i].Data < g.Linhas[j].Data })
		saldo := 0.0
		for i := range g.Linhas {
			saldo += g.Linhas[i].Debito - g.Linhas[i].Credito
			g.Linhas[i].SaldoAcumulado = saldo
			g.TotalDebito += g.Linhas[i].Debito
			g.TotalCredito += g.Linhas[i].Credito
		}
		g.Saldo = g.TotalDebito - g.TotalCredito
		g.Natureza = "D"
		if g.Saldo < 0 {
			g.Natureza = "C"
		}
		out = append(out, *g)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Conta != out[j].Conta {
			return out[i].Conta < out[j].Conta
		}
		return out[i].Moeda < out[j].Moeda
	})
	return out, nil
}

// ConsultarLancamentos retorna os lançamentos paginados para a data informada.
func (s *MovimentoContabilService) ConsultarLancamentos(ctx context.Context, data time.Time, pagina, tamanho int) (*model.PaginaLancamentos, error) {
	return s.movimentoRepo.ConsultarPaginado(ctx, data, pagina, tamanho)
}

// ConsultarLancamentosFiltrado retorna lançamentos paginados por período, boleto e versão.
// Elimina lançamentos cujo saldo líquido (normal - reversão) é zero — usado pela página de consulta do frontend.
func (s *MovimentoContabilService) ConsultarLancamentosFiltrado(ctx context.Context, dataInicio, dataFim time.Time, boleto, veiculo string, versao int, versaoModo string, pagina, tamanho int) (*model.PaginaLancamentos, error) {
	return s.movimentoRepo.ConsultarPaginadoFiltradoSemCancelados(ctx, model.FiltroMovimento{DataInicio: dataInicio, DataFim: dataFim, Boleto: boleto, Veiculo: veiculo, Versao: versao, VersaoModo: versaoModo, Pagina: pagina, Tamanho: tamanho})
}

// ConsultarLancamentosFiltradoEscopo funciona como ConsultarLancamentosFiltrado, mas
// permite restringir por produto e/ou domínio (ambos opcionais). Quando os dois estão
// vazios, delega diretamente ao repositório (paginação eficiente). Quando há filtro de
// escopo, busca todos os lançamentos que atendem aos demais critérios, filtra por
// produto/domínio e pagina o resultado — mantendo a contagem total correta.
func (s *MovimentoContabilService) ConsultarLancamentosFiltradoEscopo(ctx context.Context, dataInicio, dataFim time.Time, boleto, veiculo, produto, dominio string, versao int, versaoModo string, pagina, tamanho int) (*model.PaginaLancamentos, error) {
	// Produto/domínio entram no filtro do repositório (WHERE condicional), mantendo a paginação
	// no banco em todos os casos — inclusive quando há escopo. Campos vazios não filtram.
	return s.movimentoRepo.ConsultarPaginadoFiltradoSemCancelados(ctx, model.FiltroMovimento{
		DataInicio: dataInicio, DataFim: dataFim, Boleto: boleto, Veiculo: veiculo,
		Produto: produto, Dominio: dominio, Versao: versao, VersaoModo: versaoModo,
		Pagina: pagina, Tamanho: tamanho,
	})
}

// ExcluirMovimento exclui lançamentos de uma data e opcionalmente de uma versão específica.
func (s *MovimentoContabilService) ExcluirMovimento(ctx context.Context, data time.Time, versao int) error {
	// Captura os combos afetados ANTES de excluir (para recomputar o saldo — RN-170.3).
	var afetados []model.ProdutoDominio
	if s.saldoRepo != nil {
		set := map[string]model.ProdutoDominio{}
		for _, ind := range []bool{false, true} {
			if ls, err := s.movimentoRepo.BuscarPorDataEIndicador(ctx, data, ind); err == nil {
				for _, l := range ls {
					set[l.Produto+"\x00"+l.Dominio] = model.ProdutoDominio{Produto: l.Produto, Dominio: l.Dominio}
				}
			}
		}
		for _, c := range set {
			afetados = append(afetados, c)
		}
	}
	if err := s.movimentoRepo.ExcluirPorDataEVersao(ctx, data, versao); err != nil {
		return err
	}
	// Recomputa o saldo dos combos afetados a partir do movimento vigente (projeção; não falha a exclusão).
	if s.saldoRepo != nil && len(afetados) > 0 {
		filtro := map[string]bool{}
		for _, c := range afetados {
			filtro[c.Produto+"\x00"+c.Dominio] = true
		}
		if saldos, err := s.calcularSaldos(ctx, filtro); err == nil {
			if err := s.saldoRepo.SubstituirCombos(ctx, afetados, saldos); err != nil {
				log.Printf("[saldo] falha ao recomputar após exclusão (reconstruível): %v", err)
			}
		} else {
			log.Printf("[saldo] falha ao calcular saldos após exclusão (reconstruível): %v", err)
		}
	}
	return nil
}

// calcularSaldos recomputa o net vigente por chave canônica a partir do movimento_contabil
// (fonte da verdade). Se filtroCombos != nil, restringe aos combos informados. Base da
// reconstrução (RN-170.1) e da recomputação pós-exclusão.
func (s *MovimentoContabilService) calcularSaldos(ctx context.Context, filtroCombos map[string]bool) ([]model.SaldoConta, error) {
	pag, err := s.movimentoRepo.ConsultarPaginadoFiltrado(ctx, model.FiltroMovimento{DataInicio: time.Date(1900, 1, 1, 0, 0, 0, 0, time.UTC), DataFim: time.Date(2999, 12, 31, 0, 0, 0, 0, time.UTC), VersaoModo: "vigente", Pagina: 1, Tamanho: 999999})
	if err != nil {
		return nil, err
	}
	net := map[string]float64{}
	meta := map[string]model.SaldoConta{}
	for _, l := range pag.Lancamentos {
		if filtroCombos != nil && !filtroCombos[l.Produto+"\x00"+l.Dominio] {
			continue
		}
		key, cd, cc := canonChaveLanc(l)
		sgn := 1.0
		if l.IndicadorReversao {
			sgn = -1
		}
		net[key] += sgn * l.ValorLancamentoContabil
		meta[key] = model.SaldoConta{Produto: l.Produto, Dominio: l.Dominio, CodigoIdentificadorBoleto: l.CodigoIdentificadorBoleto, IDRegraContabil: l.IDRegraContabil, ContaDebito: cd, ContaCredito: cc, MoedaLancamento: l.MoedaLancamentoContabil, DescricaoRegra: l.DescricaoRegraContabil, DescricaoCondicao: l.DescricaoCondicaoContabil}
	}
	out := make([]model.SaldoConta, 0, len(net))
	for key, v := range net {
		sc := meta[key]
		sc.Saldo = v
		out = append(out, sc)
	}
	return out, nil
}

// ReconstruirSaldos recompõe toda a projeção de saldo a partir do movimento vigente.
// Usada na primeira carga (base recém-criada) ou para corrigir eventual divergência.
func (s *MovimentoContabilService) ReconstruirSaldos(ctx context.Context) error {
	if s.saldoRepo == nil {
		return nil
	}
	saldos, err := s.calcularSaldos(ctx, nil)
	if err != nil {
		return err
	}
	comboSet := map[string]model.ProdutoDominio{}
	for _, sc := range saldos {
		comboSet[sc.Produto+"\x00"+sc.Dominio] = model.ProdutoDominio{Produto: sc.Produto, Dominio: sc.Dominio}
	}
	var combos []model.ProdutoDominio
	for _, c := range comboSet {
		combos = append(combos, c)
	}
	return s.saldoRepo.SubstituirCombos(ctx, combos, saldos)
}

// ListarSaldos expõe a projeção de saldo para consulta (RF-065).
func (s *MovimentoContabilService) ListarSaldos(ctx context.Context, filtro model.SaldoFiltro) ([]model.SaldoConta, error) {
	if s.saldoRepo == nil {
		return []model.SaldoConta{}, nil
	}
	return s.saldoRepo.Listar(ctx, filtro)
}

// ListarSaldosPaginado expõe a projeção de saldo paginada para o menu Saldos (RF-065).
func (s *MovimentoContabilService) ListarSaldosPaginado(ctx context.Context, filtro model.SaldoFiltro, pagina, tamanho int) (*model.SaldoPagina, error) {
	if s.saldoRepo == nil {
		return &model.SaldoPagina{Pagina: pagina, Tamanho: tamanho, Saldos: []model.SaldoConta{}}, nil
	}
	return s.saldoRepo.ListarPaginado(ctx, filtro, pagina, tamanho)
}

// SaldoVazio informa se a projeção de saldo ainda não foi populada.
func (s *MovimentoContabilService) SaldoVazio(ctx context.Context) (bool, error) {
	if s.saldoRepo == nil {
		return true, nil
	}
	return s.saldoRepo.EstaVazio(ctx)
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
		model.InconsistenciaCampoMoeda:  "campo moeda",
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

// canonChaveLanc devolve a chave canônica de um lançamento (orientação de débito/crédito do
// lançamento NÃO-estorno), de modo que um lançamento e seu estorno mapeiem para a MESMA chave.
// Retorna também as contas na orientação canônica.
func canonChaveLanc(l model.LancamentoContabil) (chave, contaDeb, contaCred string) {
	contaDeb, contaCred = l.ContaDebito, l.ContaCredito
	if l.IndicadorReversao {
		contaDeb, contaCred = l.ContaCredito, l.ContaDebito
	}
	return chaveIncremental(l.Produto, l.Dominio, l.CodigoIdentificadorBoleto, l.IDRegraContabil, contaDeb, contaCred), contaDeb, contaCred
}

// resolverMoedaBaseVeiculo calcula (valor, moeda) na moeda base do veículo para um lançamento,
// conforme a configuração da condição (RF-036/RN-182):
//   - base vem da posição → lê valor e moeda base das colunas configuradas;
//   - caso contrário → converte: se a moeda do veículo (De/Para) == moeda do lançamento, usa o
//     próprio valor/moeda; senão, busca a cotação (RF-037) da data base e aplica a direção.
//
// Devolve uma inconsistência (!= nil) quando a condição exige a base mas não é possível obtê-la
// (campo ausente, moeda base do veículo inexistente, data inválida ou cotação não encontrada).
// Quando a condição não configura a base, devolve valores em branco e inc=nil (lançamento normal).
func (s *MovimentoContabilService) resolverMoedaBaseVeiculo(
	data time.Time, boleto, produto, dominio string, regra model.RegraContabil, condicao model.CondicaoRegra,
	env map[string]interface{}, veiculo string, valor float64, moeda string,
	moedaBasePorVeiculo map[string]string, cotacoes map[string]model.Cotacao, layoutData string,
) (float64, string, *model.InconsistenciaProcessamento) {

	incMoedaBase := func(campo, detalhe string, faltantes []string) *model.InconsistenciaProcessamento {
		inc := novaInconsistencia(data, boleto, produto, dominio, regra, model.InconsistenciaMoedaBase, campo, faltantes)
		if detalhe != "" {
			inc.Detalhe = detalhe
		}
		return &inc
	}

	if condicao.BaseNaPosicao {
		if faltantes := camposFaltantes(env, condicao.CampoValorMoedaBase); len(faltantes) > 0 {
			return 0, "", incMoedaBase(condicao.CampoValorMoedaBase, "", faltantes)
		}
		if faltantes := camposFaltantes(env, condicao.CampoMoedaBase); len(faltantes) > 0 {
			return 0, "", incMoedaBase(condicao.CampoMoedaBase, "", faltantes)
		}
		vb, err := s.evaluator.EvaluateValue(condicao.CampoValorMoedaBase, env)
		if err != nil {
			return 0, "", incMoedaBase(condicao.CampoValorMoedaBase, fmt.Sprintf("Valor na moeda base inválido na posição (campo %q).", condicao.CampoValorMoedaBase), nil)
		}
		mb := strings.TrimSpace(evaluator.CampoString(env, condicao.CampoMoedaBase))
		if mb == "" {
			return 0, "", incMoedaBase(condicao.CampoMoedaBase, fmt.Sprintf("Moeda base do veículo em branco na posição (campo %q).", condicao.CampoMoedaBase), nil)
		}
		return vb, mb, nil
	}

	// Sem coluna de data configurada → condição sem configuração de moeda base (não converte).
	if strings.TrimSpace(condicao.CampoDataConversao) == "" {
		return 0, "", nil
	}

	// Converter: precisa da moeda base do veículo (De/Para).
	moedaVeic := strings.TrimSpace(moedaBasePorVeiculo[chaveVeiculo(veiculo)])
	if moedaVeic == "" {
		return 0, "", incMoedaBase("moeda_base", fmt.Sprintf("Veículo %q sem moeda base cadastrada (De/Para). Não é possível converter.", veiculo), nil)
	}
	// Mesma moeda → sem conversão.
	if strings.EqualFold(moedaVeic, strings.TrimSpace(moeda)) {
		return valor, moedaVeic, nil
	}
	// Moedas diferentes → busca a cotação da data base presente na posição.
	if faltantes := camposFaltantes(env, condicao.CampoDataConversao); len(faltantes) > 0 {
		return 0, "", incMoedaBase(condicao.CampoDataConversao, "", faltantes)
	}
	// A coluna da data de conversão pode vir como time.Time ou como texto no formato do padrão;
	// normaliza para ISO (AAAA-MM-DD), que é a chave de busca da cotação.
	dataConv, ok := evaluator.NormalizarDataISO(env[condicao.CampoDataConversao], layoutData)
	if !ok {
		return 0, "", incMoedaBase(condicao.CampoDataConversao, fmt.Sprintf("Data base para conversão inválida na posição (campo %q = %q).", condicao.CampoDataConversao, evaluator.CampoString(env, condicao.CampoDataConversao)), nil)
	}
	cot, ok := cotacoes[chaveCotacao(dataConv, moedaVeic, moeda)]
	if !ok {
		return 0, "", incMoedaBase("cotacao", fmt.Sprintf("Cotação não encontrada para %s→%s em %s. Lançamento não gerado.", moedaVeic, strings.TrimSpace(moeda), dataConv), nil)
	}
	return aplicarCotacao(valor, cot), moedaVeic, nil
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
		if !regra.UsaCalculoIncremental() || !regraAplicaAPosicao(regra, env) {
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
// InserirLancamentosManuais insere N lançamentos informados pelo usuário (tela ou CSV).
// As contas/descrições vêm da regra+condição referenciadas; os lançamentos são marcados como
// origem=manual e anexados à versão vigente do combo/data (ou versão 1 se não houver). Atualiza
// a projeção de saldo (RN-170). Validação é "tudo ou nada": se qualquer linha for inválida,
// nada é inserido. Spec: RF-033 (docs/especificacao.md §5b, §11.1).
func (s *MovimentoContabilService) InserirLancamentosManuais(ctx context.Context, itens []model.LancamentoManualInput) (int, error) {
	if len(itens) == 0 {
		return 0, nil
	}
	regras, err := s.regraRepo.ListarRegrasAtivas(ctx)
	if err != nil {
		return 0, fmt.Errorf("erro ao carregar regras: %w", err)
	}
	regraByID := map[int64]model.RegraContabil{}
	for _, r := range regras {
		regraByID[r.ID] = r
	}

	lancs := make([]model.LancamentoContabil, 0, len(itens))
	for i, it := range itens {
		linha := i + 1
		data, err := time.Parse("2006-01-02", strings.TrimSpace(it.Data))
		if err != nil {
			return 0, fmt.Errorf("linha %d: data inválida (use YYYY-MM-DD)", linha)
		}
		if !EhDiaUtil(data) {
			return 0, fmt.Errorf("linha %d: %s não é dia útil", linha, data.Format("2006-01-02"))
		}
		if strings.TrimSpace(it.Produto) == "" || strings.TrimSpace(it.Dominio) == "" {
			return 0, fmt.Errorf("linha %d: produto e domínio são obrigatórios", linha)
		}
		if strings.TrimSpace(it.Boleto) == "" {
			return 0, fmt.Errorf("linha %d: boleto é obrigatório", linha)
		}
		if it.Valor == 0 {
			return 0, fmt.Errorf("linha %d: valor deve ser diferente de zero", linha)
		}
		reg, ok := regraByID[it.IDRegra]
		if !ok {
			return 0, fmt.Errorf("linha %d: regra %d não encontrada (ou inativa)", linha, it.IDRegra)
		}
		var cond *model.CondicaoRegra
		for j := range reg.Condicoes {
			if reg.Condicoes[j].ID == it.IDCondicao {
				cond = &reg.Condicoes[j]
				break
			}
		}
		if cond == nil {
			return 0, fmt.Errorf("linha %d: condição %d não pertence à regra %d (ou inativa)", linha, it.IDCondicao, it.IDRegra)
		}
		// Estorno manual: inverte débito/crédito da condição (mesma convenção do estorno
		// automático — passo 4), para que a lógica de net/saldo/"sem cancelados" desinverta
		// via canonChaveLanc e o estorno case/anule o lançamento normal da mesma condição.
		contaDeb, contaCred := cond.ContaDebito, cond.ContaCredito
		if it.IndicadorReversao {
			contaDeb, contaCred = cond.ContaCredito, cond.ContaDebito
		}
		lancs = append(lancs, model.LancamentoContabil{
			DataLoteContabil:          data,
			CodigoIdentificadorBoleto: strings.TrimSpace(it.Boleto),
			ValorLancamentoContabil:   it.Valor,
			MoedaLancamentoContabil:   strings.TrimSpace(it.Moeda),
			ContaDebito:               contaDeb,
			ContaCredito:              contaCred,
			Produto:                   strings.TrimSpace(it.Produto),
			Dominio:                   strings.TrimSpace(it.Dominio),
			IndicadorReversao:         it.IndicadorReversao,
			DescricaoRegraContabil:    reg.Descricao,
			DescricaoCondicaoContabil: cond.Condicao,
			IDRegraContabil:           reg.ID,
			Origem:                    model.OrigemManual,
			Veiculo:                   strings.TrimSpace(it.Veiculo),
		})
	}

	// Versão por combo/data: anexa à vigente atual (max) ou versão 1 se não houver.
	versaoCache := map[string]int{}
	for i := range lancs {
		l := &lancs[i]
		ck := fmtDataSvc(l.DataLoteContabil) + "\x00" + l.Produto + "\x00" + l.Dominio
		v, ok := versaoCache[ck]
		if !ok {
			v = s.versaoVigenteCombo(ctx, l.DataLoteContabil, l.Produto, l.Dominio)
			versaoCache[ck] = v
		}
		l.CodigoVersaoConteudo = v
	}

	if err := s.movimentoRepo.BulkInsert(ctx, lancs); err != nil {
		return 0, fmt.Errorf("erro ao inserir lançamentos manuais: %w", err)
	}

	// Manutenção do saldo (RN-170): anexo à vigente → soma o net das linhas inseridas.
	if s.saldoRepo != nil {
		deltaByKey := map[string]float64{}
		metaByKey := map[string]model.SaldoConta{}
		for _, l := range lancs {
			key, cd, cc := canonChaveLanc(l)
			sgn := 1.0
			if l.IndicadorReversao {
				sgn = -1
			}
			deltaByKey[key] += sgn * l.ValorLancamentoContabil
			metaByKey[key] = model.SaldoConta{Produto: l.Produto, Dominio: l.Dominio, CodigoIdentificadorBoleto: l.CodigoIdentificadorBoleto, IDRegraContabil: l.IDRegraContabil, ContaDebito: cd, ContaCredito: cc, MoedaLancamento: l.MoedaLancamentoContabil, DescricaoRegra: l.DescricaoRegraContabil, DescricaoCondicao: l.DescricaoCondicaoContabil}
		}
		var deltas []model.SaldoConta
		for key, v := range deltaByKey {
			if v == 0 {
				continue
			}
			sc := metaByKey[key]
			sc.Saldo = v
			deltas = append(deltas, sc)
		}
		if err := s.saldoRepo.AplicarDeltas(ctx, deltas); err != nil {
			log.Printf("[saldo] falha ao aplicar deltas de lançamento manual (reconstruível): %v", err)
		}
	}
	return len(lancs), nil
}

// versaoVigenteCombo devolve a maior versão vigente do combo na data (ou 1 se não houver).
func (s *MovimentoContabilService) versaoVigenteCombo(ctx context.Context, data time.Time, produto, dominio string) int {
	maxv := 0
	for _, ind := range []bool{false, true} {
		ls, err := s.movimentoRepo.BuscarPorDataEIndicador(ctx, data, ind)
		if err != nil {
			continue
		}
		for _, l := range ls {
			if l.Produto == produto && l.Dominio == dominio && l.CodigoVersaoConteudo > maxv {
				maxv = l.CodigoVersaoConteudo
			}
		}
	}
	if maxv == 0 {
		return 1
	}
	return maxv
}

// fmtDataSvc formata a data no padrão ISO (chave de cache).
func fmtDataSvc(t time.Time) string { return t.Format("2006-01-02") }

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
