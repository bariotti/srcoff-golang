package service

import (
	"context"
	"testing"

	"srcoff/internal/evaluator"
	"srcoff/internal/model"
	filerepo "srcoff/internal/repository/file"
)

// posMB monta uma posição com veículo, moeda e data de cotação, para os testes de moeda base.
func posMB(id int64, boleto string, mtm float64, moeda string) model.PosicaoCarteira {
	return model.PosicaoCarteira{
		ID: id, DataPosicaoCarteira: d("2024-01-08"), CodigoVersaoConteudo: 1,
		Campos: map[string]interface{}{
			"produto": "NDF", "dominio": "Posição",
			"codigo_identificador_boleto": boleto,
			"valor_mtm":                   mtm,
			"moeda":                       moeda,
			"veiculo_operacao":            "NASSAU",
			"data_cotacao":                "2024-01-08",
		},
	}
}

// regraMB: regra não-reverte que converte a moeda base (base vem da conversão).
func regraMB() model.RegraContabil {
	return model.RegraContabil{
		ID: 1, Descricao: "MB", Ativo: true, TipoLancamento: model.TipoNaoReverte,
		Condicoes: []model.CondicaoRegra{{
			ID: 1, IDRegra: 1, Condicao: "valor_mtm > 0",
			ContaDebito: "1001", ContaCredito: "2001",
			CampoValor: "valor_mtm", CampoMoeda: "moeda", CampoBoleto: "codigo_identificador_boleto", Ativo: true,
			BaseNaPosicao: false, CampoDataConversao: "data_cotacao",
		}},
	}
}

func svcMB(t *testing.T, dir string, pos []model.PosicaoCarteira, regra model.RegraContabil, comCotacao bool) (*MovimentoContabilService, *filerepo.MovimentoContabilRepo) {
	t.Helper()
	ctx := context.Background()
	eval := evaluator.New()
	movRepo := filerepo.NewMovimentoContabilRepo(dir)
	execRepo := filerepo.NewExecucaoRepo(dir)
	incRepo := filerepo.NewInconsistenciaRepo(dir)
	padrao := &fakePadraoLookup{padroes: []model.PadraoArquivo{
		{Produto: "NDF", Dominio: "Posição", ColunaVeiculo: "veiculo_operacao", ObrigatorioMovD1: boolPtr(false)},
	}}
	veicSvc := NewVeiculoDeParaService(filerepo.NewVeiculoDeParaRepo(dir))
	if _, err := veicSvc.Criar(ctx, model.VeiculoDePara{Veiculo: "NASSAU", Codigo: "001", MoedaBase: "USD"}); err != nil {
		t.Fatalf("criar de/para: %v", err)
	}
	cotSvc := NewCotacaoService(filerepo.NewCotacaoRepo(dir))
	if comCotacao {
		// USD→BRL dividir por 5: 1000 BRL / 5 = 200 USD.
		if _, err := cotSvc.Criar(ctx, model.Cotacao{Data: d("2024-01-08"), Moeda1: "USD", Moeda2: "BRL", Direcao: model.DirecaoDividir, Taxa: 5}); err != nil {
			t.Fatalf("criar cotação: %v", err)
		}
	}
	svc := NewMovimentoContabilService(&fakePosicaoRepo{registros: pos}, &fakeRegraRepo{regras: []model.RegraContabil{regra}}, movRepo, eval).
		ComExecucaoRepo(execRepo).ComInconsistenciaRepo(incRepo).ComPadraoRepo(padrao).
		ComMoedaBaseRepo(veicSvc).ComCotacaoRepo(cotSvc)
	return svc, movRepo
}

// TestMoedaBase_Conversao: moeda do veículo (USD) != moeda do lançamento (BRL) → converte pela cotação.
func TestMoedaBase_Conversao(t *testing.T) {
	ctx := context.Background()
	svc, movRepo := svcMB(t, t.TempDir(), []model.PosicaoCarteira{posMB(1, "B1", 1000, "BRL")}, regraMB(), true)
	if _, err := svc.GerarMovimentoEscopo(ctx, d("2024-01-08"), "", ""); err != nil {
		t.Fatalf("gerar: %v", err)
	}
	ls, _ := movRepo.BuscarPorDataEIndicador(ctx, d("2024-01-08"), false)
	if len(ls) != 1 {
		t.Fatalf("esperava 1 lançamento, obteve %d", len(ls))
	}
	if ls[0].MoedaBaseVeiculo != "USD" {
		t.Fatalf("moeda base esperada USD, obteve %q", ls[0].MoedaBaseVeiculo)
	}
	if ls[0].ValorMoedaBaseVeiculo != 200 {
		t.Fatalf("valor moeda base esperado 200 (1000/5), obteve %v", ls[0].ValorMoedaBaseVeiculo)
	}
}

