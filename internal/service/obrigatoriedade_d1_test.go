package service

import (
	"context"
	"testing"
	"time"

	"srcoff/internal/evaluator"
	"srcoff/internal/model"
	filerepo "srcoff/internal/repository/file"
)

type fakePadraoLookup struct{ padroes []model.PadraoArquivo }

func (f *fakePadraoLookup) Listar(_ context.Context) ([]model.PadraoArquivo, error) {
	return f.padroes, nil
}

func boolPtr(b bool) *bool { return &b }

func posD1(id int64, data time.Time, produto, dominio, boleto string, mtm float64) model.PosicaoCarteira {
	return model.PosicaoCarteira{
		ID: id, DataPosicaoCarteira: data, CodigoVersaoConteudo: 1,
		Campos: map[string]interface{}{
			"produto": produto, "dominio": dominio,
			"codigo_identificador_boleto": boleto,
			"valor_mtm":                   mtm,
			"moeda_principal_remanescente": "USD",
		},
	}
}

func regraSimplesD1() model.RegraContabil {
	return model.RegraContabil{
		ID: 1, Descricao: "R", Ativo: true, PostaReverte: true,
		Condicoes: []model.CondicaoRegra{{
			ID: 1, IDRegra: 1, Condicao: "valor_mtm > 0",
			ContaDebito: "1001", ContaCredito: "2001",
			CampoValor: "valor_mtm", CampoMoeda: "moeda_principal_remanescente",
			CampoBoleto: "codigo_identificador_boleto", Ativo: true,
		}},
	}
}

// TestObrigatoriedadeD1_Bloqueia valida que, com o padrão exigindo D-1 e já havendo
// movimento em uma data, processar uma data cujo D-1 útil não tem movimento é bloqueado;
// e que, após processar o D-1 útil, o processamento é liberado.
func TestObrigatoriedadeD1_Bloqueia(t *testing.T) {
	ctx := context.Background()
	eval := evaluator.New()
	dir := t.TempDir()

	seg08 := d("2024-01-08") // segunda, duas semanas antes
	sex12 := d("2024-01-12") // sexta (D-1 útil de 15/01)
	seg15 := d("2024-01-15") // segunda

	posRepo := &fakePosicaoRepo{registros: []model.PosicaoCarteira{
		posD1(1, seg08, "NDF", "Posição", "B1", 100),
		posD1(2, sex12, "NDF", "Posição", "B1", 100),
		posD1(3, seg15, "NDF", "Posição", "B1", 100),
	}}
	regraRepo := &fakeRegraRepo{regras: []model.RegraContabil{regraSimplesD1()}}
	movRepo := filerepo.NewMovimentoContabilRepo(dir)
	execRepo := filerepo.NewExecucaoRepo(dir)
	padrao := &fakePadraoLookup{padroes: []model.PadraoArquivo{
		{Produto: "NDF", Dominio: "Posição", ObrigatorioMovD1: boolPtr(true)},
	}}
	svc := NewMovimentoContabilService(posRepo, regraRepo, movRepo, eval).
		ComExecucaoRepo(execRepo).ComPadraoRepo(padrao)

	// Processa 08/01 (primeira execução do combo → permitido).
	bloq, err := svc.GerarMovimentoEscopo(ctx, seg08, "NDF", "Posição")
	if err != nil || len(bloq) != 0 {
		t.Fatalf("08/01 deveria processar sem bloqueio, err=%v bloq=%v", err, bloq)
	}

	// Processa 15/01: já há movimento (08/01), mas o D-1 útil (12/01) não tem → bloqueado.
	bloq, err = svc.GerarMovimentoEscopo(ctx, seg15, "NDF", "Posição")
	if err != nil {
		t.Fatalf("15/01 erro inesperado: %v", err)
	}
	if len(bloq) != 1 || bloq[0].Produto != "NDF" || bloq[0].Dominio != "Posição" {
		t.Fatalf("15/01 deveria ser bloqueado por falta de D-1, bloq=%v", bloq)
	}
	// Confirma que nada foi persistido para 15/01.
	if p, _ := svc.ConsultarLancamentos(ctx, seg15, 1, 100); p.Total != 0 {
		t.Fatalf("15/01 não deveria ter lançamentos (bloqueado), total=%d", p.Total)
	}

	// Processa 12/01 (D-1 útil). D-1 de 12/01 é 11/01 (sem movimento), mas já há 08/01 →
	// 12/01 também seria bloqueado? Não: usamos a regra só quando falta o D-1. Para liberar
	// 15/01 no teste, processamos 12/01 desligando a obrigatoriedade temporariamente.
	padrao.padroes[0].ObrigatorioMovD1 = boolPtr(false)
	if _, err := svc.GerarMovimentoEscopo(ctx, sex12, "NDF", "Posição"); err != nil {
		t.Fatalf("12/01 erro: %v", err)
	}
	padrao.padroes[0].ObrigatorioMovD1 = boolPtr(true)

	// Agora 15/01 tem D-1 útil (12/01) com movimento → liberado.
	bloq, err = svc.GerarMovimentoEscopo(ctx, seg15, "NDF", "Posição")
	if err != nil || len(bloq) != 0 {
		t.Fatalf("15/01 deveria processar após 12/01, err=%v bloq=%v", err, bloq)
	}
	if p, _ := svc.ConsultarLancamentos(ctx, seg15, 1, 100); p.Total == 0 {
		t.Fatalf("15/01 deveria ter lançamentos após liberado")
	}
}

