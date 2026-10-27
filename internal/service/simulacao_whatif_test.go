package service

import (
	"context"
	"testing"

	"srcoff/internal/evaluator"
	"srcoff/internal/model"
)

// TestSimularRegra_CandidataSobrePosicao valida a simulação What-if (RF-122):
// a regra candidata é avaliada sobre a posição do escopo, SEM persistir, e devolve
// os lançamentos que geraria e as inconsistências. O escopo (produto/domínio) filtra
// as posições avaliadas.
func TestSimularRegra_CandidataSobrePosicao(t *testing.T) {
	ctx := context.Background()
	eval := evaluator.New()
	dia := d("2024-01-08")

	pos := func(boleto string, valor float64, moeda string) model.PosicaoCarteira {
		return model.PosicaoCarteira{
			ID: 1, DataPosicaoCarteira: dia, CodigoVersaoConteudo: 1,
			Campos: map[string]interface{}{
				"produto": "NDF", "dominio": "Posição",
				"codigo_identificador_boleto": boleto, "valor_mtm": valor, "moeda": moeda,
			},
		}
	}
	// Posição de outro produto — excluída pelo escopo produto=NDF.
	posSwap := model.PosicaoCarteira{
		ID: 9, DataPosicaoCarteira: dia, CodigoVersaoConteudo: 1,
		Campos: map[string]interface{}{
			"produto": "SWAP", "dominio": "Posição",
			"codigo_identificador_boleto": "S1", "valor_mtm": 500.0, "moeda": "BRL",
		},
	}

	svc := NewMovimentoContabilService(
		&fakePosicaoRepo{registros: []model.PosicaoCarteira{
			pos("B1", 1000, "BRL"), // casa a condição (>0) → gera
			pos("B2", -50, "BRL"),  // condição falsa → não gera
			posSwap,                // produto diferente → fora do escopo
		}},
		&fakeRegraRepo{}, &fakeMovimentoRepo{}, eval,
	)

	in := model.SimulacaoRegraInput{
		Data:           "2024-01-08",
		Produto:        "NDF", // escopo
		TipoLancamento: model.TipoReverte,
		Condicoes: []model.SimulacaoCondicao{{
			Condicao: "valor_mtm > 0", CampoValor: "valor_mtm", CampoMoeda: "moeda",
			ContaDebito: "1001", ContaCredito: "2001",
		}},
	}
	res, err := svc.SimularRegra(ctx, in)
	if err != nil {
		t.Fatalf("SimularRegra: %v", err)
	}
	if res.PosicoesAvaliadas != 2 { // só as duas de NDF entram no escopo
		t.Errorf("avaliadas: esperava 2, obteve %d", res.PosicoesAvaliadas)
	}
	if res.TotalLancamentos != 1 || len(res.Lancamentos) != 1 {
		t.Fatalf("lançamentos: esperava 1, obteve total=%d len=%d", res.TotalLancamentos, len(res.Lancamentos))
	}
	l := res.Lancamentos[0]
	if l.Boleto != "B1" || l.Valor != 1000 || l.Moeda != "BRL" || l.ContaDebito != "1001" || l.ContaCredito != "2001" {
		t.Errorf("lançamento inesperado: %+v", l)
	}
	if res.TotalInconsistencias != 0 {
		t.Errorf("inconsistências: esperava 0, obteve %d (%+v)", res.TotalInconsistencias, res.Inconsistencias)
	}
	if res.Aviso != "" {
		t.Errorf("aviso: esperava vazio para reverte, obteve %q", res.Aviso)
	}
}

