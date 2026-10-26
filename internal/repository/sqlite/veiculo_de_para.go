package sqlite

import (
	"context"
	"database/sql"

	"srcoff/internal/model"
)

// VeiculoDeParaRepo implementa repository.VeiculoDeParaRepository em SQLite.
type VeiculoDeParaRepo struct{ db *sql.DB }

func NewVeiculoDeParaRepo(db *sql.DB) *VeiculoDeParaRepo { return &VeiculoDeParaRepo{db: db} }

func (r *VeiculoDeParaRepo) Listar(ctx context.Context) ([]model.VeiculoDePara, error) {
	rows, err := r.db.QueryContext(ctx, "SELECT id, IFNULL(veiculo,''), IFNULL(codigo,''), IFNULL(moeda_base,'') FROM veiculo_de_para ORDER BY veiculo")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.VeiculoDePara{}
	for rows.Next() {
		var v model.VeiculoDePara
		if err := rows.Scan(&v.ID, &v.Veiculo, &v.Codigo, &v.MoedaBase); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (r *VeiculoDeParaRepo) Criar(ctx context.Context, v model.VeiculoDePara) (int64, error) {
	res, err := r.db.ExecContext(ctx, "INSERT INTO veiculo_de_para (veiculo, codigo, moeda_base) VALUES (?,?,?)", v.Veiculo, v.Codigo, v.MoedaBase)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (r *VeiculoDeParaRepo) Excluir(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM veiculo_de_para WHERE id = ?", id)
	return err
}
