package repository

import (
	"context"
	"database/sql"
)

// ConfiguracaoRepo persiste configurações chave→valor no SQL Server.
// Ver migration 006_padrao_arquivo.sql.
type ConfiguracaoRepo struct {
	db *sql.DB
}

func NewConfiguracaoRepo(db *sql.DB) *ConfiguracaoRepo {
	return &ConfiguracaoRepo{db: db}
}

func (r *ConfiguracaoRepo) Obter(ctx context.Context, chave string) (string, error) {
	var valor sql.NullString
	err := r.db.QueryRowContext(ctx, "SELECT valor FROM configuracao WHERE chave = @p1", chave).Scan(&valor)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return valor.String, nil
}

func (r *ConfiguracaoRepo) Definir(ctx context.Context, chave, valor string) error {
	_, err := r.db.ExecContext(ctx,
		`MERGE configuracao AS alvo
		 USING (SELECT @p1 AS chave, @p2 AS valor) AS origem
		 ON alvo.chave = origem.chave
		 WHEN MATCHED THEN UPDATE SET valor = origem.valor
		 WHEN NOT MATCHED THEN INSERT (chave, valor) VALUES (origem.chave, origem.valor);`,
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
