package service

import (
	"context"
	"testing"

	"srcoff/internal/evaluator"
	"srcoff/internal/model"
	filerepo "srcoff/internal/repository/file"
)

// TestExecucaoBateComConsulta garante que a contagem gravada na execução
// (Processados × Pendentes) é a MESMA exibida na consulta de movimento:
// versão vigente e sem os pares lançamento+estorno de saldo zero.
//
// Cenário: BOL-CANCEL existe na sexta e na segunda com o mesmo valor → o lançamento
// da segunda e o estorno da sexta se cancelam (ocultos na consulta). BOL-KEEP só
// existe na segunda → permanece visível. Portanto a segunda deve mostrar 1 lançamento
// visível, e a execução deve registrar 1 lançamento / 0 estornos (e não 2 / 1 crus).
func TestExecucaoBateComConsulta(t *testing.T) {
	ctx := context.Background()
	eval := evaluator.New()
	dir := t.TempDir()

	sexta := DiaUtilAnterior(baseDate) // 2024-01-12 (sexta), dia útil anterior à segunda
	segunda := baseDate                // 2024-01-15 (segunda)

	posRepo := &fakePosicaoRepo{registros: []model.PosicaoCarteira{
		posIntegra(1, sexta, "BOL-CANCEL", 200.0, 1000.0, "USD"),
		posIntegra(2, segunda, "BOL-CANCEL", 200.0, 1000.0, "USD"),
		posIntegra(3, segunda, "BOL-KEEP", 300.0, 1000.0, "USD"),
	}}

	regra := model.RegraContabil{
		ID: 1, Descricao: "Regra", Ativo: true, PostaReverte: true,
		Condicoes: []model.CondicaoRegra{{
			ID: 1, IDRegra: 1, Condicao: "valor_mtm > 0",
			ContaDebito: "1001", ContaCredito: "2001",
			CampoValor: "valor_mtm", CampoMoeda: "moeda_principal_remanescente",
			CampoBoleto: "codigo_identificador_boleto", Ativo: true,
		}},
	}
	regraRepo := &fakeRegraRepo{regras: []model.RegraContabil{regra}}

	movRepo := filerepo.NewMovimentoContabilRepo(dir)
	execRepo := filerepo.NewExecucaoRepo(dir)
	svc := NewMovimentoContabilService(posRepo, regraRepo, movRepo, eval).ComExecucaoRepo(execRepo)

	if _, err := svc.GerarMovimentoEscopo(ctx, sexta, "", ""); err != nil {
		t.Fatalf("processar sexta: %v", err)
	}
	if _, err := svc.GerarMovimentoEscopo(ctx, segunda, "", ""); err != nil {
		t.Fatalf("processar segunda: %v", err)
	}

	// Consulta (fonte da verdade): quantidade visível para a segunda.
	consulta, err := svc.ConsultarLancamentosFiltrado(ctx, segunda, segunda, "", 0, "vigente", 1, 1000)
	if err != nil {
		t.Fatalf("consulta: %v", err)
	}
	if consulta.Total != 1 {
		t.Fatalf("consulta deveria ter 1 lançamento visível (BOL-KEEP), tem %d", consulta.Total)
	}

	// Execução (Processados × Pendentes) deve bater com a consulta.
	execs, err := execRepo.ListarPorData(ctx, segunda)
	if err != nil {
		t.Fatalf("listar execução: %v", err)
	}
	if len(execs) != 1 {
		t.Fatalf("esperava 1 registro de execução para a segunda, obteve %d", len(execs))
	}
	e := execs[0]
	if e.QtdLancamentos != 1 || e.QtdEstornos != 0 {
		t.Fatalf("execução deveria bater com a consulta (1 lançamento, 0 estornos), obteve %d/%d", e.QtdLancamentos, e.QtdEstornos)
	}
}
