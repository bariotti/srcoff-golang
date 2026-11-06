package service

import (
	"context"
	"testing"

	"srcoff/internal/evaluator"
	"srcoff/internal/model"
	filerepo "srcoff/internal/repository/file"
)

type fakeFechamentoReader struct{ cfg model.FechamentoConfig }

func (f *fakeFechamentoReader) Carregar(context.Context) (model.FechamentoConfig, error) {
	return f.cfg, nil
}

// TestFechamentoBloqueiaCompetenciaPosterior valida o controle de fechamento (RF-132): a
// primeira competência de um escopo passa livre; uma competência (mês) POSTERIOR e não
// liberada é bloqueada; após liberar o mês/ano, processa normalmente.
func TestFechamentoBloqueiaCompetenciaPosterior(t *testing.T) {
	ctx := context.Background()
	eval := evaluator.New()
	dir := t.TempDir()

	out15 := d("2024-10-15") // terça
	nov15 := d("2024-11-15") // sexta — competência seguinte

	posRepo := &fakePosicaoRepo{registros: []model.PosicaoCarteira{
		posD1(1, out15, "NDF", "Posição", "B1", 100),
		posD1(2, nov15, "NDF", "Posição", "B1", 100),
	}}
	regraRepo := &fakeRegraRepo{regras: []model.RegraContabil{regraSimplesD1()}}
	movRepo := filerepo.NewMovimentoContabilRepo(dir)
	execRepo := filerepo.NewExecucaoRepo(dir)
	// D-1 desligado para isolar a trava de fechamento.
	padrao := &fakePadraoLookup{padroes: []model.PadraoArquivo{
		{Produto: "NDF", Dominio: "Posição", ObrigatorioMovD1: boolPtr(false)},
	}}
	fech := &fakeFechamentoReader{cfg: model.FechamentoConfig{Modo: model.FechamentoTodos}}
	svc := NewMovimentoContabilService(posRepo, regraRepo, movRepo, eval).
		ComExecucaoRepo(execRepo).ComPadraoRepo(padrao).ComFechamentoRepo(fech)

	// 1ª competência (out/2024) passa livre, mesmo sem liberação (não há fechamento anterior).
	if bloq, err := svc.GerarMovimentoEscopo(ctx, out15, "NDF", "Posição"); err != nil || len(bloq) != 0 {
		t.Fatalf("out/2024 deveria processar (1ª competência), err=%v bloq=%v", err, bloq)
	}

	// nov/2024 é posterior e não liberada → bloqueia com motivo "fechamento".
	bloq, err := svc.GerarMovimentoEscopo(ctx, nov15, "NDF", "Posição")
	if err != nil {
		t.Fatalf("nov/2024 erro: %v", err)
	}
	if len(bloq) != 1 || bloq[0].Motivo != "fechamento" {
		t.Fatalf("nov/2024 deveria bloquear por fechamento, bloq=%+v", bloq)
	}
	if p, _ := svc.ConsultarLancamentos(ctx, nov15, 1, 100); p.Total != 0 {
		t.Fatalf("nov/2024 não deveria ter lançamentos (bloqueado), total=%d", p.Total)
	}

	// Libera nov/2024 → processa normalmente.
	fech.cfg.Liberacoes = []model.FechamentoLiberacao{{Ano: 2024, Mes: 11}}
	if bloq, err := svc.GerarMovimentoEscopo(ctx, nov15, "NDF", "Posição"); err != nil || len(bloq) != 0 {
		t.Fatalf("nov/2024 deveria processar após liberação, err=%v bloq=%v", err, bloq)
	}
	if p, _ := svc.ConsultarLancamentos(ctx, nov15, 1, 100); p.Total == 0 {
		t.Fatalf("nov/2024 deveria ter lançamentos após liberação")
	}
}
