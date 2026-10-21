package service

import (
	"context"
	"testing"

	"srcoff/internal/evaluator"
	"srcoff/internal/model"
	filerepo "srcoff/internal/repository/file"
)

// TestVeiculo_CopiadoDaPosicaoParaMovimento valida que o veículo da operação, definido no
// padrão de arquivo (coluna normalizada), é copiado da posição para o lançamento contábil
// e herdado pelo estorno. Spec: RF-034/RN-180.
func TestVeiculo_CopiadoDaPosicaoParaMovimento(t *testing.T) {
	ctx := context.Background()
	eval := evaluator.New()
	dir := t.TempDir()

	d08 := d("2024-01-08")
	d09 := d("2024-01-09") // dia útil seguinte: gera o estorno do lançamento de d08

	pos08 := model.PosicaoCarteira{
		ID: 1, DataPosicaoCarteira: d08, CodigoVersaoConteudo: 1,
		Campos: map[string]interface{}{
			"produto": "NDF", "dominio": "Posição",
			"codigo_identificador_boleto":  "B1",
			"valor_mtm":                    1000.0,
			"moeda_principal_remanescente": "USD",
			"veiculo_operacao":             "VH123", // coluna do veículo (já normalizada)
		},
	}
	// Mesma posição em d09 mantém o combo no escopo, para que o estorno de d08 seja gerado.
	pos09 := pos08
	pos09.ID = 2
	pos09.DataPosicaoCarteira = d09
	posRepo := &fakePosicaoRepo{registros: []model.PosicaoCarteira{pos08, pos09}}
	regraRepo := &fakeRegraRepo{regras: []model.RegraContabil{regraSimplesD1()}}
	movRepo := filerepo.NewMovimentoContabilRepo(dir)
	execRepo := filerepo.NewExecucaoRepo(dir)
	// Padrão informa a coluna do veículo com acento/maiúsculas — deve normalizar para "veiculo_operacao".
	padrao := &fakePadraoLookup{padroes: []model.PadraoArquivo{
		{Produto: "NDF", Dominio: "Posição", ColunaVeiculo: "Veículo Operação", ObrigatorioMovD1: boolPtr(false)},
	}}
	svc := NewMovimentoContabilService(posRepo, regraRepo, movRepo, eval).
		ComExecucaoRepo(execRepo).ComPadraoRepo(padrao)

	if _, err := svc.GerarMovimentoEscopo(ctx, d08, "", ""); err != nil {
		t.Fatalf("gerar d08: %v", err)
	}

	// O lançamento normal de d08 deve carregar o veículo copiado da posição.
	ls, _ := movRepo.BuscarPorDataEIndicador(ctx, d08, false)
	if len(ls) != 1 {
		t.Fatalf("esperava 1 lançamento em d08, obteve %d", len(ls))
	}
	if ls[0].Veiculo != "VH123" {
		t.Fatalf("veículo esperado VH123 no lançamento, obteve %q", ls[0].Veiculo)
	}

	// O estorno gerado em d09 (regra reverte) herda o veículo da origem.
	if _, err := svc.GerarMovimentoEscopo(ctx, d09, "", ""); err != nil {
		t.Fatalf("gerar d09: %v", err)
	}
	est, _ := movRepo.BuscarPorDataEIndicador(ctx, d09, true)
	if len(est) != 1 {
		t.Fatalf("esperava 1 estorno em d09, obteve %d", len(est))
	}
	if est[0].Veiculo != "VH123" {
		t.Fatalf("veículo esperado VH123 no estorno, obteve %q", est[0].Veiculo)
	}
}
