package service

import (
	"context"
	"testing"
	"time"

	"srcoff/internal/evaluator"
	"srcoff/internal/model"
	filerepo "srcoff/internal/repository/file"
)

// regraIncrementalReverte: regra do tipo incremental_reverte (cálculo |D0 - D-N| e,
// quando o boleto some da posição, baixa o saldo líquido postado — RN-121).
func regraIncrementalReverte() model.RegraContabil {
	return model.RegraContabil{
		ID: 1, Descricao: "IncRev", Ativo: true, TipoLancamento: model.TipoIncrementalReverte,
		Condicoes: []model.CondicaoRegra{{
			ID: 1, IDRegra: 1, Condicao: "valor_mtm > 0",
			ContaDebito: "1001", ContaCredito: "2001",
			CampoValor: "valor_mtm", CampoMoeda: "moeda_principal_remanescente",
			CampoBoleto: "codigo_identificador_boleto", Ativo: true,
		}},
	}
}

// TestIncrementalReverte_BaixaPeloSaldoLiquido valida que, quando o boleto some da posição,
// o tipo incremental_reverte estorna o SALDO LÍQUIDO POSTADO (soma dos lançamentos), e não o
// valor da posição D-N — provado pelo caso de queda (1000→1300→900 posta 1000+300+400=1700,
// enquanto a posição D-N vale 900). B2 permanece na posição para manter o combo com posições.
func TestIncrementalReverte_BaixaPeloSaldoLiquido(t *testing.T) {
	ctx := context.Background()
	eval := evaluator.New()
	dir := t.TempDir()

	d08 := d("2024-01-08")
	d09 := d("2024-01-09")
	d10 := d("2024-01-10")
	d11 := d("2024-01-11")

	posRepo := &fakePosicaoRepo{registros: []model.PosicaoCarteira{
		// B1 (vai sumir em d11): 1000 -> 1300 -> 900
		posD1(1, d08, "NDF", "Posição", "B1", 1000),
		posD1(2, d09, "NDF", "Posição", "B1", 1300),
		posD1(3, d10, "NDF", "Posição", "B1", 900),
		// B2 presente todos os dias (mantém o combo com posições em d11)
		posD1(4, d08, "NDF", "Posição", "B2", 500),
		posD1(5, d09, "NDF", "Posição", "B2", 500),
		posD1(6, d10, "NDF", "Posição", "B2", 500),
		posD1(7, d11, "NDF", "Posição", "B2", 500),
	}}
	regraRepo := &fakeRegraRepo{regras: []model.RegraContabil{regraIncrementalReverte()}}
	movRepo := filerepo.NewMovimentoContabilRepo(dir)
	execRepo := filerepo.NewExecucaoRepo(dir)
	padrao := &fakePadraoLookup{padroes: []model.PadraoArquivo{
		{Produto: "NDF", Dominio: "Posição", ObrigatorioMovD1: boolPtr(false)},
	}}
	svc := NewMovimentoContabilService(posRepo, regraRepo, movRepo, eval).
		ComExecucaoRepo(execRepo).ComPadraoRepo(padrao)

	// Valor do lançamento (não-estorno) de um boleto numa data.
	valorLanc := func(data time.Time, boleto string) (int, float64) {
		ls, _ := movRepo.BuscarPorDataEIndicador(ctx, data, false)
		n, soma := 0, 0.0
		for _, l := range ls {
			if l.CodigoIdentificadorBoleto == boleto {
				n++
				soma += l.ValorLancamentoContabil
			}
		}
		return n, soma
	}
	// Valor do estorno (baixa) de um boleto numa data.
	valorEstorno := func(data time.Time, boleto string) (int, float64) {
		ls, _ := movRepo.BuscarPorDataEIndicador(ctx, data, true)
		n, soma := 0, 0.0
		for _, l := range ls {
			if l.CodigoIdentificadorBoleto == boleto {
				n++
				soma += l.ValorLancamentoContabil
			}
		}
		return n, soma
	}

	for _, dt := range []time.Time{d08, d09, d10, d11} {
		if _, err := svc.GerarMovimentoEscopo(ctx, dt, "NDF", "Posição"); err != nil {
			t.Fatalf("processar %s: %v", dt.Format("2006-01-02"), err)
		}
	}

	// B1: lançamentos incrementais 1000 -> 300 -> 400, sem estorno até d10.
	if n, v := valorLanc(d08, "B1"); n != 1 || v != 1000 {
		t.Fatalf("B1 08/01 esperado 1000, obteve n=%d v=%v", n, v)
	}
	if n, v := valorLanc(d09, "B1"); n != 1 || v != 300 {
		t.Fatalf("B1 09/01 esperado 300, obteve n=%d v=%v", n, v)
	}
	if n, v := valorLanc(d10, "B1"); n != 1 || v != 400 {
		t.Fatalf("B1 10/01 esperado 400, obteve n=%d v=%v", n, v)
	}
	for _, dt := range []time.Time{d08, d09, d10} {
		if n, _ := valorEstorno(dt, "B1"); n != 0 {
			t.Fatalf("B1 %s não deveria ter estorno ainda", dt.Format("2006-01-02"))
		}
	}

	// d11: B1 sumiu da posição → baixa pelo SALDO LÍQUIDO (1000+300+400 = 1700), sem lançamento normal.
	if n, _ := valorLanc(d11, "B1"); n != 0 {
		t.Fatalf("B1 11/01 não deveria ter lançamento normal (sumiu da posição)")
	}
	if n, v := valorEstorno(d11, "B1"); n != 1 || v != 1700 {
		t.Fatalf("B1 11/01 esperava baixa de 1700 (saldo líquido), obteve n=%d v=%v", n, v)
	}

	// Idempotência (RN-121.4): reprocessar d11 não gera nova baixa (saldo já zerado).
	if _, err := svc.GerarMovimentoEscopo(ctx, d11, "NDF", "Posição"); err != nil {
		t.Fatalf("reprocessar 11/01: %v", err)
	}
	if n, v := valorEstorno(d11, "B1"); n != 1 || v != 1700 {
		t.Fatalf("B1 11/01 após reprocesso deveria manter 1 baixa de 1700, obteve n=%d v=%v", n, v)
	}
}
