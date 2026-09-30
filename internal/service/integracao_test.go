package service

import (
	"context"
	"testing"
	"time"

	"srcoff/internal/evaluator"
	"srcoff/internal/model"
)

// posIntegra cria uma posição dinâmica para os testes de integração.
func posIntegra(id int64, data time.Time, boleto string, mtm, principal float64, moeda string) model.PosicaoCarteira {
	return model.PosicaoCarteira{
		ID:                   id,
		DataPosicaoCarteira:  data,
		CodigoVersaoConteudo: 1,
		Campos: map[string]interface{}{
			"codigo_identificador_boleto":    boleto,
			"descricao_veiculo":              "NASSAU",
			"indicador_contraparte_afiliada": true,
			"valor_mtm":                      mtm,
			"principal_remanescente":         principal,
			"moeda_principal_remanescente":   moeda,
		},
	}
}

// ---------------------------------------------------------------------------
// Test 1: Fluxo completo — inserir posição → gerar movimento → gerar estorno → consultar lote consolidado
// ---------------------------------------------------------------------------

// Valida: Requisitos 6.1, 6.2
func TestFluxoCompleto_MovimentoEstornoConsulta(t *testing.T) {
	ctx := context.Background()
	eval := evaluator.New()

	// D-1 é o dia útil anterior a baseDate (segunda 2024-01-15 → sexta 2024-01-12).
	dataDMenos1 := DiaUtilAnterior(baseDate)

	// Posição para D-1
	posicaoD1 := posIntegra(1, dataDMenos1, "BOLETO-D1", 200.0, 1000.0, "USD")
	// Posição para D (mesmo boleto, valor MTM diferente para gerar estorno)
	posicaoD := posIntegra(2, baseDate, "BOLETO-D1", 500.0, 1000.0, "USD")

	posRepo := &fakePosicaoRepo{registros: []model.PosicaoCarteira{posicaoD1, posicaoD}}

	regra := model.RegraContabil{
		ID:        1,
		Descricao: "Regra Integração",
		Ativo:     true,
		Condicoes: []model.CondicaoRegra{
			{
				ID:           1,
				IDRegra:      1,
				Condicao:     "valor_mtm > 0",
				ContaDebito:  "1001",
				ContaCredito: "2001",
				CampoValor:   "valor_mtm",
				CampoMoeda:   "moeda_principal_remanescente",
				CampoBoleto:  "codigo_identificador_boleto",
				Ativo:        true,
			},
		},
	}
	regraRepo := &fakeRegraRepo{regras: []model.RegraContabil{regra}}

	movRepo := &fakeMovimentoRepoPaginado{}

	svc := NewMovimentoContabilService(posRepo, regraRepo, movRepo, eval)

	// Passo 1: Gerar movimento para D-1
	if err := svc.GerarMovimento(ctx, dataDMenos1); err != nil {
		t.Fatalf("GerarMovimento D-1 falhou: %v", err)
	}

	// Passo 2: Gerar movimento para D
	if err := svc.GerarMovimento(ctx, baseDate); err != nil {
		t.Fatalf("GerarMovimento D falhou: %v", err)
	}

	// Verificar que há lançamentos para D-1 e D
	lancD1, _ := movRepo.BuscarPorDataEIndicador(ctx, dataDMenos1, false)
	if len(lancD1) == 0 {
		t.Fatal("esperava lançamentos para D-1, mas não encontrou nenhum")
	}

	lancD, _ := movRepo.BuscarPorDataEIndicador(ctx, baseDate, false)
	if len(lancD) == 0 {
		t.Fatal("esperava lançamentos para D, mas não encontrou nenhum")
	}

	// Passo 3: Gerar estorno para D
	// D-1 tem ValorMTM=200.0 e D tem ValorMTM=500.0 → valores divergem → estorno gerado
	if err := svc.GerarEstorno(ctx, baseDate); err != nil {
		t.Fatalf("GerarEstorno falhou: %v", err)
	}

	// Verificar que estornos foram gerados para D
	estornos, _ := movRepo.BuscarPorDataEIndicador(ctx, baseDate, true)
	if len(estornos) == 0 {
		t.Fatal("esperava estornos para D, mas não encontrou nenhum")
	}

	// Verificar invariantes dos estornos
	for _, e := range estornos {
		if !e.IndicadorReversao {
			t.Errorf("estorno deve ter IndicadorReversao=true, mas tem false")
		}
		if !e.DataLoteContabil.Equal(baseDate) {
			t.Errorf("estorno deve ter DataLoteContabil=%v, mas tem %v", baseDate, e.DataLoteContabil)
		}
		// Contas devem estar invertidas em relação ao lançamento original de D-1
		for _, orig := range lancD1 {
			if orig.CodigoIdentificadorBoleto == e.CodigoIdentificadorBoleto && orig.IDRegraContabil == e.IDRegraContabil {
				if e.ContaDebito != orig.ContaCredito {
					t.Errorf("ContaDebito do estorno deve ser ContaCredito do original: got %q, want %q", e.ContaDebito, orig.ContaCredito)
				}
				if e.ContaCredito != orig.ContaDebito {
					t.Errorf("ContaCredito do estorno deve ser ContaDebito do original: got %q, want %q", e.ContaCredito, orig.ContaDebito)
				}
			}
		}
	}

	// Passo 4: Consultar lote consolidado para D (movimento + estorno)
	result, err := svc.ConsultarLancamentos(ctx, baseDate, 1, 1000)
	if err != nil {
		t.Fatalf("ConsultarLancamentos falhou: %v", err)
	}

	expectedTotal := len(lancD) + len(estornos)
	if result.Total != expectedTotal {
		t.Errorf("Total esperado=%d, obtido=%d", expectedTotal, result.Total)
	}

	// Todos os lançamentos retornados devem ter DataLoteContabil == baseDate
	for _, l := range result.Lancamentos {
		if !l.DataLoteContabil.Equal(baseDate) {
			t.Errorf("lançamento com DataLoteContabil=%v, esperava %v", l.DataLoteContabil, baseDate)
		}
	}

	// Deve haver pelo menos um estorno com IndicadorReversao=true
	temEstorno := false
	for _, l := range result.Lancamentos {
		if l.IndicadorReversao {
			temEstorno = true
			break
		}
	}
	if !temEstorno {
		t.Error("lote consolidado deve conter pelo menos um estorno (IndicadorReversao=true)")
	}
}

