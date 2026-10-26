package sqlite

import (
	"path/filepath"
	"testing"
)

// TestIndicesMovimentoCriados garante que os indices que suportam as subqueries
// correlacionadas da consulta (vigente MAX de versao e saldo-zero) sao criados no Open.
func TestIndicesMovimentoCriados(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "idx.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	esperados := map[string]bool{
		"IX_mov_data_prod_dom_ver": false,
		"IX_mov_data_boleto_regra": false,
	}
	rows, err := db.Query("SELECT name FROM sqlite_master WHERE type='index' AND tbl_name='movimento_contabil'")
	if err != nil {
		t.Fatalf("query sqlite_master: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			t.Fatal(err)
		}
		if _, ok := esperados[n]; ok {
			esperados[n] = true
		}
	}
	for nome, ok := range esperados {
		if !ok {
			t.Fatalf("indice %s nao foi criado", nome)
		}
	}
}