// TestMoedaBase_DataComoTime: a coluna da data de conversão é a data da posição (injetada como
// time.Time) — deve ser normalizada e permitir a conversão (regressão do erro reportado).
func TestMoedaBase_DataComoTime(t *testing.T) {
	ctx := context.Background()
	regra := regraMB()
	regra.Condicoes[0].CampoDataConversao = "data_posicao_carteira" // injetado como time.Time pelo PosicaoToEnv
	svc, movRepo := svcMB(t, t.TempDir(), []model.PosicaoCarteira{posMB(1, "B1", 1000, "BRL")}, regra, true)
	if _, err := svc.GerarMovimentoEscopo(ctx, d("2024-01-08"), "", ""); err != nil {
		t.Fatalf("gerar: %v", err)
	}
	ls, _ := movRepo.BuscarPorDataEIndicador(ctx, d("2024-01-08"), false)
	if len(ls) != 1 || ls[0].MoedaBaseVeiculo != "USD" || ls[0].ValorMoedaBaseVeiculo != 200 {
		t.Fatalf("esperava 1 lançamento USD/200 com data time.Time, obteve %+v", ls)
	}
}

// TestMoedaBase_MesmaMoeda: moeda do veículo == moeda do lançamento → valor/moeda base = valor/moeda do lançamento.
func TestMoedaBase_MesmaMoeda(t *testing.T) {
	ctx := context.Background()
	svc, movRepo := svcMB(t, t.TempDir(), []model.PosicaoCarteira{posMB(1, "B1", 1000, "USD")}, regraMB(), false)
	if _, err := svc.GerarMovimentoEscopo(ctx, d("2024-01-08"), "", ""); err != nil {
		t.Fatalf("gerar: %v", err)
	}
	ls, _ := movRepo.BuscarPorDataEIndicador(ctx, d("2024-01-08"), false)
	if len(ls) != 1 || ls[0].MoedaBaseVeiculo != "USD" || ls[0].ValorMoedaBaseVeiculo != 1000 {
		t.Fatalf("esperava 1 lançamento USD/1000, obteve %+v", ls)
	}
}

// TestMoedaBase_SemCotacao: moedas diferentes e SEM cotação → inconsistência, lançamento não gerado.
func TestMoedaBase_SemCotacao(t *testing.T) {
	ctx := context.Background()
	svc, movRepo := svcMB(t, t.TempDir(), []model.PosicaoCarteira{posMB(1, "B1", 1000, "BRL")}, regraMB(), false)
	if _, err := svc.GerarMovimentoEscopo(ctx, d("2024-01-08"), "", ""); err != nil {
		t.Fatalf("gerar: %v", err)
	}
	ls, _ := movRepo.BuscarPorDataEIndicador(ctx, d("2024-01-08"), false)
	if len(ls) != 0 {
		t.Fatalf("sem cotação não deveria gerar lançamento, obteve %d", len(ls))
	}
}

// TestMoedaBase_DaPosicao: base vem da posição → copia valor e moeda base das colunas.
func TestMoedaBase_DaPosicao(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	pos := model.PosicaoCarteira{
		ID: 1, DataPosicaoCarteira: d("2024-01-08"), CodigoVersaoConteudo: 1,
		Campos: map[string]interface{}{
			"produto": "NDF", "dominio": "Posição",
			"codigo_identificador_boleto": "B1",
			"valor_mtm":                   1000.0, "moeda": "BRL",
			"veiculo_operacao":            "NASSAU",
			"valor_base_col":              193.0, "moeda_base_col": "USD",
		},
	}
	regra := regraMB()
	regra.Condicoes[0].BaseNaPosicao = true
	regra.Condicoes[0].CampoDataConversao = ""
	regra.Condicoes[0].CampoValorMoedaBase = "valor_base_col"
	regra.Condicoes[0].CampoMoedaBase = "moeda_base_col"
	svc, movRepo := svcMB(t, dir, []model.PosicaoCarteira{pos}, regra, false)
	if _, err := svc.GerarMovimentoEscopo(ctx, d("2024-01-08"), "", ""); err != nil {
		t.Fatalf("gerar: %v", err)
	}
	ls, _ := movRepo.BuscarPorDataEIndicador(ctx, d("2024-01-08"), false)
	if len(ls) != 1 || ls[0].MoedaBaseVeiculo != "USD" || ls[0].ValorMoedaBaseVeiculo != 193 {
		t.Fatalf("esperava base da posição USD/193, obteve %+v", ls)
	}
}
