// Package sqlite implementa os repositórios usando SQLite (driver pure-Go
// modernc.org/sqlite) como backend de persistência. É um terceiro backend ao lado
// de "file" e "sqlserver", selecionável por STORAGE_BACKEND=sqlite.
//
// Diferenças de dialeto em relação ao SQL Server: placeholders "?" (posicionais),
// IFNULL no lugar de ISNULL, LIMIT/OFFSET no lugar de OFFSET..FETCH, AUTOINCREMENT +
// LastInsertId no lugar de SCOPE_IDENTITY, UPSERT (ON CONFLICT) no lugar de MERGE.
// Datas são gravadas como TEXT ISO e convertidas de/para time.Time em Go.
package sqlite

import (
	"context"
	"database/sql"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// Open abre (criando se necessário) o banco SQLite no caminho informado e garante o schema.
// O caminho é passado como DSN simples (sem prefixo "file:") para evitar problemas de URI
// com caminhos Windows (backslashes e "C:").
func Open(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	// Uma única conexão evita "database is locked" em escritas concorrentes no SQLite.
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		return nil, err
	}
	if _, err := db.Exec("PRAGMA busy_timeout=5000"); err != nil {
		return nil, err
	}
	if err := CriarSchema(context.Background(), db); err != nil {
		return nil, err
	}
	if err := seedDefaults(context.Background(), db); err != nil {
		return nil, err
	}
	return db, nil
}

// seedDefaults insere as opções padrão dos combos (Produto/Domínio), em paridade com o
// backend de arquivo e com a migration 004 do SQL Server. Idempotente (índice único).
func seedDefaults(ctx context.Context, db *sql.DB) error {
	defaults := []struct{ cat, val string }{
		{"produto", "NDF"}, {"produto", "SWAP"}, {"produto", "FXO"},
		{"dominio", "Posição"}, {"dominio", "Liquidação"},
	}
	for _, d := range defaults {
		if _, err := db.ExecContext(ctx, "INSERT OR IGNORE INTO parametrizacao (categoria, valor) VALUES (?, ?)", d.cat, d.val); err != nil {
			return err
		}
	}
	return nil
}

// CriarSchema cria todas as tabelas/índices (idempotente). Executa cada statement
// separadamente, pois o database/sql roda um comando por Exec.
func CriarSchema(ctx context.Context, db *sql.DB) error {
	for _, stmt := range strings.Split(schema, ";") {
		s := strings.TrimSpace(stmt)
		if s == "" {
			continue
		}
		if _, err := db.ExecContext(ctx, s); err != nil {
			return err
		}
	}
	return nil
}

const schema = `
CREATE TABLE IF NOT EXISTS regra_contabil (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  descricao TEXT NOT NULL,
  codigo_produto_corporativo TEXT,
  dominio TEXT,
  campo_produto TEXT,
  pre_condicao TEXT,
  ativo INTEGER NOT NULL DEFAULT 1,
  tipo_lancamento TEXT NOT NULL DEFAULT 'reverte'
);
CREATE TABLE IF NOT EXISTS condicao_regra (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  id_regra INTEGER NOT NULL,
  condicao TEXT NOT NULL,
  conta_debito TEXT NOT NULL,
  conta_credito TEXT NOT NULL,
  campo_valor TEXT NOT NULL,
  campo_moeda TEXT NOT NULL,
  campo_boleto TEXT,
  ativo INTEGER NOT NULL DEFAULT 1
);
CREATE TABLE IF NOT EXISTS posicao_carteira (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  data_posicao_carteira TEXT NOT NULL,
  codigo_versao_conteudo INTEGER NOT NULL,
  campos TEXT
);
CREATE INDEX IF NOT EXISTS IX_posicao_data_versao ON posicao_carteira (data_posicao_carteira, codigo_versao_conteudo);
CREATE TABLE IF NOT EXISTS movimento_contabil (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  data_lote_contabil TEXT NOT NULL,
  codigo_versao_conteudo INTEGER NOT NULL,
  codigo_identificador_boleto TEXT NOT NULL,
  valor_lancamento_contabil REAL NOT NULL,
  moeda_lancamento_contabil TEXT NOT NULL,
  conta_debito TEXT NOT NULL,
  conta_credito TEXT NOT NULL,
  produto TEXT,
  dominio TEXT,
  indicador_reversao INTEGER NOT NULL DEFAULT 0,
  descricao_regra_contabil TEXT,
  descricao_condicao_contabil TEXT,
  id_regra_contabil INTEGER
);
CREATE INDEX IF NOT EXISTS IX_movimento_data_versao ON movimento_contabil (data_lote_contabil, codigo_versao_conteudo);
CREATE INDEX IF NOT EXISTS IX_movimento_data_reversao ON movimento_contabil (data_lote_contabil, indicador_reversao);
CREATE TABLE IF NOT EXISTS parametrizacao (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  categoria TEXT NOT NULL,
  valor TEXT NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS UX_param_categoria_valor ON parametrizacao (categoria, valor);
CREATE TABLE IF NOT EXISTS inconsistencia_processamento (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  data_lote_contabil TEXT NOT NULL,
  codigo_identificador_boleto TEXT,
  produto TEXT,
  dominio TEXT,
  id_regra_contabil INTEGER,
  descricao_regra_contabil TEXT,
  tipo TEXT,
  expressao TEXT,
  campos_faltantes TEXT,
  detalhe TEXT,
  criado_em TEXT
);
CREATE TABLE IF NOT EXISTS padrao_arquivo (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  padrao TEXT NOT NULL,
  produto TEXT NOT NULL,
  dominio TEXT,
  delimitador TEXT,
  separador_decimal TEXT,
  separador_milhar TEXT,
  formato_data TEXT,
  coluna_data TEXT,
  coluna_boleto TEXT,
  obrigatorio_mov_d1 INTEGER
);
CREATE TABLE IF NOT EXISTS configuracao (
  chave TEXT PRIMARY KEY,
  valor TEXT
);
CREATE TABLE IF NOT EXISTS movimento_execucao (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  data_lote TEXT NOT NULL,
  produto TEXT NOT NULL,
  dominio TEXT NOT NULL,
  qtd_lancamentos INTEGER NOT NULL DEFAULT 0,
  qtd_estornos INTEGER NOT NULL DEFAULT 0,
  qtd_movimento INTEGER NOT NULL DEFAULT 0,
  criado_em TEXT
);
CREATE TABLE IF NOT EXISTS notificacao (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  tipo TEXT,
  data_lote TEXT,
  produto TEXT,
  dominio TEXT,
  mensagem TEXT,
  lida INTEGER NOT NULL DEFAULT 0,
  criado_em TEXT
);
`

// ── Helpers de data/hora (SQLite guarda como TEXT) ──────────────────────────

func fmtData(t time.Time) string { return t.Format("2006-01-02") }
func fmtTS(t time.Time) string    { return t.Format("2006-01-02 15:04:05") }

// parseData converte "YYYY-MM-DD..." em time.Time (apenas a data).
func parseData(s string) time.Time {
	if len(s) >= 10 {
		s = s[:10]
	}
	t, _ := time.Parse("2006-01-02", s)
	return t
}

// parseTS converte um timestamp textual em time.Time, tolerando formatos comuns.
func parseTS(s string) time.Time {
	for _, l := range []string{"2006-01-02 15:04:05", "2006-01-02T15:04:05Z", time.RFC3339, "2006-01-02"} {
		if t, err := time.Parse(l, s); err == nil {
			return t
		}
	}
	return time.Time{}
}
