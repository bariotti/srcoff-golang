package repository

import (
	"context"
	"database/sql"
	"time"

	"srcoff/internal/model"
)

// ExecucaoRepo persiste o log de execução do contábil no SQL Server.
type ExecucaoRepo struct {
	db *sql.DB
}

func NewExecucaoRepo(db *sql.DB) *ExecucaoRepo {
	return &ExecucaoRepo{db: db}
}

func (r *ExecucaoRepo) RegistrarExecucao(ctx context.Context, e model.MovimentoExecucao) error {
	dataStr := e.DataLote.Format("2006-01-02")
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx,
		"DELETE FROM movimento_execucao WHERE data_lote = @p1 AND produto = @p2 AND dominio = @p3",
		dataStr, e.Produto, e.Dominio,
	); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		"INSERT INTO movimento_execucao (data_lote, produto, dominio, qtd_lancamentos, qtd_estornos, criado_em) VALUES (@p1,@p2,@p3,@p4,@p5,@p6)",
		dataStr, e.Produto, e.Dominio, e.QtdLancamentos, e.QtdEstornos, time.Now(),
	); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *ExecucaoRepo) ListarPorData(ctx context.Context, data time.Time) ([]model.MovimentoExecucao, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT id, data_lote, produto, dominio, qtd_lancamentos, qtd_estornos, criado_em FROM movimento_execucao WHERE data_lote = @p1 ORDER BY produto, dominio",
		data.Format("2006-01-02"),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []model.MovimentoExecucao{}
	for rows.Next() {
		var e model.MovimentoExecucao
		if err := rows.Scan(&e.ID, &e.DataLote, &e.Produto, &e.Dominio, &e.QtdLancamentos, &e.QtdEstornos, &e.CriadoEm); err != nil {
			return nil, err
		}
		result = append(result, e)
	}
	return result, rows.Err()
}
