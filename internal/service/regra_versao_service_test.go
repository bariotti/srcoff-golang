package service

import (
	"context"
	"path/filepath"
	"testing"

	"srcoff/internal/model"
	sqliterepo "srcoff/internal/repository/sqlite"
)

// TestHistoricoRegraFluxo cobre o versionamento do cadastro (RF-133) fim-a-fim sobre o SQLite:
// cada alteração salva gera uma versão; a restauração ressuscita condições excluídas depois,
// remove as criadas depois, e é append-only (anexa "Revertido para vN").
func TestHistoricoRegraFluxo(t *testing.T) {
	db, err := sqliterepo.Open(filepath.Join(t.TempDir(), "svc_hist.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()
	ctx := context.Background()
	repo := sqliterepo.NewRegraContabilRepo(db)
	vrepo := sqliterepo.NewRegraVersaoRepo(db)
	svc := NewRegraContabilService(repo).ComVersaoRepo(vrepo)

	if !svc.HistoricoDisponivel() {
		t.Fatal("histórico deveria estar disponível (sqlite)")
	}

	// v1: criar regra.
	id, err := svc.CriarRegra(ctx, model.RegraContabil{Descricao: "Regra teste"})
	if err != nil {
		t.Fatalf("CriarRegra: %v", err)
	}
	cond := func(expr string) model.CondicaoRegra {
		return model.CondicaoRegra{IDRegra: id, Condicao: expr, ContaDebito: "100", ContaCredito: "200", CampoValor: "valor", CampoMoeda: "moeda",
			BaseNaPosicao: true, CampoValorMoedaBase: "valor_base", CampoMoedaBase: "moeda_base"}
	}
	// v2: condição A.
	idA, err := svc.CriarCondicao(ctx, cond("valor > 0"))
	if err != nil {
		t.Fatalf("CriarCondicao A: %v", err)
	}
	// v3: condição B.
	idB, err := svc.CriarCondicao(ctx, cond("valor < 0"))
	if err != nil {
		t.Fatalf("CriarCondicao B: %v", err)
	}
	// v4: exclui B.
	if err := svc.ExcluirCondicao(ctx, idB); err != nil {
		t.Fatalf("ExcluirCondicao B: %v", err)
	}

	hist, err := svc.Historico(ctx, id)
	if err != nil {
		t.Fatalf("Historico: %v", err)
	}
	if len(hist) != 4 {
		t.Fatalf("esperava 4 versões (criar, +A, +B, -B), veio %d: %+v", len(hist), resumos(hist))
	}
	if hist[0].Resumo != "Condição excluída" {
		t.Fatalf("v4 deveria ser 'Condição excluída', veio %q", hist[0].Resumo)
	}

	// Estado atual: só A visível.
	if conds, _ := svc.ListarCondicoes(ctx, id); len(conds) != 1 || conds[0].ID != idA {
		t.Fatalf("antes da restauração deveria haver só A visível, veio %+v", conds)
	}

	// Restaura v3 (A e B ativas) → B ressuscita; nova versão v5.
	if err := svc.RestaurarVersao(ctx, id, 3); err != nil {
		t.Fatalf("RestaurarVersao 3: %v", err)
	}
	conds, _ := svc.ListarCondicoes(ctx, id)
	if len(conds) != 2 {
		t.Fatalf("após restaurar v3 deveria haver 2 condições (A e B ressuscitada), veio %+v", conds)
	}
	hist, _ = svc.Historico(ctx, id)
	if len(hist) != 5 || hist[0].Resumo != "Revertido para v3" {
		t.Fatalf("v5 deveria ser 'Revertido para v3', veio len=%d %q", len(hist), hist[0].Resumo)
	}

	// Cria condição C (v6), depois restaura v2 (só A) → B e C devem sair.
	idC, err := svc.CriarCondicao(ctx, cond("valor == 0"))
	if err != nil {
		t.Fatalf("CriarCondicao C: %v", err)
	}
	if err := svc.RestaurarVersao(ctx, id, 2); err != nil {
		t.Fatalf("RestaurarVersao 2: %v", err)
	}
	conds, _ = svc.ListarCondicoes(ctx, id)
	if len(conds) != 1 || conds[0].ID != idA {
		t.Fatalf("após restaurar v2 deveria sobrar só A; B(%d) e C(%d) excluídas; veio %+v", idB, idC, conds)
	}
	hist, _ = svc.Historico(ctx, id)
	if hist[0].Resumo != "Revertido para v2" {
		t.Fatalf("última versão deveria ser 'Revertido para v2', veio %q", hist[0].Resumo)
	}
}

// TestHistoricoOffSemRepo garante que, sem repositório de versões (backends file/sqlserver), o
// cadastro funciona e a feature responde como indisponível.
func TestHistoricoOffSemRepo(t *testing.T) {
	db, err := sqliterepo.Open(filepath.Join(t.TempDir(), "svc_off.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()
	ctx := context.Background()
	svc := NewRegraContabilService(sqliterepo.NewRegraContabilRepo(db)) // sem ComVersaoRepo

	if svc.HistoricoDisponivel() {
		t.Fatal("sem repo de versões, histórico deveria estar off")
	}
	id, err := svc.CriarRegra(ctx, model.RegraContabil{Descricao: "R"})
	if err != nil {
		t.Fatalf("CriarRegra deveria funcionar mesmo com histórico off: %v", err)
	}
	if _, err := svc.Historico(ctx, id); err == nil {
		t.Fatal("Historico deveria falhar quando a feature está off")
	}
}

// TestRestaurarParaVersaoSemCondicoes cobre o caso em que a versão-alvo não tinha NENHUMA
// condição: restaurar deve EXCLUIR a condição criada depois.
func TestRestaurarParaVersaoSemCondicoes(t *testing.T) {
	db, err := sqliterepo.Open(filepath.Join(t.TempDir(), "svc_zero.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()
	ctx := context.Background()
	repo := sqliterepo.NewRegraContabilRepo(db)
	vrepo := sqliterepo.NewRegraVersaoRepo(db)
	svc := NewRegraContabilService(repo).ComVersaoRepo(vrepo)

	id, _ := svc.CriarRegra(ctx, model.RegraContabil{Descricao: "R"}) // v1 (0 cond)
	// v2: edita a regra (ainda 0 condições) — replica o fluxo HTTP.
	if err := svc.EditarRegra(ctx, model.RegraContabil{ID: id, Descricao: "R", PreCondicao: "a > 0", TipoLancamento: "reverte", Status: "ativa"}); err != nil {
		t.Fatalf("EditarRegra: %v", err)
	}
	// v3: adiciona condição.
	_, err = svc.CriarCondicao(ctx, model.CondicaoRegra{IDRegra: id, Condicao: "valor > 0", ContaDebito: "1", ContaCredito: "2", CampoValor: "valor", CampoMoeda: "moeda", BaseNaPosicao: true, CampoValorMoedaBase: "vb", CampoMoedaBase: "mb"})
	if err != nil {
		t.Fatalf("CriarCondicao: %v", err)
	}

	if err := svc.RestaurarVersao(ctx, id, 2); err != nil {
		t.Fatalf("RestaurarVersao 2: %v", err)
	}
	conds, _ := svc.ListarCondicoes(ctx, id)
	if len(conds) != 0 {
		t.Fatalf("restaurar v1 (sem condições) deveria excluir a condição criada depois; veio %d visíveis", len(conds))
	}
}

func resumos(vs []model.RegraVersao) []string {
	out := make([]string, len(vs))
	for i, v := range vs {
		out[i] = v.Resumo
	}
	return out
}
