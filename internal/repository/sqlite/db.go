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
	// Migração leve de colunas em bancos JÁ existentes (CREATE TABLE IF NOT EXISTS não
	// altera tabela existente; o SQLite não tem "ADD COLUMN IF NOT EXISTS").
	if err := migrarColunas(context.Background(), db); err != nil {
		return nil, err
	}
	// Sem seed de opções padrão: os combos (Produto/Domínio) nascem vazios e são
	// cadastrados pelo usuário via Parametrizações (ADR-009, docs/especificacao.md §8.4).
	return db, nil
}

// migrarColunas garante colunas adicionadas ao longo da evolução do schema em bancos
// já existentes. Idempotente: só executa o ALTER quando a coluna não existe.
func migrarColunas(ctx context.Context, db *sql.DB) error {
	alteracoes := []struct{ tabela, coluna, ddl string }{
		{"movimento_contabil", "origem", "origem TEXT NOT NULL DEFAULT 'automatico'"},
		{"regra_contabil", "tipo_lancamento", "tipo_lancamento TEXT NOT NULL DEFAULT 'reverte'"},
		{"movimento_execucao", "qtd_movimento", "qtd_movimento INTEGER NOT NULL DEFAULT 0"},
	}
	for _, a := range alteracoes {
		existe, err := colunaExiste(ctx, db, a.tabela, a.coluna)
		if err != nil {
			return err
		}
		if !existe {
			if _, err := db.ExecContext(ctx, "ALTER TABLE "+a.tabela+" ADD COLUMN "+a.ddl); err != nil {
				return err
			}
		}
	}
	return nil
}

// colunaExiste consulta o PRAGMA table_info para saber se a coluna já existe na tabela.
func colunaExiste(ctx context.Context, db *sql.DB, tabela, coluna string) (bool, error) {
	rows, err := db.QueryContext(ctx, "PRAGMA table_info("+tabela+")")
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var cid, notnull, pk int
		var name, ctype string
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return false, err
		}
		if name == coluna {
			return true, nil
		}
	}
	return false, rows.Err()
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
  id_regra_contabil INTEGER,
  origem TEXT NOT NULL DEFAULT 'automatico'
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
CREATE TABLE IF NOT EXISTS saldo_conta (
  produto TEXT NOT NULL,
  dominio TEXT NOT NULL,
  codigo_identificador_boleto TEXT NOT NULL,
  id_regra_contabil INTEGER NOT NULL,
  conta_debito TEXT NOT NULL,
  conta_credito TEXT NOT NULL,
  saldo REAL NOT NULL DEFAULT 0,
  moeda_lancamento TEXT,
  descricao_regra TEXT,
  descricao_condicao TEXT,
  PRIMARY KEY (produto, dominio, codigo_identificador_boleto, id_regra_contabil, conta_debito, conta_credito)
);
CREATE INDEX IF NOT EXISTS IX_saldo_combo ON saldo_conta (produto, dominio);
`

// ── Helpers de data/hora (SQLite guarda como TEXT) ──────────────────────────

func fmtData(t time.Time) string { return t.Format("2006-01-02") }
func fmtTS(t time.Time) string   { return t.Format("2006-01-02 15:04:05") }

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
