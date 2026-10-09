package service

import (
	"context"
	"testing"
	"time"

	"srcoff/internal/evaluator"
	"srcoff/internal/model"
	filerepo "srcoff/internal/repository/file"
)

// regraIncremental: regra do tipo incremental (não estorna; valor = | |D0| - |D-N| |).
func regraIncremental() model.RegraContabil {
	return model.RegraContabil{
		ID: 1, Descricao: "Inc", Ativo: true, TipoLancamento: model.TipoIncremental,
		Condicoes: []model.CondicaoRegra{{
			ID: 1, IDRegra: 1, Condicao: "valor_mtm > 0",
			ContaDebito: "1001", ContaCredito: "2001",
			CampoValor: "valor_mtm", CampoMoeda: "moeda_principal_remanescente",
			CampoBoleto: "codigo_identificador_boleto", Ativo: true,
		}},
	}
}

// TestIncremental_DiferencaD0MenosDN valida o lançamento incremental = | |valor_D0| - |valor_D-N| |,
// sem estorno, inclusive na queda (valor absoluto). D-N vem da posição do último dia com movimento.
func TestIncremental_DiferencaD0MenosDN(t *testing.T) {
	ctx := context.Background()
	eval := evaluator.New()
	dir := t.TempDir()

	d08 := d("2024-01-08")
	d09 := d("2024-01-09")
	d10 := d("2024-01-10")

	posRepo := &fakePosicaoRepo{registros: []model.PosicaoCarteira{
		posD1(1, d08, "NDF", "Posição", "B1", 1000),
		posD1(2, d09, "NDF", "Posição", "B1", 1300),
		posD1(3, d10, "NDF", "Posição", "B1", 900),
	}}
	regraRepo := &fakeRegraRepo{regras: []model.RegraContabil{regraIncremental()}}
	movRepo := filerepo.NewMovimentoContabilRepo(dir)
	execRepo := filerepo.NewExecucaoRepo(dir)
	padrao := &fakePadraoLookup{padroes: []model.PadraoArquivo{
		{Produto: "NDF", Dominio: "Posição", ObrigatorioMovD1: boolPtr(false)},
	}}
	svc := NewMovimentoContabilService(posRepo, regraRepo, movRepo, eval).
		ComExecucaoRepo(execRepo).ComPadraoRepo(padrao)

	valorMov := func(data time.Time) (int, float64) {
		ls, _ := movRepo.BuscarPorDataEIndicador(ctx, data, false)
		if len(ls) == 0 {
			return 0, 0
		}
		return len(ls), ls[0].ValorLancamentoContabil
	}
	semEstorno := func(data time.Time) bool {
		est, _ := movRepo.BuscarPorDataEIndicador(ctx, data, true)
		return len(est) == 0
	}

	// D1 (08/01): primeira ocorrência → valor cheio (1000), sem estorno.
	if _, err := svc.GerarMovimentoEscopo(ctx, d08, "NDF", "Posição"); err != nil {
		t.Fatalf("08/01: %v", err)
	}
	if n, v := valorMov(d08); n != 1 || v != 1000 {
		t.Fatalf("08/01 esperado 1 lançamento de 1000, obteve n=%d v=%v", n, v)
	}
	if !semEstorno(d08) {
		t.Fatalf("08/01 incremental não deveria gerar estorno")
	}

	// D2 (09/01): |1300 - 1000| = 300, sem estorno.
	if _, err := svc.GerarMovimentoEscopo(ctx, d09, "NDF", "Posição"); err != nil {
		t.Fatalf("09/01: %v", err)
	}
	if n, v := valorMov(d09); n != 1 || v != 300 {
		t.Fatalf("09/01 esperado 300, obteve n=%d v=%v", n, v)
	}
	if !semEstorno(d09) {
		t.Fatalf("09/01 incremental não deveria gerar estorno")
	}

	// D3 (10/01, queda): |900 - 1300| = 400 (absoluto), sem estorno.
	if _, err := svc.GerarMovimentoEscopo(ctx, d10, "NDF", "Posição"); err != nil {
		t.Fatalf("10/01: %v", err)
	}
	if n, v := valorMov(d10); n != 1 || v != 400 {
		t.Fatalf("10/01 esperado 400 (absoluto), obteve n=%d v=%v", n, v)
	}
	if !semEstorno(d10) {
		t.Fatalf("10/01 incremental não deveria gerar estorno")
	}
}
