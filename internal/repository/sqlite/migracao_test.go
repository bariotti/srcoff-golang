package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"srcoff/internal/model"
)

// TestMigrarColunas_AdicionaOrigemEmBancoLegado simula um banco já existente cuja tabela
// movimento_contabil NÃO tem a coluna 'origem' e garante que Open() a adiciona (ALTER),
// permitindo o BulkInsert — reproduz e valida a correção do erro
// "table movimento_contabil has no column named origem".
func TestMigrarColunas_AdicionaOrigemEmBancoLegado(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legado.db")

	// 1. Cria um banco "legado": movimento_contabil SEM a coluna origem.
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open raw: %v", err)
	}
	_, err = raw.Exec(`CREATE TABLE movimento_contabil (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		data_lote_contabil TEXT NOT NULL,
		codigo_versao_conteudo INTEGER NOT NULL,
		codigo_identificador_boleto TEXT NOT NULL,
		valor_lancamento_contabil REAL NOT NULL,
		moeda_lancamento_contabil TEXT NOT NULL,
		conta_debito TEXT NOT NULL,
		conta_credito TEXT NOT NULL,
		produto TEXT, dominio TEXT,
		indicador_reversao INTEGER NOT NULL DEFAULT 0,
		descricao_regra_contabil TEXT, descricao_condicao_contabil TEXT,
		id_regra_contabil INTEGER
	)`)
	if err != nil {
		t.Fatalf("criar tabela legada: %v", err)
	}
	raw.Close()

	// 2. Abre pela aplicação — deve migrar (ADD COLUMN origem) sem apagar dados.
	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open (deveria migrar): %v", err)
	}
	defer db.Close()

	existe, err := colunaExiste(context.Background(), db, "movimento_contabil", "origem")
	if err != nil {
		t.Fatalf("colunaExiste: %v", err)
	}
	if !existe {
		t.Fatal("coluna origem deveria ter sido adicionada pela migração")
	}

	// 3. BulkInsert agora funciona com origem.
	repo := NewMovimentoContabilRepo(db)
	if err := repo.BulkInsert(context.Background(), []model.LancamentoContabil{{
		DataLoteContabil: time.Date(2024, 1, 8, 0, 0, 0, 0, time.UTC), CodigoVersaoConteudo: 1,
		CodigoIdentificadorBoleto: "M1", ValorLancamentoContabil: 100, MoedaLancamentoContabil: "USD",
		ContaDebito: "1001", ContaCredito: "2001", Produto: "NDF", Dominio: "Posição",
		Origem: model.OrigemManual,
	}}); err != nil {
		t.Fatalf("BulkInsert após migração: %v", err)
	}
	ls, err := repo.BuscarPorDataEIndicador(context.Background(), time.Date(2024, 1, 8, 0, 0, 0, 0, time.UTC), false)
	if err != nil || len(ls) != 1 || ls[0].Origem != model.OrigemManual {
		t.Fatalf("esperava 1 lançamento origem=manual, obteve %+v err=%v", ls, err)
	}
}
