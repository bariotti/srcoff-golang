package sqlite

import (
	"context"
	"path/filepath"
	"testing"

	"srcoff/internal/model"
)

func TestRegraVersaoRepo(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "hist.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()
	ctx := context.Background()
	regraRepo := NewRegraContabilRepo(db)
	vRepo := NewRegraVersaoRepo(db)

	id, err := regraRepo.CriarRegra(ctx, model.RegraContabil{Descricao: "R1", Status: model.StatusAtiva})
	if err != nil {
		t.Fatalf("CriarRegra: %v", err)
	}

	// v1: regra sem condições.
	atual, ok, err := vRepo.Carregar(ctx, id)
	if err != nil || !ok {
		t.Fatalf("Carregar: %v ok=%v", err, ok)
	}
	if len(atual.Condicoes) != 0 {
		t.Fatalf("v1 deveria ter 0 condições, veio %d", len(atual.Condicoes))
	}
	if n, err := vRepo.Snapshot(ctx, id, atual, "Regra criada", "sistema"); err != nil || n != 1 {
		t.Fatalf("Snapshot v1: n=%d err=%v", n, err)
	}

	// v2: adiciona uma condição e tira novo snapshot.
	if _, err := regraRepo.CriarCondicao(ctx, model.CondicaoRegra{IDRegra: id, Condicao: "valor > 0", ContaDebito: "1", ContaCredito: "2", CampoValor: "valor", CampoMoeda: "moeda", Status: model.StatusAtiva}); err != nil {
		t.Fatalf("CriarCondicao: %v", err)
	}
	atual2, _, _ := vRepo.Carregar(ctx, id)
	if len(atual2.Condicoes) != 1 {
		t.Fatalf("v2 deveria ter 1 condição, veio %d", len(atual2.Condicoes))
	}
	if n, err := vRepo.Snapshot(ctx, id, atual2, "Condição adicionada", "sistema"); err != nil || n != 2 {
		t.Fatalf("Snapshot v2: n=%d err=%v", n, err)
	}

	// Listar: ordem decrescente, a maior (v2) é a ativa.
	vs, err := vRepo.Listar(ctx, id)
	if err != nil || len(vs) != 2 {
		t.Fatalf("Listar: %d versões err=%v", len(vs), err)
	}
	if vs[0].Versao != 2 || !vs[0].Ativa {
		t.Fatalf("v2 deveria vir primeiro e ser ativa, veio %+v", vs[0])
	}
	if vs[1].Versao != 1 || vs[1].Ativa {
		t.Fatalf("v1 deveria vir por último e NÃO ser ativa, veio %+v", vs[1])
	}

	// Obter: o snapshot de cada versão guarda as condições daquele instante.
	g1, ok, _ := vRepo.Obter(ctx, id, 1)
	if !ok || len(g1.Regra.Condicoes) != 0 {
		t.Fatalf("Obter v1: ok=%v conds=%d", ok, len(g1.Regra.Condicoes))
	}
	g2, _, _ := vRepo.Obter(ctx, id, 2)
	if len(g2.Regra.Condicoes) != 1 {
		t.Fatalf("Obter v2: conds=%d", len(g2.Regra.Condicoes))
	}

	// UltimaVersao e ResumoTodos.
	u, ok, _ := vRepo.UltimaVersao(ctx, id)
	if !ok || u.Versao != 2 {
		t.Fatalf("UltimaVersao: ok=%v v=%d", ok, u.Versao)
	}
	res, err := vRepo.ResumoTodos(ctx)
	if err != nil || len(res) != 1 || res[0].IDRegra != id || res[0].Versao != 2 {
		t.Fatalf("ResumoTodos: %+v err=%v", res, err)
	}
}
