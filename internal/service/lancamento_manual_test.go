package service

import (
	"context"
	"testing"

	"srcoff/internal/evaluator"
	"srcoff/internal/model"
	filerepo "srcoff/internal/repository/file"
)

func regraManual() model.RegraContabil {
	return model.RegraContabil{
		ID: 1, Descricao: "Regra Manual", Ativo: true, TipoLancamento: model.TipoReverte,
		Condicoes: []model.CondicaoRegra{{
			ID: 1, IDRegra: 1, Condicao: "valor_mtm > 0",
			ContaDebito: "1001", ContaCredito: "2001",
			CampoValor: "valor_mtm", CampoMoeda: "moeda", CampoBoleto: "codigo_identificador_boleto", Ativo: true,
		}},
	}
}

// TestLancamentoManual_InsereComOrigemEContasDaCondicao valida a inserção manual:
// contas vêm da condição, origem=manual, versão 1, e o saldo é atualizado.
func TestLancamentoManual_InsereComOrigemEContasDaCondicao(t *testing.T) {
	ctx := context.Background()
	eval := evaluator.New()
	dir := t.TempDir()
	movRepo := filerepo.NewMovimentoContabilRepo(dir)
	saldoRepo := filerepo.NewSaldoContaRepo(dir)
	svc := NewMovimentoContabilService(&fakePosicaoRepo{}, &fakeRegraRepo{regras: []model.RegraContabil{regraManual()}}, movRepo, eval).
		ComSaldoRepo(saldoRepo)

	n, err := svc.InserirLancamentosManuais(ctx, []model.LancamentoManualInput{
		{Data: "2024-01-08", Produto: "NDF", Dominio: "Posição", IDRegra: 1, IDCondicao: 1, Boleto: "M1", Valor: 500, Moeda: "USD"},
		{Data: "2024-01-08", Produto: "NDF", Dominio: "Posição", IDRegra: 1, IDCondicao: 1, Boleto: "M2", Valor: 300, Moeda: "USD"},
	})
	if err != nil {
		t.Fatalf("inserir manual: %v", err)
	}
	if n != 2 {
		t.Fatalf("esperava 2 inseridos, obteve %d", n)
	}

	ls, _ := movRepo.BuscarPorDataEIndicador(ctx, d("2024-01-08"), false)
	if len(ls) != 2 {
		t.Fatalf("esperava 2 lançamentos, obteve %d", len(ls))
	}
	for _, l := range ls {
		if l.Origem != model.OrigemManual {
			t.Fatalf("origem esperada manual, obteve %q", l.Origem)
		}
		if l.ContaDebito != "1001" || l.ContaCredito != "2001" {
			t.Fatalf("contas deveriam vir da condição (1001/2001), obteve %s/%s", l.ContaDebito, l.ContaCredito)
		}
		if l.DescricaoRegraContabil != "Regra Manual" || l.DescricaoCondicaoContabil != "valor_mtm > 0" {
			t.Fatalf("descrições regra/condição incorretas: %q / %q", l.DescricaoRegraContabil, l.DescricaoCondicaoContabil)
		}
		if l.CodigoVersaoConteudo != 1 {
			t.Fatalf("versão esperada 1, obteve %d", l.CodigoVersaoConteudo)
		}
	}

	saldos, _ := saldoRepo.Listar(ctx, model.SaldoFiltro{})
	m := map[string]float64{}
	for _, s := range saldos {
		m[s.CodigoIdentificadorBoleto] = s.Saldo
	}
	if m["M1"] != 500 || m["M2"] != 300 {
		t.Fatalf("saldo manual incorreto: M1=%v M2=%v", m["M1"], m["M2"])
	}
}