// ---------------------------------------------------------------------------
// Test 2: Nova regra aplicada no próximo processamento sem redeploy
// ---------------------------------------------------------------------------

// Valida: Requisito 2.3
func TestNovaRegra_AplicadaNoProximoProcessamento(t *testing.T) {
	ctx := context.Background()
	eval := evaluator.New()

	posicao := posIntegra(1, baseDate, "BOLETO-001", 500.0, 1000.0, "USD")

	posRepo := &fakePosicaoRepo{registros: []model.PosicaoCarteira{posicao}}

	regraRepo := &fakeRegraRepo{
		regras: []model.RegraContabil{
			{
				ID:        1,
				Descricao: "Regra 1",
				Ativo:     true,
				Condicoes: []model.CondicaoRegra{
					{
						ID:           1,
						IDRegra:      1,
						Condicao:     "valor_mtm > 0",
						ContaDebito:  "1001",
						ContaCredito: "2001",
						CampoValor:   "valor_mtm",
						CampoMoeda:   "moeda_principal_remanescente",
						CampoBoleto:  "codigo_identificador_boleto",
						Ativo:        true,
					},
				},
			},
		},
	}

	// Passo 1: Processar com 1 regra
	movRepo1 := &fakeMovimentoRepo{}
	svc1 := NewMovimentoContabilService(posRepo, regraRepo, movRepo1, eval)

	if err := svc1.GerarMovimento(ctx, baseDate); err != nil {
		t.Fatalf("GerarMovimento com 1 regra falhou: %v", err)
	}

	if len(movRepo1.lancamentos) != 1 {
		t.Fatalf("esperava 1 lançamento com 1 regra, obteve %d", len(movRepo1.lancamentos))
	}

	// Passo 2: Adicionar segunda regra ao repositório (simula cadastro sem redeploy)
	regraRepo.regras = append(regraRepo.regras, model.RegraContabil{
		ID:        2,
		Descricao: "Regra 2",
		Ativo:     true,
		Condicoes: []model.CondicaoRegra{
			{
				ID:           2,
				IDRegra:      2,
				Condicao:     "principal_remanescente > 0",
				ContaDebito:  "3001",
				ContaCredito: "4001",
				CampoValor:   "principal_remanescente",
				CampoMoeda:   "moeda_principal_remanescente",
				CampoBoleto:  "codigo_identificador_boleto",
				Ativo:        true,
			},
		},
	})

	// Passo 3: Processar novamente com 2 regras (mesmo serviço, sem redeploy)
	movRepo2 := &fakeMovimentoRepo{}
	svc2 := NewMovimentoContabilService(posRepo, regraRepo, movRepo2, eval)

	if err := svc2.GerarMovimento(ctx, baseDate); err != nil {
		t.Fatalf("GerarMovimento com 2 regras falhou: %v", err)
	}

	// Deve gerar 2 lançamentos — um por regra — sem necessidade de redeploy
	if len(movRepo2.lancamentos) != 2 {
		t.Fatalf("esperava 2 lançamentos com 2 regras, obteve %d", len(movRepo2.lancamentos))
	}
}