// TestObrigatoriedadeD1_Desligada_EstornoMaiorDataAnterior valida que, com a flag false,
// não há validação e o estorno usa o movimento da maior data anterior disponível.
func TestObrigatoriedadeD1_Desligada_EstornoMaiorDataAnterior(t *testing.T) {
	ctx := context.Background()
	eval := evaluator.New()
	dir := t.TempDir()

	seg08 := d("2024-01-08")
	seg15 := d("2024-01-15")

	posRepo := &fakePosicaoRepo{registros: []model.PosicaoCarteira{
		posD1(1, seg08, "NDF", "Posição", "B1", 100),
		posD1(2, seg15, "NDF", "Posição", "B1", 100),
	}}
	regraRepo := &fakeRegraRepo{regras: []model.RegraContabil{regraSimplesD1()}}
	movRepo := filerepo.NewMovimentoContabilRepo(dir)
	execRepo := filerepo.NewExecucaoRepo(dir)
	padrao := &fakePadraoLookup{padroes: []model.PadraoArquivo{
		{Produto: "NDF", Dominio: "Posição", ObrigatorioMovD1: boolPtr(false)},
	}}
	svc := NewMovimentoContabilService(posRepo, regraRepo, movRepo, eval).
		ComExecucaoRepo(execRepo).ComPadraoRepo(padrao)

	// 08/01: sem movimento anterior → sem estorno, só o movimento do dia.
	if _, err := svc.GerarMovimentoEscopo(ctx, seg08, "NDF", "Posição"); err != nil {
		t.Fatalf("08/01 erro: %v", err)
	}
	est08, _ := movRepo.BuscarPorDataEIndicador(ctx, seg08, true)
	if len(est08) != 0 {
		t.Fatalf("08/01 não deveria ter estorno (sem data anterior), tem %d", len(est08))
	}

	// 15/01 com flag false: não valida D-1; estorno usa a maior data anterior (08/01).
	bloq, err := svc.GerarMovimentoEscopo(ctx, seg15, "NDF", "Posição")
	if err != nil || len(bloq) != 0 {
		t.Fatalf("15/01 não deveria bloquear (flag false), err=%v bloq=%v", err, bloq)
	}
	est15, _ := movRepo.BuscarPorDataEIndicador(ctx, seg15, true)
	if len(est15) == 0 {
		t.Fatalf("15/01 deveria ter estorno da maior data anterior (08/01)")
	}
}
