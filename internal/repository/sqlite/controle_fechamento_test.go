package sqlite

import (
	"context"
	"path/filepath"
	"testing"

	"srcoff/internal/model"
)

func TestControleFechamentoRepo(t *testing.T) {
	ctx := context.Background()
	db, err := Open(filepath.Join(t.TempDir(), "fech.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()
	repo := NewControleFechamentoRepo(db)

	// Estado inicial: modo off, sem escopos/liberações.
	cfg, err := repo.Carregar(ctx)
	if err != nil {
		t.Fatalf("Carregar: %v", err)
	}
	if cfg.Modo != model.FechamentoOff || len(cfg.Escopos) != 0 || len(cfg.Liberacoes) != 0 {
		t.Fatalf("estado inicial inesperado: %+v", cfg)
	}

	// Define modo selecionados + escopos (ignora duplicata).
	if err := repo.DefinirModo(ctx, model.FechamentoSelecionados); err != nil {
		t.Fatalf("DefinirModo: %v", err)
	}
	if err := repo.AdicionarEscopo(ctx, "NDF", "Posicao"); err != nil {
		t.Fatalf("AdicionarEscopo: %v", err)
	}
	if err := repo.AdicionarEscopo(ctx, "NDF", "Posicao"); err != nil {
		t.Fatalf("AdicionarEscopo dup: %v", err)
	}
	cfg, _ = repo.Carregar(ctx)
	if cfg.Modo != model.FechamentoSelecionados || len(cfg.Escopos) != 1 {
		t.Fatalf("esperava modo selecionados com 1 escopo: %+v", cfg)
	}

	// Libera uma competência (ignora duplicata) e confere.
	if err := repo.Liberar(ctx, 2026, 11, "NDF", "Posicao"); err != nil {
		t.Fatalf("Liberar: %v", err)
	}
	if err := repo.Liberar(ctx, 2026, 11, "NDF", "Posicao"); err != nil {
		t.Fatalf("Liberar dup: %v", err)
	}
	cfg, _ = repo.Carregar(ctx)
	if len(cfg.Liberacoes) != 1 || !cfg.Liberado(2026, 11, "NDF", "Posicao") {
		t.Fatalf("esperava 1 liberação válida: %+v", cfg.Liberacoes)
	}

	// Remove escopo e liberação (ids vêm do próprio estado).
	if err := repo.RemoverLiberacao(ctx, cfg.Liberacoes[0].ID); err != nil {
		t.Fatalf("RemoverLiberacao: %v", err)
	}
	if err := repo.RemoverEscopo(ctx, cfg.Escopos[0].ID); err != nil {
		t.Fatalf("RemoverEscopo: %v", err)
	}
	cfg, _ = repo.Carregar(ctx)
	if len(cfg.Escopos) != 0 || len(cfg.Liberacoes) != 0 {
		t.Fatalf("esperava escopos/liberações vazios após remoção: %+v", cfg)
	}
}
