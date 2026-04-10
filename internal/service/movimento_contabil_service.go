package service

import (
	"context"
	"fmt"
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
// contábeis ativas e persiste os lançamentos resultantes em lote.
func (s *MovimentoContabilService) GerarMovimento(ctx context.Context, data time.Time) error {
	// 1. Buscar posição com versão máxima para a data
	posicoes, err := s.posicaoRepo.BuscarPorDataEVersaoMaxima(ctx, data)
	if err != nil {
		return fmt.Errorf("erro ao buscar posicao_carteira: %w", err)
	}

	// 2. Retornar erro de ausência de dados se posição vazia
	if len(posicoes) == 0 {
		return fmt.Errorf("nenhum registro de posicao_carteira encontrado para a data %s", data.Format("2006-01-02"))
	}

	// 3. Carregar todas as regras e condições ativas
	regras, err := s.regraRepo.ListarRegrasAtivas(ctx)
	if err != nil {
		return fmt.Errorf("erro ao carregar regras contábeis: %w", err)
	}

	// 4. Para cada posição × regra × condição: avaliar e montar lançamentos
	var lancamentos []model.LancamentoContabil

	for _, posicao := range posicoes {
		env := evaluator.PosicaoToEnv(posicao)

		for _, regra := range regras {
			for _, condicao := range regra.Condicoes {
				if !condicao.Ativo {
					continue
				}

				// Avaliar expressão booleana
				ok, err := s.evaluator.EvaluateCondition(condicao.Condicao, env)
				if err != nil {
					evaluator.LogEvalError(data, posicao.CodigoIdentificadorBoleto, condicao.Condicao, err)
					continue
				}
				if !ok {
					continue
				}

				// Avaliar expressão de valor
				valor, err := s.evaluator.EvaluateValue(condicao.CampoValor, env)
				if err != nil {
					evaluator.LogEvalError(data, posicao.CodigoIdentificadorBoleto, condicao.CampoValor, err)
					continue
				}

				// Obter moeda do env
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

	// 5. Calcular código_versao_conteudo como próxima versão
	versao, err := s.movimentoRepo.ObterProximaVersao(ctx, data)
	if err != nil {
		return fmt.Errorf("erro ao obter próxima versão: %w", err)
	}

	// 6. Definir versão em todos os lançamentos
	for i := range lancamentos {
		lancamentos[i].CodigoVersaoConteudo = versao
	}

	// 7. Bulk insert de todos os lançamentos
	if err := s.movimentoRepo.BulkInsert(ctx, lancamentos); err != nil {
		return fmt.Errorf("erro ao persistir lançamentos: %w", err)
	}

	return nil
}

// ConsultarLancamentos retorna os lançamentos paginados para a data informada.
func (s *MovimentoContabilService) ConsultarLancamentos(ctx context.Context, data time.Time, pagina, tamanho int) (*model.PaginaLancamentos, error) {
	return s.movimentoRepo.ConsultarPaginado(ctx, data, pagina, tamanho)
}

// ConsultarLancamentosFiltrado retorna lançamentos paginados por período, boleto e versão.
func (s *MovimentoContabilService) ConsultarLancamentosFiltrado(ctx context.Context, dataInicio, dataFim time.Time, boleto string, versao int, versaoModo string, pagina, tamanho int) (*model.PaginaLancamentos, error) {
	return s.movimentoRepo.ConsultarPaginadoFiltrado(ctx, dataInicio, dataFim, boleto, versao, versaoModo, pagina, tamanho)
}

// GerarEstorno compara os lançamentos de D-1 com os de D e gera estornos para
// lançamentos com valor divergente ou sem correspondente em D.
func (s *MovimentoContabilService) GerarEstorno(ctx context.Context, data time.Time) error {
	dMenos1 := data.AddDate(0, 0, -1)

	// 1. Buscar lançamentos de D-1 (indicador_reversao=false)
	lancamentosD1, err := s.movimentoRepo.BuscarPorDataEIndicador(ctx, dMenos1, false)
	if err != nil {
		return fmt.Errorf("erro ao buscar lançamentos de D-1: %w", err)
	}

	// 2. Se D-1 vazio, retornar erro de ausência
	if len(lancamentosD1) == 0 {
		return fmt.Errorf("nenhum lote contábil encontrado para D-1 (%s)", dMenos1.Format("2006-01-02"))
	}

	// 3. Buscar lançamentos de D (indicador_reversao=false)
	lancamentosD, err := s.movimentoRepo.BuscarPorDataEIndicador(ctx, data, false)
	if err != nil {
		return fmt.Errorf("erro ao buscar lançamentos de D: %w", err)
	}

	// 4. Construir mapa de lançamentos de D por chave (boleto, idRegra)
	type chave struct {
		boleto  string
		idRegra int64
	}
	mapaD := make(map[chave]model.LancamentoContabil, len(lancamentosD))
	for _, l := range lancamentosD {
		mapaD[chave{l.CodigoIdentificadorBoleto, l.IDRegraContabil}] = l
	}

	// 5. Para cada lançamento de D-1, verificar se estorno é necessário
	var estornos []model.LancamentoContabil
	for _, l1 := range lancamentosD1 {
		k := chave{l1.CodigoIdentificadorBoleto, l1.IDRegraContabil}
		lD, exists := mapaD[k]

		// Gerar estorno se: sem correspondente em D, ou valor divergente
		if !exists || lD.ValorLancamentoContabil != l1.ValorLancamentoContabil {
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
	}

	if len(estornos) == 0 {
		return nil
	}

	// 6. Obter versão atual para a data D (estorno usa a mesma versão do lote, não incrementa)
	versao, err := s.movimentoRepo.ObterVersaoAtual(ctx, data)
	if err != nil {
		return fmt.Errorf("erro ao obter versão atual para estorno: %w", err)
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
