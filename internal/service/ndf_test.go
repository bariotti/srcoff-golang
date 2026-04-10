package service

import (
	"context"
	"testing"
	"time"

	"srcoff/internal/evaluator"
	"srcoff/internal/model"
)

// ndfRules retorna as 4 CondicaoRegra do produto NDF (Nassau) agrupadas em uma RegraContabil.
func ndfRules() model.RegraContabil {
	return model.RegraContabil{
		ID:        10,
		Descricao: "NDF Nassau",
		Ativo:     true,
		Condicoes: []model.CondicaoRegra{
			// Regra 1: Nassau + afiliada + MTM > 0
			{
				ID:           1,
				Condicao:     `descricao_veiculo == "NASSAU" && indicador_contraparte_afiliada == true && valor_mtm > 0`,
				ContaDebito:  "111111111",
				ContaCredito: "222222222",
				CampoValor:   "principal_remanescente + valor_mtm",
				CampoMoeda:   "moeda_principal_remanescente",
				Ativo:        true,
			},
			// Regra 2: Nassau + afiliada + MTM < 0
			{
				ID:           2,
				Condicao:     `descricao_veiculo == "NASSAU" && indicador_contraparte_afiliada == true && valor_mtm < 0`,
				ContaDebito:  "333333333",
				ContaCredito: "444444444",
				CampoValor:   "principal_remanescente",
				CampoMoeda:   "moeda_principal_remanescente",
				Ativo:        true,
			},
			// Regra 3: Nassau + não-afiliada + MTM > 0
			{
				ID:           3,
				Condicao:     `descricao_veiculo == "NASSAU" && indicador_contraparte_afiliada == false && valor_mtm > 0`,
				ContaDebito:  "555555555",
				ContaCredito: "666666666",
				CampoValor:   "principal_remanescente + valor_mtm",
				CampoMoeda:   "moeda_principal_remanescente",
				Ativo:        true,
			},
			// Regra 4: Nassau + não-afiliada + MTM < 0
			{
				ID:           4,
				Condicao:     `descricao_veiculo == "NASSAU" && indicador_contraparte_afiliada == false && valor_mtm < 0`,
				ContaDebito:  "777777777",
				ContaCredito: "888888888",
				CampoValor:   "principal_remanescente",
				CampoMoeda:   "moeda_principal_remanescente",
				Ativo:        true,
			},
		},
	}
}

var ndfBaseDate = time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)

// TestNDF_Nassau_Afiliada_MTMPositivo valida o Requisito 4.1:
// Nassau + afiliada + MTM > 0 → conta_debito="111111111", conta_credito="222222222",
// valor=principal_remanescente+valor_mtm, moeda=moeda_principal_remanescente
func TestNDF_Nassau_Afiliada_MTMPositivo(t *testing.T) {
	posicao := model.PosicaoCarteira{
		ID:                           1,
		DataPosicaoCarteira:          ndfBaseDate,
		CodigoVersaoConteudo:         1,
		CodigoIdentificadorBoleto:    "BOLETO-NDF-001",
		DescricaoVeiculo:             "NASSAU",
		IndicadorContraparteAfiliada: true,
		ValorMTM:                     500.0,
		PrincipalRemanescente:        1000.0,
		MoedaPrincipalRemanescente:   "USD",
	}

	posRepo := &fakePosicaoRepo{registros: []model.PosicaoCarteira{posicao}}
	regraRepo := &fakeRegraRepo{regras: []model.RegraContabil{ndfRules()}}
	movRepo := &fakeMovimentoRepo{}

	svc := NewMovimentoContabilService(posRepo, regraRepo, movRepo, evaluator.New())
	err := svc.GerarMovimento(context.Background(), ndfBaseDate)
	if err != nil {
		t.Fatalf("GerarMovimento retornou erro inesperado: %v", err)
	}

	if len(movRepo.lancamentos) != 1 {
		t.Fatalf("esperado 1 lançamento, obtido %d", len(movRepo.lancamentos))
	}

	l := movRepo.lancamentos[0]
	if l.ContaDebito != "111111111" {
		t.Errorf("ContaDebito: esperado %q, obtido %q", "111111111", l.ContaDebito)
	}
	if l.ContaCredito != "222222222" {
		t.Errorf("ContaCredito: esperado %q, obtido %q", "222222222", l.ContaCredito)
	}
	expectedValor := posicao.PrincipalRemanescente + posicao.ValorMTM // 1500.0
	if l.ValorLancamentoContabil != expectedValor {
		t.Errorf("ValorLancamentoContabil: esperado %v, obtido %v", expectedValor, l.ValorLancamentoContabil)
	}
	if l.MoedaLancamentoContabil != "USD" {
		t.Errorf("MoedaLancamentoContabil: esperado %q, obtido %q", "USD", l.MoedaLancamentoContabil)
	}
}

