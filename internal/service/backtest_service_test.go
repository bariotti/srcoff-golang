package service

import (
	"context"
	"testing"
	"time"

	"srcoff/internal/evaluator"
	"srcoff/internal/model"
)

// TestBacktestRegra valida o backtest (RF-135): avalia a regra candidata sobre as últimas N
// datas com posição e agrega por dia e por par de contas, sem persistir.
func TestBacktestRegra(t *testing.T) {
	ctx := context.Background()
	eval := evaluator.New()
	d8 := d("2024-01-08") // segunda
	d9 := d("2024-01-09") // terça
	pos := func(dia time.Time, boleto string, valor float64) model.PosicaoCarteira {
		return model.PosicaoCarteira{
			DataPosicaoCarteira: dia, CodigoVersaoConteudo: 1,
			Campos: map[string]interface{}{
				"produto": "NDF", "dominio": "Posição",
				"codigo_identificador_boleto": boleto, "valor_mtm": valor, "moeda": "BRL",
			},
		}
	}
	svc := NewMovimentoContabilService(&fakePosicaoRepo{registros: []model.PosicaoCarteira{
		pos(d8, "B1", 1000), pos(d8, "B2", -5), // B2 negativo não gera
		pos(d9, "B1", 2000),
	}}, &fakeRegraRepo{}, &fakeMovimentoRepo{}, eval)

	in := model.BacktestInput{Dias: 2, Regra: model.SimulacaoRegraInput{
		Produto: "NDF", TipoLancamento: model.TipoReverte,
		Condicoes: []model.SimulacaoCondicao{{
			Condicao: "valor_mtm > 0", CampoValor: "valor_mtm", CampoMoeda: "moeda",
			ContaDebito: "1001", ContaCredito: "2001",
		}},
	}}
	res, err := svc.BacktestRegra(ctx, in)
	if err != nil {
		t.Fatalf("BacktestRegra: %v", err)
	}
	if res.DiasComPosicao != 2 {
		t.Fatalf("dias com posição: esperava 2, veio %d", res.DiasComPosicao)
	}
	if res.TotalLancamentos != 2 { // B1 em 09 e B1 em 08
		t.Fatalf("total lançamentos: esperava 2, veio %d", res.TotalLancamentos)
	}
	if res.SomaValor != 3000 {
		t.Fatalf("soma valor: esperava 3000, veio %v", res.SomaValor)
	}
	if res.DataFim != "2024-01-09" || res.DataInicio != "2024-01-08" {
		t.Fatalf("intervalo: %s..%s", res.DataInicio, res.DataFim)
	}
	if len(res.Dias) != 2 || res.Dias[0].Data != "2024-01-09" {
		t.Fatalf("série por dia (mais recente primeiro) inesperada: %+v", res.Dias)
	}
	if len(res.PorConta) != 1 || res.PorConta[0].Qtd != 2 || res.PorConta[0].SomaValor != 3000 {
		t.Fatalf("por conta: %+v", res.PorConta)
	}
}

func TestBacktestRegraSemPosicao(t *testing.T) {
	svc := NewMovimentoContabilService(&fakePosicaoRepo{}, &fakeRegraRepo{}, &fakeMovimentoRepo{}, evaluator.New())
	_, err := svc.BacktestRegra(context.Background(), model.BacktestInput{Dias: 5, Regra: model.SimulacaoRegraInput{
		Condicoes: []model.SimulacaoCondicao{{Condicao: "x > 0", CampoValor: "x", CampoMoeda: "m", ContaDebito: "1", ContaCredito: "2"}},
	}})
	if err == nil {
		t.Fatal("esperava erro quando não há posição importada")
	}
}
