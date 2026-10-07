package sqlite

import (
	"context"
	"database/sql"
)

// ConfiguracaoRepo implementa repository.ConfiguracaoRepository em SQLite.
type ConfiguracaoRepo struct{ db *sql.DB }

func NewConfiguracaoRepo(db *sql.DB) *ConfiguracaoRepo { return &ConfiguracaoRepo{db: db} }

func (r *ConfiguracaoRepo) Obter(ctx context.Context, chave string) (string, error) {
	var valor sql.NullString
	err := r.db.QueryRowContext(ctx, "SELECT valor FROM configuracao WHERE chave = ?", chave).Scan(&valor)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return valor.String, nil
}

func (r *ConfiguracaoRepo) Definir(ctx context.Context, chave, valor string) error {
	// UPSERT (chave é PK).
	_, err := r.db.ExecContext(ctx,
		"INSERT INTO configuracao (chave, valor) VALUES (?, ?) ON CONFLICT(chave) DO UPDATE SET valor = excluded.valor",
		chave, valor,
	)
	return err
}

func (r *ConfiguracaoRepo) ListarTodas(ctx context.Context) (map[string]string, error) {
	rows, err := r.db.QueryContext(ctx, "SELECT chave, valor FROM configuracao")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := map[string]string{}
	for rows.Next() {
		var k string
		var v sql.NullString
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		m[k] = v.String
	}
	return m, rows.Err()
}
