package sqlite

import (
	"context"
	"database/sql"

	"srcoff/internal/model"
)

// CotacaoRepo implementa repository.CotacaoRepository em SQLite.
type CotacaoRepo struct{ db *sql.DB }

func NewCotacaoRepo(db *sql.DB) *CotacaoRepo { return &CotacaoRepo{db: db} }

func (r *CotacaoRepo) Listar(ctx context.Context) ([]model.Cotacao, error) {
	rows, err := r.db.QueryContext(ctx, "SELECT id, data, IFNULL(moeda1,''), IFNULL(moeda2,''), IFNULL(direcao,''), IFNULL(taxa,0) FROM cotacao ORDER BY data DESC, moeda1, moeda2")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Cotacao{}
	for rows.Next() {
		var c model.Cotacao
		var dataStr string
		if err := rows.Scan(&c.ID, &dataStr, &c.Moeda1, &c.Moeda2, &c.Direcao, &c.Taxa); err != nil {
			return nil, err
		}
		c.Data = parseData(dataStr)
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *CotacaoRepo) Criar(ctx context.Context, c model.Cotacao) (int64, error) {
	res, err := r.db.ExecContext(ctx, "INSERT INTO cotacao (data, moeda1, moeda2, direcao, taxa) VALUES (?,?,?,?,?)",
		fmtData(c.Data), c.Moeda1, c.Moeda2, c.Direcao, c.Taxa)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (r *CotacaoRepo) Excluir(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM cotacao WHERE id = ?", id)
	return err
}