// TestLancamentoManual_EstornoInverteContas valida que um estorno manual
// (indicador_reversao=true) inverte débito/crédito da condição — mesma convenção do estorno
// automático — de modo que o par normal+estorno da mesma condição/boleto se anule.
func TestLancamentoManual_EstornoInverteContas(t *testing.T) {
	ctx := context.Background()
	eval := evaluator.New()
	dir := t.TempDir()
	movRepo := filerepo.NewMovimentoContabilRepo(dir)
	saldoRepo := filerepo.NewSaldoContaRepo(dir)
	svc := NewMovimentoContabilService(&fakePosicaoRepo{}, &fakeRegraRepo{regras: []model.RegraContabil{regraManual()}}, movRepo, eval).
		ComSaldoRepo(saldoRepo)

	_, err := svc.InserirLancamentosManuais(ctx, []model.LancamentoManualInput{
		{Data: "2024-01-08", Produto: "NDF", Dominio: "Posição", IDRegra: 1, IDCondicao: 1, Boleto: "E1", Valor: 500, Moeda: "USD"},
		{Data: "2024-01-08", Produto: "NDF", Dominio: "Posição", IDRegra: 1, IDCondicao: 1, Boleto: "E1", Valor: 500, Moeda: "USD", IndicadorReversao: true},
	})
	if err != nil {
		t.Fatalf("inserir manual: %v", err)
	}

	// O estorno deve ter as contas invertidas em relação à condição (1001/2001 → 2001/1001).
	est, _ := movRepo.BuscarPorDataEIndicador(ctx, d("2024-01-08"), true)
	if len(est) != 1 {
		t.Fatalf("esperava 1 estorno, obteve %d", len(est))
	}
	if est[0].ContaDebito != "2001" || est[0].ContaCredito != "1001" {
		t.Fatalf("estorno deveria inverter contas (2001/1001), obteve %s/%s", est[0].ContaDebito, est[0].ContaCredito)
	}

	// Net/"sem cancelados": o par normal+estorno da mesma condição/boleto se anula → 0 visíveis.
	vis, err := movRepo.ConsultarPaginadoFiltradoSemCancelados(ctx, model.FiltroMovimento{DataInicio: d("2024-01-08"), DataFim: d("2024-01-08"), VersaoModo: "vigente", Pagina: 1, Tamanho: 999999})
	if err != nil {
		t.Fatalf("sem cancelados: %v", err)
	}
	if len(vis.Lancamentos) != 0 {
		t.Fatalf("par normal+estorno deveria anular (0 visíveis), obteve %d", len(vis.Lancamentos))
	}

	// Saldo líquido da chave de E1 deve ser 0.
	saldos, _ := saldoRepo.Listar(ctx, model.SaldoFiltro{IncluiZero: true})
	for _, s := range saldos {
		if s.CodigoIdentificadorBoleto == "E1" && s.Saldo != 0 {
			t.Fatalf("saldo de E1 deveria ser 0, obteve %v", s.Saldo)
		}
	}
}

// TestLancamentoManual_ValidacaoTudoOuNada: regra inexistente → nada inserido.
func TestLancamentoManual_ValidacaoTudoOuNada(t *testing.T) {
	ctx := context.Background()
	eval := evaluator.New()
	dir := t.TempDir()
	movRepo := filerepo.NewMovimentoContabilRepo(dir)
	svc := NewMovimentoContabilService(&fakePosicaoRepo{}, &fakeRegraRepo{regras: []model.RegraContabil{regraManual()}}, movRepo, eval)

	_, err := svc.InserirLancamentosManuais(ctx, []model.LancamentoManualInput{
		{Data: "2024-01-08", Produto: "NDF", Dominio: "Posição", IDRegra: 1, IDCondicao: 1, Boleto: "M1", Valor: 500, Moeda: "USD"},
		{Data: "2024-01-08", Produto: "NDF", Dominio: "Posição", IDRegra: 99, IDCondicao: 1, Boleto: "M2", Valor: 300, Moeda: "USD"},
	})
	if err == nil {
		t.Fatal("esperava erro de validação (regra 99 inexistente)")
	}
	ls, _ := movRepo.BuscarPorDataEIndicador(ctx, d("2024-01-08"), false)
	if len(ls) != 0 {
		t.Fatalf("nenhum lançamento deveria ter sido inserido, obteve %d", len(ls))
	}
}
