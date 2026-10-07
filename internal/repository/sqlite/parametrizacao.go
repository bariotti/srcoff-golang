package sqlite

import (
	"context"
	"database/sql"
)

// ParametrizacaoRepo implementa repository.ParametrizacaoRepository em SQLite.
type ParametrizacaoRepo struct{ db *sql.DB }

func NewParametrizacaoRepo(db *sql.DB) *ParametrizacaoRepo { return &ParametrizacaoRepo{db: db} }

func (r *ParametrizacaoRepo) ListarOpcoes(ctx context.Context, categoria string) ([]string, error) {
	rows, err := r.db.QueryContext(ctx, "SELECT valor FROM parametrizacao WHERE categoria = ? ORDER BY valor", categoria)
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
	// Índice único (categoria, valor) garante a ausência de duplicatas.
	_, err := r.db.ExecContext(ctx, "INSERT OR IGNORE INTO parametrizacao (categoria, valor) VALUES (?, ?)", categoria, valor)
	return err
}

func (r *ParametrizacaoRepo) RemoverOpcao(ctx context.Context, categoria, valor string) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM parametrizacao WHERE categoria = ? AND valor = ?", categoria, valor)
	return err
}
