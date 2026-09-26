package repository

import (
	"context"
	"database/sql"
)

// ParametrizacaoRepo persiste as opções parametrizáveis por categoria no SQL Server
// (tabela parametrizacao: categoria, valor). Ver migration 004_parametrizacao.sql.
type ParametrizacaoRepo struct {
	db *sql.DB
}

func NewParametrizacaoRepo(db *sql.DB) *ParametrizacaoRepo {
	return &ParametrizacaoRepo{db: db}
}

func (r *ParametrizacaoRepo) ListarOpcoes(ctx context.Context, categoria string) ([]string, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT valor FROM parametrizacao WHERE categoria = @p1 ORDER BY valor",
		categoria,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	opcoes := []string{}
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		opcoes = append(opcoes, v)
	}
	return opcoes, rows.Err()
}

func (r *ParametrizacaoRepo) AdicionarOpcao(ctx context.Context, categoria, valor string) error {
	// Evita duplicatas (categoria, valor).
	_, err := r.db.ExecContext(ctx,
		"IF NOT EXISTS (SELECT 1 FROM parametrizacao WHERE categoria = @p1 AND valor = @p2) INSERT INTO parametrizacao (categoria, valor) VALUES (@p1, @p2)",
		categoria, valor,
	)
	return err
}

func (r *ParametrizacaoRepo) RemoverOpcao(ctx context.Context, categoria, valor string) error {
	_, err := r.db.ExecContext(ctx,
		"DELETE FROM parametrizacao WHERE categoria = @p1 AND valor = @p2",
		categoria, valor,
	)
	return err
}
