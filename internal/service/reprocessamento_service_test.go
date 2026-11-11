package service

import (
	"context"
	"testing"
	"time"

	"srcoff/internal/evaluator"
	"srcoff/internal/model"
)

// TestReprocessarSeletivo valida o reprocessamento seletivo (RF-137): com um boleto informado,
// reprocessa apenas as combinações (produto/domínio) em que ele aparece; o escopo
// produto/domínio restringe ainda mais; boleto inexistente é erro.
func TestReprocessarSeletivo(t *testing.T) {
	ctx := context.Background()
	eval := evaluator.New()
	dia := baseDate // dia útil

	pos := func(id int64, produto, boleto string) model.PosicaoCarteira {
		return model.PosicaoCarteira{
			ID: id, DataPosicaoCarteira: dia, CodigoVersaoConteudo: 1,
			Campos: map[string]interface{}{
				"produto": produto, "dominio": "Posição",
				"codigo_identificador_boleto": boleto, "valor_mtm": 100.0, "moeda": "USD",
			},
		}
	}
	posRepo := &fakePosicaoRepo{registros: []model.PosicaoCarteira{
		pos(1, "NDF", "B1"), pos(2, "SWAP", "B1"), pos(3, "NDF", "B2"),
	}}
	regra := model.RegraContabil{
		ID: 1, Descricao: "R", Ativo: true,
		Condicoes: []model.CondicaoRegra{{
			ID: 1, IDRegra: 1, Condicao: "valor_mtm > 0", ContaDebito: "1001", ContaCredito: "2001",
			CampoValor: "valor_mtm", CampoMoeda: "moeda", CampoBoleto: "codigo_identificador_boleto", Ativo: true,
		}},
	}
	svc := NewMovimentoContabilService(posRepo, &fakeRegraRepo{regras: []model.RegraContabil{regra}}, &fakeMovimentoRepoPaginado{}, eval)

	// Boleto B1 aparece em NDF e SWAP → 2 combos.
	res, err := svc.ReprocessarSeletivo(ctx, dia, "", "", "B1")
	if err != nil {
		t.Fatalf("ReprocessarSeletivo B1: %v", err)
	}
	if len(res.CombosProcessados) != 2 {
		t.Fatalf("B1 deveria reprocessar 2 combos, veio %d (%+v)", len(res.CombosProcessados), res.CombosProcessados)
	}

	// Escopo produto=NDF restringe a 1 combo.
	res2, err := svc.ReprocessarSeletivo(ctx, dia, "NDF", "", "B1")
	if err != nil {
		t.Fatalf("ReprocessarSeletivo NDF/B1: %v", err)
	}
	if len(res2.CombosProcessados) != 1 || res2.CombosProcessados[0].Produto != "NDF" {
		t.Fatalf("NDF/B1 deveria reprocessar só NDF, veio %+v", res2.CombosProcessados)
	}

	// Boleto inexistente → erro.
	if _, err := svc.ReprocessarSeletivo(ctx, dia, "", "", "NAO-EXISTE"); err == nil {
		t.Fatal("boleto inexistente deveria dar erro")
	}
}

// TestReprocessarSeletivoDiaNaoUtil: reprocessar numa data não útil propaga o erro da geração.
func TestReprocessarSeletivoDiaNaoUtil(t *testing.T) {
	sabado := time.Date(2024, 1, 13, 0, 0, 0, 0, time.UTC) // sábado
	svc := NewMovimentoContabilService(&fakePosicaoRepo{}, &fakeRegraRepo{}, &fakeMovimentoRepoPaginado{}, evaluator.New())
	if _, err := svc.ReprocessarSeletivo(context.Background(), sabado, "NDF", "Posição", ""); err == nil {
		t.Fatal("esperava erro ao reprocessar em dia não útil")
	}
}
