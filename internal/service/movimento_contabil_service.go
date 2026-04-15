package service

import (
	"context"
	"fmt"
	"log"
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

// MovimentoContabilService implementa a lógica de geração e consulta de movimentos contábeis.
type MovimentoContabilService struct {
	posicaoRepo   posicaoCarteiraRepo
	regraRepo     regraContabilRepo
	movimentoRepo movimentoContabilRepo
	evaluator     evaluator.Evaluator
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

// GerarMovimento processa a posição de carteira para a data informada, avalia as regras
// contábeis ativas, gera os estornos de D-1 em memória e persiste tudo em um único BulkInsert.
func (s *MovimentoContabilService) GerarMovimento(ctx context.Context, data time.Time) error {
	// 1. Buscar posição com versão máxima para a data
	posicoes, err := s.posicaoRepo.BuscarPorDataEVersaoMaxima(ctx, data)
	if err != nil {
		return fmt.Errorf("erro ao buscar posicao_carteira: %w", err)
	}
	if len(posicoes) == 0 {
		return fmt.Errorf("nenhum registro de posicao_carteira encontrado para a data %s", data.Format("2006-01-02"))
	}

	// 2. Carregar todas as regras e condições ativas
	regras, err := s.regraRepo.ListarRegrasAtivas(ctx)
	if err != nil {
		return fmt.Errorf("erro ao carregar regras contábeis: %w", err)
	}

	// 3. Gerar lançamentos de D em memória
	var lancamentos []model.LancamentoContabil
	for _, posicao := range posicoes {
		env := evaluator.PosicaoToEnv(posicao)
		for _, regra := range regras {
			// Filtrar pelo produto: só aplica a regra se o produto coincidir.
			// Se posição ou regra não tiver produto definido, aplica para todos (retrocompatibilidade).
			if regra.CodigoProdutoCorporativo != "" && posicao.Produto != "" &&
				regra.CodigoProdutoCorporativo != posicao.Produto {
				continue
			}
			for _, condicao := range regra.Condicoes {
				if !condicao.Ativo {
					continue
				}
				ok, err := s.evaluator.EvaluateCondition(condicao.Condicao, env)
				if err != nil {
					evaluator.LogEvalError(data, posicao.CodigoIdentificadorBoleto, condicao.Condicao, err)
					continue
				}
				if !ok {
					continue
				}
				valor, err := s.evaluator.EvaluateValue(condicao.CampoValor, env)
				if err != nil {
					evaluator.LogEvalError(data, posicao.CodigoIdentificadorBoleto, condicao.CampoValor, err)
					continue
				}
				moeda := ""
				if v, found := env[condicao.CampoMoeda]; found {
					if s, ok := v.(string); ok {
						moeda = s
					}
				}
				lancamentos = append(lancamentos, model.LancamentoContabil{
					DataLoteContabil:          data,
					CodigoIdentificadorBoleto: posicao.CodigoIdentificadorBoleto,
					ValorLancamentoContabil:   valor,
					MoedaLancamentoContabil:   moeda,
					ContaDebito:               condicao.ContaDebito,
					ContaCredito:              condicao.ContaCredito,
					IndicadorReversao:         false,
					DescricaoRegraContabil:    regra.Descricao,
					DescricaoCondicaoContabil: condicao.Condicao,
					IDRegraContabil:           regra.ID,
				})
			}
		}
	}

	// 4. Calcular próxima versão para D
	versao, err := s.movimentoRepo.ObterProximaVersao(ctx, data)
	if err != nil {
		return fmt.Errorf("erro ao obter próxima versão: %w", err)
	}
	for i := range lancamentos {
		lancamentos[i].CodigoVersaoConteudo = versao
	}

	// 5. Gerar estornos de D-1 em memória, considerando os lançamentos de D recém-gerados
	dMenos1 := data.AddDate(0, 0, -1)
	lancamentosD1, err := s.movimentoRepo.BuscarPorDataEIndicador(ctx, dMenos1, false)
	if err != nil {
		return fmt.Errorf("erro ao buscar lançamentos de D-1: %w", err)
	}

	var estornos []model.LancamentoContabil
	if len(lancamentosD1) > 0 {
		log.Printf("[movimento+estorno] gerando %d estornos de D-1 (%s) para D (%s)",
			len(lancamentosD1), dMenos1.Format("2006-01-02"), data.Format("2006-01-02"))

		for _, l1 := range lancamentosD1 {
			estornos = append(estornos, model.LancamentoContabil{
				DataLoteContabil:          data,
				CodigoVersaoConteudo:      versao, // mesma versão do movimento de D
				CodigoIdentificadorBoleto: l1.CodigoIdentificadorBoleto,
				ValorLancamentoContabil:   l1.ValorLancamentoContabil,
				MoedaLancamentoContabil:   l1.MoedaLancamentoContabil,
				ContaDebito:               l1.ContaCredito, // contas invertidas
				ContaCredito:              l1.ContaDebito,
				IndicadorReversao:         true,
				DescricaoRegraContabil:    l1.DescricaoRegraContabil,
				DescricaoCondicaoContabil: l1.DescricaoCondicaoContabil,
				IDRegraContabil:           l1.IDRegraContabil,
			})
		}
	} else {
		log.Printf("[movimento+estorno] sem lançamentos em D-1 (%s), estorno não gerado", dMenos1.Format("2006-01-02"))
	}

	// 6. Persistir movimento + estornos em um único BulkInsert
	todos := append(lancamentos, estornos...)
	if err := s.movimentoRepo.BulkInsert(ctx, todos); err != nil {
		return fmt.Errorf("erro ao persistir lançamentos: %w", err)
	}

	log.Printf("[movimento+estorno] persistidos %d lançamentos e %d estornos para %s versão %d",
		len(lancamentos), len(estornos), data.Format("2006-01-02"), versao)
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