// TestSimularRegra_FiltroBoleto: o escopo por boleto avalia só aquele boleto.
func TestSimularRegra_FiltroBoleto(t *testing.T) {
	ctx := context.Background()
	eval := evaluator.New()
	dia := d("2024-01-08")
	pos := func(boleto string, valor float64) model.PosicaoCarteira {
		return model.PosicaoCarteira{
			ID: 1, DataPosicaoCarteira: dia, CodigoVersaoConteudo: 1,
			Campos: map[string]interface{}{
				"produto": "NDF", "dominio": "Posição",
				"codigo_identificador_boleto": boleto, "valor_mtm": valor, "moeda": "BRL",
			},
		}
	}
	svc := NewMovimentoContabilService(
		&fakePosicaoRepo{registros: []model.PosicaoCarteira{pos("B1", 1000), pos("B2", 2000)}},
		&fakeRegraRepo{}, &fakeMovimentoRepo{}, eval,
	)
	res, err := svc.SimularRegra(ctx, model.SimulacaoRegraInput{
		Data: "2024-01-08", Boleto: "B2", TipoLancamento: model.TipoReverte,
		Condicoes: []model.SimulacaoCondicao{{
			Condicao: "valor_mtm > 0", CampoValor: "valor_mtm", CampoMoeda: "moeda",
			ContaDebito: "1001", ContaCredito: "2001",
		}},
	})
	if err != nil {
		t.Fatalf("SimularRegra: %v", err)
	}
	if res.PosicoesAvaliadas != 1 || res.TotalLancamentos != 1 {
		t.Fatalf("filtro boleto: esperava 1 avaliada e 1 lançamento, obteve av=%d lanc=%d", res.PosicoesAvaliadas, res.TotalLancamentos)
	}
	if res.Lancamentos[0].Boleto != "B2" {
		t.Errorf("filtro boleto: esperava B2, obteve %s", res.Lancamentos[0].Boleto)
	}
}

// TestSimularRegra_InconsistenciaCampoAusente: campo referenciado inexistente na
// posição vira inconsistência e não gera lançamento (mesma semântica da geração).
func TestSimularRegra_InconsistenciaCampoAusente(t *testing.T) {
	ctx := context.Background()
	eval := evaluator.New()
	dia := d("2024-01-08")

	pos := model.PosicaoCarteira{
		ID: 1, DataPosicaoCarteira: dia, CodigoVersaoConteudo: 1,
		Campos: map[string]interface{}{
			"produto": "NDF", "dominio": "Posição", "codigo_identificador_boleto": "B1", "valor_mtm": 1000.0, "moeda": "BRL",
		},
	}
	svc := NewMovimentoContabilService(
		&fakePosicaoRepo{registros: []model.PosicaoCarteira{pos}},
		&fakeRegraRepo{}, &fakeMovimentoRepo{}, eval,
	)
	in := model.SimulacaoRegraInput{
		Data:           "2024-01-08",
		TipoLancamento: model.TipoIncremental, // deve preencher o Aviso
		Condicoes: []model.SimulacaoCondicao{{
			Condicao: "campo_inexistente > 0", CampoValor: "valor_mtm", CampoMoeda: "moeda",
			ContaDebito: "1001", ContaCredito: "2001",
		}},
	}
	res, err := svc.SimularRegra(ctx, in)
	if err != nil {
		t.Fatalf("SimularRegra: %v", err)
	}
	if res.TotalLancamentos != 0 {
		t.Errorf("lançamentos: esperava 0, obteve %d", res.TotalLancamentos)
	}
	if res.TotalInconsistencias != 1 || len(res.Inconsistencias) != 1 {
		t.Fatalf("inconsistências: esperava 1, obteve total=%d len=%d", res.TotalInconsistencias, len(res.Inconsistencias))
	}
	if res.Aviso == "" {
		t.Errorf("aviso: esperava nota para regra incremental")
	}
}

// TestSimularRegra_SemCondicao: sem condição válida → erro.
func TestSimularRegra_SemCondicao(t *testing.T) {
	svc := NewMovimentoContabilService(&fakePosicaoRepo{}, &fakeRegraRepo{}, &fakeMovimentoRepo{}, evaluator.New())
	if _, err := svc.SimularRegra(context.Background(), model.SimulacaoRegraInput{Data: "2024-01-08"}); err == nil {
		t.Fatal("esperava erro quando não há condição")
	}
}
