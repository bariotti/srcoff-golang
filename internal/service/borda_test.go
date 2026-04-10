package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"srcoff/internal/evaluator"
	"srcoff/internal/model"
)

var testDate = time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)

// ---------------------------------------------------------------------------
// Test 1: Posição vazia → erro de ausência
// Valida: Requisito 1.3
// ---------------------------------------------------------------------------

func TestGerarMovimento_PosicaoVazia_RetornaErroAusencia(t *testing.T) {
	posRepo := &fakePosicaoRepo{registros: []model.PosicaoCarteira{}}
	regraRepo := &fakeRegraRepo{regras: []model.RegraContabil{}}
	movRepo := &fakeMovimentoRepo{}
	eval := evaluator.New()

	svc := NewMovimentoContabilService(posRepo, regraRepo, movRepo, eval)
	err := svc.GerarMovimento(context.Background(), testDate)

	if err == nil {
		t.Fatal("esperava erro de ausência, mas não houve erro")
	}
	if !strings.Contains(err.Error(), "nenhum registro de posicao_carteira") {
		t.Fatalf("mensagem de erro inesperada: %v", err)
	}
	if len(movRepo.lancamentos) != 0 {
		t.Fatalf("esperava 0 lançamentos, obteve %d", len(movRepo.lancamentos))
	}
}

// ---------------------------------------------------------------------------
// Test 2: Lote D-1 inexistente → erro de ausência
// Valida: Requisito 5.8
// ---------------------------------------------------------------------------

func TestGerarEstorno_LoteD1Inexistente_RetornaErroAusencia(t *testing.T) {
	movRepo := &fakeMovimentoRepoEstorno{
		lancamentosD1: []model.LancamentoContabil{},
		lancamentosD:  []model.LancamentoContabil{},
		dataD:         testDate,
	}
	eval := evaluator.New()

	svc := NewMovimentoContabilService(&fakePosicaoRepo{}, &fakeRegraRepo{}, movRepo, eval)
	err := svc.GerarEstorno(context.Background(), testDate)

	if err == nil {
		t.Fatal("esperava erro de ausência, mas não houve erro")
	}
	if !strings.Contains(err.Error(), "nenhum lote contábil encontrado para D-1") {
		t.Fatalf("mensagem de erro inesperada: %v", err)
	}
	if len(movRepo.inseridos) != 0 {
		t.Fatalf("esperava 0 estornos inseridos, obteve %d", len(movRepo.inseridos))
	}
}

// ---------------------------------------------------------------------------
// Test 3: Expressão booleana inválida → log de erro, outros registros processados
// Valida: Requisitos 2.4, 2.5
// ---------------------------------------------------------------------------

func TestGerarMovimento_ExpressaoBooleanaInvalida_ContinuaProcessamento(t *testing.T) {
	posicoes := []model.PosicaoCarteira{
		{
			ID:                        1,
			DataPosicaoCarteira:       testDate,
			CodigoVersaoConteudo:      1,
			CodigoIdentificadorBoleto: "BOLETO-001",
			ValorMTM:                  100.0,
			MoedaPrincipalRemanescente: "USD",
		},
		{
			ID:                        2,
			DataPosicaoCarteira:       testDate,
			CodigoVersaoConteudo:      1,
			CodigoIdentificadorBoleto: "BOLETO-002",
			ValorMTM:                  100.0,
			MoedaPrincipalRemanescente: "USD",
		},
	}

	regra := model.RegraContabil{
		ID:        1,
		Descricao: "Regra Borda",
		Ativo:     true,
		Condicoes: []model.CondicaoRegra{
			{
				ID:           1,
				Condicao:     "INVALID_EXPR_!!!",
				ContaDebito:  "1001",
				ContaCredito: "2001",
				CampoValor:   "valor_mtm",
				CampoMoeda:   "moeda_principal_remanescente",
				Ativo:        true,
			},
			{
				ID:           2,
				Condicao:     "valor_mtm > 0",
				ContaDebito:  "1002",
				ContaCredito: "2002",
				CampoValor:   "valor_mtm",
				CampoMoeda:   "moeda_principal_remanescente",
				Ativo:        true,
			},
		},
	}

	posRepo := &fakePosicaoRepo{registros: posicoes}
	regraRepo := &fakeRegraRepo{regras: []model.RegraContabil{regra}}
	movRepo := &fakeMovimentoRepo{}
	eval := evaluator.New()

	svc := NewMovimentoContabilService(posRepo, regraRepo, movRepo, eval)
	err := svc.GerarMovimento(context.Background(), testDate)

	if err != nil {
		t.Fatalf("não esperava erro, obteve: %v", err)
	}
	// Condição válida gera 1 lançamento por posição (2 posições)
	if len(movRepo.lancamentos) != 2 {
		t.Fatalf("esperava 2 lançamentos, obteve %d", len(movRepo.lancamentos))
	}
}

// ---------------------------------------------------------------------------
// Test 4: Expressão de valor inválida → log de erro, outros registros processados
// Valida: Requisitos 2.4, 2.5
// ---------------------------------------------------------------------------

func TestGerarMovimento_ExpressaoValorInvalida_ContinuaProcessamento(t *testing.T) {
	posicoes := []model.PosicaoCarteira{
		{
			ID:                        1,
			DataPosicaoCarteira:       testDate,
			CodigoVersaoConteudo:      1,
			CodigoIdentificadorBoleto: "BOLETO-001",
			ValorMTM:                  100.0,
			MoedaPrincipalRemanescente: "USD",
		},
		{
			ID:                        2,
			DataPosicaoCarteira:       testDate,
			CodigoVersaoConteudo:      1,
			CodigoIdentificadorBoleto: "BOLETO-002",
			ValorMTM:                  100.0,
			MoedaPrincipalRemanescente: "USD",
		},
	}

	regra := model.RegraContabil{
		ID:        1,
		Descricao: "Regra Borda",
		Ativo:     true,
		Condicoes: []model.CondicaoRegra{
			{
				ID:           1,
				Condicao:     "valor_mtm > 0",
				ContaDebito:  "1001",
				ContaCredito: "2001",
				CampoValor:   "CAMPO_INVALIDO_!!!",
				CampoMoeda:   "moeda_principal_remanescente",
				Ativo:        true,
			},
			{
				ID:           2,
				Condicao:     "valor_mtm > 0",
				ContaDebito:  "1002",
				ContaCredito: "2002",
				CampoValor:   "valor_mtm",
				CampoMoeda:   "moeda_principal_remanescente",
				Ativo:        true,
			},
		},
	}

	posRepo := &fakePosicaoRepo{registros: posicoes}
	regraRepo := &fakeRegraRepo{regras: []model.RegraContabil{regra}}
	movRepo := &fakeMovimentoRepo{}
	eval := evaluator.New()

	svc := NewMovimentoContabilService(posRepo, regraRepo, movRepo, eval)
	err := svc.GerarMovimento(context.Background(), testDate)

	if err != nil {
		t.Fatalf("não esperava erro, obteve: %v", err)
	}
	// Condição com campo_valor válido gera 1 lançamento por posição (2 posições)
	if len(movRepo.lancamentos) != 2 {
		t.Fatalf("esperava 2 lançamentos, obteve %d", len(movRepo.lancamentos))
	}
}