// TestNDF_Nassau_Afiliada_MTMNegativo valida o Requisito 4.2:
// Nassau + afiliada + MTM < 0 → conta_debito="333333333", conta_credito="444444444",
// valor=principal_remanescente, moeda=moeda_principal_remanescente
func TestNDF_Nassau_Afiliada_MTMNegativo(t *testing.T) {
	posicao := model.PosicaoCarteira{
		ID:                           2,
		DataPosicaoCarteira:          ndfBaseDate,
		CodigoVersaoConteudo:         1,
		CodigoIdentificadorBoleto:    "BOLETO-NDF-001",
		DescricaoVeiculo:             "NASSAU",
		IndicadorContraparteAfiliada: true,
		ValorMTM:                     -300.0,
		PrincipalRemanescente:        1000.0,
		MoedaPrincipalRemanescente:   "USD",
	}

	posRepo := &fakePosicaoRepo{registros: []model.PosicaoCarteira{posicao}}
	regraRepo := &fakeRegraRepo{regras: []model.RegraContabil{ndfRules()}}
	movRepo := &fakeMovimentoRepo{}

	svc := NewMovimentoContabilService(posRepo, regraRepo, movRepo, evaluator.New())
	err := svc.GerarMovimento(context.Background(), ndfBaseDate)
	if err != nil {
		t.Fatalf("GerarMovimento retornou erro inesperado: %v", err)
	}

	if len(movRepo.lancamentos) != 1 {
		t.Fatalf("esperado 1 lançamento, obtido %d", len(movRepo.lancamentos))
	}

	l := movRepo.lancamentos[0]
	if l.ContaDebito != "333333333" {
		t.Errorf("ContaDebito: esperado %q, obtido %q", "333333333", l.ContaDebito)
	}
	if l.ContaCredito != "444444444" {
		t.Errorf("ContaCredito: esperado %q, obtido %q", "444444444", l.ContaCredito)
	}
	expectedValor := posicao.PrincipalRemanescente // 1000.0
	if l.ValorLancamentoContabil != expectedValor {
		t.Errorf("ValorLancamentoContabil: esperado %v, obtido %v", expectedValor, l.ValorLancamentoContabil)
	}
	if l.MoedaLancamentoContabil != "USD" {
		t.Errorf("MoedaLancamentoContabil: esperado %q, obtido %q", "USD", l.MoedaLancamentoContabil)
	}
}

// TestNDF_Nassau_NaoAfiliada_MTMPositivo valida o Requisito 4.3:
// Nassau + não-afiliada + MTM > 0 → conta_debito="555555555", conta_credito="666666666",
// valor=principal_remanescente+valor_mtm, moeda=moeda_principal_remanescente
func TestNDF_Nassau_NaoAfiliada_MTMPositivo(t *testing.T) {
	posicao := model.PosicaoCarteira{
		ID:                           3,
		DataPosicaoCarteira:          ndfBaseDate,
		CodigoVersaoConteudo:         1,
		CodigoIdentificadorBoleto:    "BOLETO-NDF-001",
		DescricaoVeiculo:             "NASSAU",
		IndicadorContraparteAfiliada: false,
		ValorMTM:                     500.0,
		PrincipalRemanescente:        1000.0,
		MoedaPrincipalRemanescente:   "USD",
	}

	posRepo := &fakePosicaoRepo{registros: []model.PosicaoCarteira{posicao}}
	regraRepo := &fakeRegraRepo{regras: []model.RegraContabil{ndfRules()}}
	movRepo := &fakeMovimentoRepo{}

	svc := NewMovimentoContabilService(posRepo, regraRepo, movRepo, evaluator.New())
	err := svc.GerarMovimento(context.Background(), ndfBaseDate)
	if err != nil {
		t.Fatalf("GerarMovimento retornou erro inesperado: %v", err)
	}

	if len(movRepo.lancamentos) != 1 {
		t.Fatalf("esperado 1 lançamento, obtido %d", len(movRepo.lancamentos))
	}

	l := movRepo.lancamentos[0]
	if l.ContaDebito != "555555555" {
		t.Errorf("ContaDebito: esperado %q, obtido %q", "555555555", l.ContaDebito)
	}
	if l.ContaCredito != "666666666" {
		t.Errorf("ContaCredito: esperado %q, obtido %q", "666666666", l.ContaCredito)
	}
	expectedValor := posicao.PrincipalRemanescente + posicao.ValorMTM // 1500.0
	if l.ValorLancamentoContabil != expectedValor {
		t.Errorf("ValorLancamentoContabil: esperado %v, obtido %v", expectedValor, l.ValorLancamentoContabil)
	}
	if l.MoedaLancamentoContabil != "USD" {
		t.Errorf("MoedaLancamentoContabil: esperado %q, obtido %q", "USD", l.MoedaLancamentoContabil)
	}
}

