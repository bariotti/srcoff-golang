package sqlite

import (
	"context"
	"path/filepath"
	"testing"

	"srcoff/internal/model"
)

func TestAuditoriaRepo(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "aud.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()
	ctx := context.Background()
	r := NewAuditoriaRepo(db)

	if err := r.Inserir(ctx, model.EventoAuditoria{CriadoEm: "2026-11-10 10:00:00", Categoria: "cadastro", Acao: "regra.criada", Entidade: "regra #1", Origem: "sistema", Detalhe: "nova", Dados: map[string]interface{}{"id": 1}}); err != nil {
		t.Fatalf("Inserir 1: %v", err)
	}
	if err := r.Inserir(ctx, model.EventoAuditoria{CriadoEm: "2026-11-10 11:00:00", Categoria: "geracao", Acao: "movimento.gerado", Entidade: "NDF", Origem: "sistema"}); err != nil {
		t.Fatalf("Inserir 2: %v", err)
	}

	pag, err := r.Listar(ctx, model.FiltroAuditoria{})
	if err != nil || pag.Total != 2 || len(pag.Eventos) != 2 {
		t.Fatalf("Listar tudo: total=%d len=%d err=%v", pag.Total, len(pag.Eventos), err)
	}
	if pag.Eventos[0].Acao != "movimento.gerado" {
		t.Fatalf("mais recente deveria vir primeiro, veio %q", pag.Eventos[0].Acao)
	}

	pagCad, _ := r.Listar(ctx, model.FiltroAuditoria{Categoria: "cadastro"})
	if pagCad.Total != 1 || pagCad.Eventos[0].Entidade != "regra #1" {
		t.Fatalf("filtro categoria: %+v", pagCad)
	}
	if pagCad.Eventos[0].Dados["id"] != float64(1) {
		t.Fatalf("dados deveriam desserializar (id=1), veio %v", pagCad.Eventos[0].Dados["id"])
	}

	pagTexto, _ := r.Listar(ctx, model.FiltroAuditoria{Texto: "gerado"})
	if pagTexto.Total != 1 || pagTexto.Eventos[0].Categoria != "geracao" {
		t.Fatalf("filtro texto: %+v", pagTexto)
	}

	pagData, _ := r.Listar(ctx, model.FiltroAuditoria{De: "2026-11-10", Ate: "2026-11-10"})
	if pagData.Total != 2 {
		t.Fatalf("filtro data (dia inteiro) deveria pegar 2, veio %d", pagData.Total)
	}
}
