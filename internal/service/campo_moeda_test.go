package service

import (
	"context"
	"testing"

	"srcoff/internal/evaluator"
	"srcoff/internal/model"
	filerepo "srcoff/internal/repository/file"
)

// TestCampoMoeda_InconsistenciaAusenteOuBranco valida a RN-110.5:
// - campo_moeda referenciando campo inexistente na posicao -> inconsistencia CAMPO_MOEDA, sem lancamento (item 1);
// - campo_moeda existente mas em branco -> inconsistencia CAMPO_MOEDA, sem lancamento (item 2);
// - campo_moeda presente e preenchido -> gera normalmente (controle).
func TestCampoMoeda_InconsistenciaAusenteOuBranco(t *testing.T) {
	ctx := context.Background()
	eval := evaluator.New()
	d08 := d("2024-01-08")

	regra := model.RegraContabil{
		ID: 1, Descricao: "MTM", Ativo: true, TipoLancamento: model.TipoReverte,
		Condicoes: []model.CondicaoRegra{{
			ID: 1, IDRegra: 1, Condicao: "valor_mtm > 0",
			ContaDebito: "1001", ContaCredito: "2001",
			CampoValor: "valor_mtm", CampoMoeda: "moeda_mtm", CampoBoleto: "codigo_identificador_boleto", Ativo: true,
		}},
	}

	run := func(t *testing.T, pos model.PosicaoCarteira) (int, []model.InconsistenciaProcessamento) {
		t.Helper()
		dir := t.TempDir()
		movRepo := filerepo.NewMovimentoContabilRepo(dir)
		incRepo := filerepo.NewInconsistenciaRepo(dir)
		svc := NewMovimentoContabilService(&fakePosicaoRepo{registros: []model.PosicaoCarteira{pos}}, &fakeRegraRepo{regras: []model.RegraContabil{regra}}, movRepo, eval).
			ComInconsistenciaRepo(incRepo)
		if _, err := svc.GerarMovimentoEscopo(ctx, d08, "NDF", "Posição"); err != nil {
			t.Fatalf("gerar: %v", err)
		}
		ls, _ := movRepo.BuscarPorDataEIndicador(ctx, d08, false)
		inc, _ := incRepo.ListarPorData(ctx, d08)
		return len(ls), inc
	}

	posComCampos := func(campos map[string]interface{}) model.PosicaoCarteira {
		return model.PosicaoCarteira{ID: 1, DataPosicaoCarteira: d08, CodigoVersaoConteudo: 1, Campos: campos}
	}

	// Item 1: moeda_mtm ausente na posição (posD1 não tem esse campo).
	if n, inc := run(t, posD1(1, d08, "NDF", "Posição", "B1", 1000)); n != 0 || len(inc) != 1 || inc[0].Tipo != model.InconsistenciaCampoMoeda {
		t.Fatalf("item1 (ausente): esperava 0 lançamento e 1 inconsistência CAMPO_MOEDA; obteve n=%d inc=%+v", n, inc)
	}

	// Item 2: moeda_mtm existe porém em branco.
	branco := posComCampos(map[string]interface{}{
		"produto": "NDF", "dominio": "Posição", "codigo_identificador_boleto": "B1", "valor_mtm": 1000.0, "moeda_mtm": "",
	})
	if n, inc := run(t, branco); n != 0 || len(inc) != 1 || inc[0].Tipo != model.InconsistenciaCampoMoeda {
		t.Fatalf("item2 (branco): esperava 0 lançamento e 1 inconsistência CAMPO_MOEDA; obteve n=%d inc=%+v", n, inc)
	}

	// Controle: moeda preenchida → gera 1 lançamento, sem inconsistência.
	ok := posComCampos(map[string]interface{}{
		"produto": "NDF", "dominio": "Posição", "codigo_identificador_boleto": "B1", "valor_mtm": 1000.0, "moeda_mtm": "BRL",
	})
	if n, inc := run(t, ok); n != 1 || len(inc) != 0 {
		t.Fatalf("controle (preenchido): esperava 1 lançamento e 0 inconsistência; obteve n=%d inc=%+v", n, inc)
	}
}
