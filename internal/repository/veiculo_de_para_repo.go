package repository

import (
	"context"
	"database/sql"

	"srcoff/internal/model"
)

// VeiculoDeParaRepo persiste o De/Para de veículo → código no SQL Server (RF-035).
type VeiculoDeParaRepo struct {
	db *sql.DB
}

func NewVeiculoDeParaRepo(db *sql.DB) *VeiculoDeParaRepo { return &VeiculoDeParaRepo{db: db} }

func (r *VeiculoDeParaRepo) Listar(ctx context.Context) ([]model.VeiculoDePara, error) {
	rows, err := r.db.QueryContext(ctx, "SELECT id, ISNULL(veiculo,''), ISNULL(codigo,''), ISNULL(moeda_base,'') FROM veiculo_de_para ORDER BY veiculo")
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
	var id int64
	err := r.db.QueryRowContext(ctx,
		"INSERT INTO veiculo_de_para (veiculo, codigo, moeda_base) VALUES (@p1, @p2, @p3); SELECT SCOPE_IDENTITY()",
		v.Veiculo, v.Codigo, v.MoedaBase,
	).Scan(&id)
	return id, err
}

func (r *VeiculoDeParaRepo) Excluir(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM veiculo_de_para WHERE id = @p1", id)
	return err
}
