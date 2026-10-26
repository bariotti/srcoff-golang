package repository

import (
	"context"
	"database/sql"

	"srcoff/internal/model"
)

// CotacaoRepo persiste as taxas de conversão entre moedas por data no SQL Server (RF-037).
type CotacaoRepo struct {
	db *sql.DB
}

func NewCotacaoRepo(db *sql.DB) *CotacaoRepo { return &CotacaoRepo{db: db} }

func (r *CotacaoRepo) Listar(ctx context.Context) ([]model.Cotacao, error) {
	rows, err := r.db.QueryContext(ctx, "SELECT id, data, ISNULL(moeda1,''), ISNULL(moeda2,''), ISNULL(direcao,''), ISNULL(taxa,0) FROM cotacao ORDER BY data DESC, moeda1, moeda2")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Cotacao{}
	for rows.Next() {
		var c model.Cotacao
		if err := rows.Scan(&c.ID, &c.Data, &c.Moeda1, &c.Moeda2, &c.Direcao, &c.Taxa); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *CotacaoRepo) Criar(ctx context.Context, c model.Cotacao) (int64, error) {
	var id int64
	err := r.db.QueryRowContext(ctx,
		"INSERT INTO cotacao (data, moeda1, moeda2, direcao, taxa) VALUES (@p1, @p2, @p3, @p4, @p5); SELECT SCOPE_IDENTITY()",
		c.Data.Format("2006-01-02"), c.Moeda1, c.Moeda2, c.Direcao, c.Taxa,
	).Scan(&id)
	return id, err
}

func (r *CotacaoRepo) Excluir(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM cotacao WHERE id = @p1", id)
	return err
}