// TestNDF_Nassau_NaoAfiliada_MTMNegativo valida o Requisito 4.4:
// Nassau + não-afiliada + MTM < 0 → conta_debito="777777777", conta_credito="888888888",
// valor=principal_remanescente, moeda=moeda_principal_remanescente
func TestNDF_Nassau_NaoAfiliada_MTMNegativo(t *testing.T) {
	posicao := model.PosicaoCarteira{
		ID:                           4,
		DataPosicaoCarteira:          ndfBaseDate,
		CodigoVersaoConteudo:         1,
		CodigoIdentificadorBoleto:    "BOLETO-NDF-001",
		DescricaoVeiculo:             "NASSAU",
		IndicadorContraparteAfiliada: false,
		ValorMTM:                     -300.0,
		PrincipalRemanescente:        1000.0,
		MoedaPrincipalRemanescente:   "USD",
	}

	posRepo := &fakePosicaoRepo{registros: []model.PosicaoCarteira{posicao}}
	regraRepo := &fakeRegraRepo{regras: []model.RegraContabil{ndfRules()}}
	movRepo := &fakeMovimentoRepo{}

	svc := NewMovimentoContabilService(posRepo, regraRepo, movRepo, evaluator.New())
	err := svc.GerarMovimento(context.Background(), ndfBaseDate)
	if err != nil {
		t.Fatalf("GerarMovimento retornou erro inesperado: %v", err)
	}

	if len(movRepo.lancamentos) != 1 {
		t.Fatalf("esperado 1 lançamento, obtido %d", len(movRepo.lancamentos))
	}

	l := movRepo.lancamentos[0]
	if l.ContaDebito != "777777777" {
		t.Errorf("ContaDebito: esperado %q, obtido %q", "777777777", l.ContaDebito)
	}
	if l.ContaCredito != "888888888" {
		t.Errorf("ContaCredito: esperado %q, obtido %q", "888888888", l.ContaCredito)
	}
	expectedValor := posicao.PrincipalRemanescente // 1000.0
	if l.ValorLancamentoContabil != expectedValor {
		t.Errorf("ValorLancamentoContabil: esperado %v, obtido %v", expectedValor, l.ValorLancamentoContabil)
	}
	if l.MoedaLancamentoContabil != "USD" {
		t.Errorf("MoedaLancamentoContabil: esperado %q, obtido %q", "USD", l.MoedaLancamentoContabil)
	}
}

// TestNDF_SemCondicaoSatisfeita_NaoGeraLancamento valida o Requisito 4.5:
// Quando nenhuma condição é satisfeita, nenhum lançamento deve ser gerado.
func TestNDF_SemCondicaoSatisfeita_NaoGeraLancamento(t *testing.T) {
	// DescricaoVeiculo "OUTRO" não satisfaz nenhuma das 4 regras NDF
	posicao := model.PosicaoCarteira{
		ID:                           5,
		DataPosicaoCarteira:          ndfBaseDate,
		CodigoVersaoConteudo:         1,
		CodigoIdentificadorBoleto:    "BOLETO-NDF-001",
		DescricaoVeiculo:             "OUTRO",
		IndicadorContraparteAfiliada: true,
		ValorMTM:                     500.0,
		PrincipalRemanescente:        1000.0,
		MoedaPrincipalRemanescente:   "USD",
	}

	posRepo := &fakePosicaoRepo{registros: []model.PosicaoCarteira{posicao}}
	regraRepo := &fakeRegraRepo{regras: []model.RegraContabil{ndfRules()}}
	movRepo := &fakeMovimentoRepo{}

	svc := NewMovimentoContabilService(posRepo, regraRepo, movRepo, evaluator.New())
	err := svc.GerarMovimento(context.Background(), ndfBaseDate)
	if err != nil {
		t.Fatalf("GerarMovimento retornou erro inesperado: %v", err)
	}

	if len(movRepo.lancamentos) != 0 {
		t.Errorf("esperado 0 lançamentos, obtido %d", len(movRepo.lancamentos))
	}
}
