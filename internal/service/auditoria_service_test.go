package service

import (
	"context"
	"path/filepath"
	"testing"

	"srcoff/internal/model"
	sqliterepo "srcoff/internal/repository/sqlite"
)

type fakeAuditRepo struct{ eventos []model.EventoAuditoria }

func (f *fakeAuditRepo) Inserir(_ context.Context, ev model.EventoAuditoria) error {
	f.eventos = append(f.eventos, ev)
	return nil
}
func (f *fakeAuditRepo) Listar(_ context.Context, _ model.FiltroAuditoria) (*model.PaginaAuditoria, error) {
	return &model.PaginaAuditoria{Total: len(f.eventos), Eventos: f.eventos}, nil
}

// TestAuditoriaRegistraCadastro garante que uma ação de cadastro emite um evento de auditoria
// com a categoria/ação corretas (hook ligado via ComAuditoria).
func TestAuditoriaRegistraCadastro(t *testing.T) {
	db, err := sqliterepo.Open(filepath.Join(t.TempDir(), "aud_svc.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()
	ctx := context.Background()
	far := &fakeAuditRepo{}
	aud := NewAuditoriaService(far)
	svc := NewRegraContabilService(sqliterepo.NewRegraContabilRepo(db)).ComAuditoria(aud)

	if _, err := svc.CriarRegra(ctx, model.RegraContabil{Descricao: "Regra X"}); err != nil {
		t.Fatalf("CriarRegra: %v", err)
	}
	if len(far.eventos) != 1 {
		t.Fatalf("esperava 1 evento de auditoria, veio %d", len(far.eventos))
	}
	ev := far.eventos[0]
	if ev.Categoria != model.AudCadastro || ev.Acao != "regra.criada" {
		t.Fatalf("evento inesperado: cat=%q acao=%q", ev.Categoria, ev.Acao)
	}
	if ev.CriadoEm == "" || ev.Origem != "sistema" {
		t.Fatalf("evento sem timestamp/origem: %+v", ev)
	}
}

// TestAuditoriaOff: sem repositório, a trilha fica indisponível e Listar falha, mas Registrar
// (linha de log) não quebra.
func TestAuditoriaOff(t *testing.T) {
	aud := NewAuditoriaService(nil)
	if aud.Disponivel() {
		t.Fatal("sem repo, auditoria deveria estar off")
	}
	aud.Log(context.Background(), model.AudGeracao, "x", "y", "sistema", "z", nil) // não deve panicar
	if _, err := aud.Listar(context.Background(), model.FiltroAuditoria{}); err == nil {
		t.Fatal("Listar deveria falhar com auditoria off")
	}
	// Serviço nil também é seguro.
	var nilSvc *AuditoriaService
	nilSvc.Registrar(context.Background(), model.EventoAuditoria{Acao: "ignore"})
}
