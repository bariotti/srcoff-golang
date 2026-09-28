package service

import (
	"context"
	"fmt"
	"log"
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

// execucaoRepoWriter registra a execução do contábil por combinação.
type execucaoRepoWriter interface {
	RegistrarExecucao(ctx context.Context, e model.MovimentoExecucao) error
}

// MovimentoContabilService implementa a lógica de geração e consulta de movimentos contábeis.
type MovimentoContabilService struct {
	posicaoRepo        posicaoCarteiraRepo
	regraRepo          regraContabilRepo
	movimentoRepo      movimentoContabilRepo
	evaluator          evaluator.Evaluator
	inconsistenciaRepo inconsistenciaRepoWriter
	execucaoRepo       execucaoRepoWriter
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

// GerarMovimento processa toda a data (todas as combinações). Mantido para
// compatibilidade; delega para GerarMovimentoEscopo sem filtro.
func (s *MovimentoContabilService) GerarMovimento(ctx context.Context, data time.Time) error {
	return s.GerarMovimentoEscopo(ctx, data, "", "")
}

// GerarMovimentoEscopo processa a posição de carteira para a data, restrita ao
// Produto e/ou Domínio informados (vazio = sem filtro naquela dimensão). Avalia as
// regras ativas do escopo, gera lançamentos e estornos de D-1 (também do escopo),
// grava produto/domínio em cada lançamento e registra a execução por combinação.
func (s *MovimentoContabilService) GerarMovimentoEscopo(ctx context.Context, data time.Time, produtoFiltro, dominioFiltro string) error {
	// 1. Buscar posição com versão máxima para a data
	todasPosicoes, err := s.posicaoRepo.BuscarPorDataEVersaoMaxima(ctx, data)
	if err != nil {
		return fmt.Errorf("erro ao buscar posicao_carteira: %w", err)
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
		return fmt.Errorf("nenhum registro de posicao_carteira encontrado para a data %s no escopo informado", data.Format("2006-01-02"))
	}

	// 2. Carregar todas as regras e condições ativas
	regras, err := s.regraRepo.ListarRegrasAtivas(ctx)
	if err != nil {
		return fmt.Errorf("erro ao carregar regras contábeis: %w", err)
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

	// Combinações (produto, domínio) processadas neste escopo.
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

	// 4. Gerar estornos de D-1 em memória, restritos ao escopo (produto/domínio).
	dMenos1 := data.AddDate(0, 0, -1)
	lancamentosD1, err := s.movimentoRepo.BuscarPorDataEIndicador(ctx, dMenos1, false)
	if err != nil {
		return fmt.Errorf("erro ao buscar lançamentos de D-1: %w", err)
	}

	var estornos []model.LancamentoContabil
	if len(lancamentosD1) > 0 {
		regras, err := s.regraRepo.ListarRegrasAtivas(ctx)
		if err != nil {
			return fmt.Errorf("erro ao carregar regras para estorno: %w", err)
		}
		regraPostaReverte := make(map[int64]bool, len(regras))
		for _, reg := range regras {
			regraPostaReverte[reg.ID] = reg.PostaReverte
		}

		for _, l1 := range lancamentosD1 {
			// Restringe o estorno ao escopo processado.
			if produtoFiltro != "" && l1.Produto != produtoFiltro {
				continue
			}
			if dominioFiltro != "" && l1.Dominio != dominioFiltro {
				continue
			}
			if pr, found := regraPostaReverte[l1.IDRegraContabil]; found && !pr {
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
		// Garante que combinações vindas do estorno (sem posição hoje) também sejam registradas.
		for _, e := range estornos {
			combosSet[e.Produto+"\x00"+e.Dominio] = model.ProdutoDominio{Produto: e.Produto, Dominio: e.Dominio}
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
			return fmt.Errorf("erro ao obter versão vigente: %w", err)
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
		return fmt.Errorf("erro ao persistir lançamentos: %w", err)
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

	// 7. Registrar a execução por combinação (data, produto, domínio).
	if s.execucaoRepo != nil {
		for _, c := range combosSet {
			k := c.Produto + "\x00" + c.Dominio
			if err := s.execucaoRepo.RegistrarExecucao(ctx, model.MovimentoExecucao{
				DataLote: data, Produto: c.Produto, Dominio: c.Dominio,
				QtdLancamentos: qtdLanc[k], QtdEstornos: qtdEst[k],
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
	return nil
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

// gerarEstornoInterno busca lançamentos de D-1 (versão vigente) e gera estornos para D.
func (s *MovimentoContabilService) gerarEstornoInterno(ctx context.Context, data time.Time) error {
	dMenos1 := data.AddDate(0, 0, -1)

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
