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
		ID: 1, Descricao: "R", Ativo: true, TipoLancamento: model.TipoReverte,
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

// TestObrigatoriedadeD1_ReprocessarMesmaDataNaoBloqueia reproduz o bug relatado: uma
// primeira rodada que não gerou lançamentos (por inconsistências) registra execução para
// a própria data; ao reprocessar o MESMO dia, a obrigatoriedade de D-1 não pode bloquear,
// pois a execução da própria data não conta como histórico anterior.
func TestObrigatoriedadeD1_ReprocessarMesmaDataNaoBloqueia(t *testing.T) {
	ctx := context.Background()
	eval := evaluator.New()
	dir := t.TempDir()

	seg15 := d("2024-01-15") // segunda; D-1 útil = sexta 12/01

	posRepo := &fakePosicaoRepo{registros: []model.PosicaoCarteira{
		posD1(1, seg15, "NDF", "Posição", "B1", 100),
	}}
	regraRepo := &fakeRegraRepo{regras: []model.RegraContabil{regraSimplesD1()}}
	movRepo := filerepo.NewMovimentoContabilRepo(dir)
	execRepo := filerepo.NewExecucaoRepo(dir)
	padrao := &fakePadraoLookup{padroes: []model.PadraoArquivo{
		{Produto: "NDF", Dominio: "Posição", ObrigatorioMovD1: boolPtr(true)},
	}}
	svc := NewMovimentoContabilService(posRepo, regraRepo, movRepo, eval).
		ComExecucaoRepo(execRepo).ComPadraoRepo(padrao)

	// Simula a 1ª rodada do mesmo dia (0 lançamentos por inconsistências), que registrou
	// execução para a própria data 15/01.
	if err := execRepo.RegistrarExecucao(ctx, model.MovimentoExecucao{
		DataLote: seg15, Produto: "NDF", Dominio: "Posição", QtdLancamentos: 0,
	}); err != nil {
		t.Fatalf("registrar execução anterior: %v", err)
	}

	// Reprocessar a MESMA data não deve ser bloqueado (não há data anterior).
	bloq, err := svc.GerarMovimentoEscopo(ctx, seg15, "NDF", "Posição")
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(bloq) != 0 {
		t.Fatalf("reprocessar a 1ª data não deveria bloquear, bloq=%v", bloq)
	}
	if p, _ := svc.ConsultarLancamentos(ctx, seg15, 1, 100); p.Total == 0 {
		t.Fatalf("15/01 deveria ter lançamentos após o reprocesso")
	}
}

// TestObrigatoriedadeD1_DiaAnteriorSemLancamentoNaoConta valida que um dia anterior que
// rodou mas NÃO gerou lançamentos (0 lançamentos) não conta como "tem contábil": por isso
// não dispara a obrigatoriedade de D-1 ao processar uma data posterior.
func TestObrigatoriedadeD1_DiaAnteriorSemLancamentoNaoConta(t *testing.T) {
	ctx := context.Background()
	eval := evaluator.New()
	dir := t.TempDir()

	qua10 := d("2024-01-10") // quarta; D-1 útil = terça 09/01
	seg08 := d("2024-01-08") // segunda, anterior; teve rodada com 0 lançamentos

	posRepo := &fakePosicaoRepo{registros: []model.PosicaoCarteira{
		posD1(1, qua10, "NDF", "Posição", "B1", 100),
	}}
	regraRepo := &fakeRegraRepo{regras: []model.RegraContabil{regraSimplesD1()}}
	movRepo := filerepo.NewMovimentoContabilRepo(dir)
	execRepo := filerepo.NewExecucaoRepo(dir)
	padrao := &fakePadraoLookup{padroes: []model.PadraoArquivo{
		{Produto: "NDF", Dominio: "Posição", ObrigatorioMovD1: boolPtr(true)},
	}}
	svc := NewMovimentoContabilService(posRepo, regraRepo, movRepo, eval).
		ComExecucaoRepo(execRepo).ComPadraoRepo(padrao)

	// 08/01 rodou mas não gerou lançamentos (0) — não é "contábil".
	if err := execRepo.RegistrarExecucao(ctx, model.MovimentoExecucao{
		DataLote: seg08, Produto: "NDF", Dominio: "Posição", QtdLancamentos: 0,
	}); err != nil {
		t.Fatalf("registrar execução sem lançamentos: %v", err)
	}

	// Processar 10/01 não deve bloquear, pois não há contábil (lançamentos) em data anterior.
	bloq, err := svc.GerarMovimentoEscopo(ctx, qua10, "NDF", "Posição")
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(bloq) != 0 {
		t.Fatalf("10/01 não deveria bloquear (08/01 teve 0 lançamentos), bloq=%v", bloq)
	}
	if p, _ := svc.ConsultarLancamentos(ctx, qua10, 1, 100); p.Total == 0 {
		t.Fatalf("10/01 deveria ter lançamentos")
	}
}
