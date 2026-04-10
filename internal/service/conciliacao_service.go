package service

import (
	"context"
	"fmt"
	"time"

	"srcoff/internal/model"
)

// ConciliacaoService executa a conciliação entre posição de carteira e movimento contábil.
type ConciliacaoService struct {
	posicaoRepo   posicaoCarteiraRepo
	movimentoRepo movimentoContabilRepo
}

func NewConciliacaoService(posicaoRepo posicaoCarteiraRepo, movimentoRepo movimentoContabilRepo) *ConciliacaoService {
	return &ConciliacaoService{posicaoRepo: posicaoRepo, movimentoRepo: movimentoRepo}
}

func (s *ConciliacaoService) Conciliar(ctx context.Context, data time.Time) (*model.ResultadoConciliacao, error) {
	dataStr := data.Format("2006-01-02")

	// Buscar posição (versão máxima)
	posicoes, err := s.posicaoRepo.BuscarPorDataEVersaoMaxima(ctx, data)
	if err != nil {
		return nil, fmt.Errorf("erro ao buscar posição: %w", err)
	}

	// Buscar todos os movimentos da data (versão vigente — maior versão por data)
	inicio := data
	fim := data
	movimentos, err := s.movimentoRepo.ConsultarPaginadoFiltrado(ctx, inicio, fim, "", 0, "vigente", 1, 999999)
	if err != nil {
		return nil, fmt.Errorf("erro ao buscar movimentos: %w", err)
	}

	resultado := &model.ResultadoConciliacao{
		Data:            dataStr,
		TotalPosicoes:   len(posicoes),
		TotalMovimentos: movimentos.Total,
	}

	// Índice de boletos presentes no movimento
	boletosNoMovimento := make(map[string]bool, len(movimentos.Lancamentos))
	for _, m := range movimentos.Lancamentos {
		boletosNoMovimento[m.CodigoIdentificadorBoleto] = true
	}

	// Validação 1: posição sem movimento
	for _, p := range posicoes {
		if !boletosNoMovimento[p.CodigoIdentificadorBoleto] {
			resultado.Inconsistencias = append(resultado.Inconsistencias, model.Inconsistencia{
				Tipo:                      model.InconsistenciaSemMovimento,
				CodigoIdentificadorBoleto: p.CodigoIdentificadorBoleto,
				Detalhe:                   fmt.Sprintf("Boleto presente na posição de %s não possui lançamento contábil", dataStr),
			})
		}
	}

	// Validação 2: duplicidade (mesmo boleto + regra + indicador_reversao)
	type chaveDup struct {
		boleto    string
		regra     string
		reversao  bool
	}
	contagem := make(map[chaveDup]int)
	regrasPorChave := make(map[chaveDup]string)
	for _, m := range movimentos.Lancamentos {
		k := chaveDup{m.CodigoIdentificadorBoleto, m.DescricaoRegraContabil, m.IndicadorReversao}
		contagem[k]++
		regrasPorChave[k] = m.DescricaoRegraContabil
	}
	for k, qtd := range contagem {
		if qtd > 1 {
			rev := "Não"
			if k.reversao {
				rev = "Sim"
			}
			resultado.Inconsistencias = append(resultado.Inconsistencias, model.Inconsistencia{
				Tipo:                      model.InconsistenciaDuplicidade,
				CodigoIdentificadorBoleto: k.boleto,
				DescricaoRegra:            k.regra,
				IndicadorReversao:         k.reversao,
				Detalhe:                   fmt.Sprintf("%d lançamentos para boleto=%s, regra=%s, reversão=%s", qtd, k.boleto, k.regra, rev),
			})
		}
	}

	return resultado, nil
}
