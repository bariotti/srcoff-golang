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
		"INSERT INTO movimento_execucao (data_lote, produto, dominio, qtd_lancamentos, qtd_estornos, qtd_movimento, criado_em) VALUES (@p1,@p2,@p3,@p4,@p5,@p6,@p7)",
		dataStr, e.Produto, e.Dominio, e.QtdLancamentos, e.QtdEstornos, e.QtdMovimento, time.Now(),
	); err != nil {
		return err
	}
	return tx.Commit()
}

// DatasExecutadas retorna as datas distintas com execução para (produto, domínio).
func (r *ExecucaoRepo) DatasExecutadas(ctx context.Context, produto, dominio string) ([]time.Time, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT DISTINCT data_lote FROM movimento_execucao WHERE produto = @p1 AND dominio = @p2 ORDER BY data_lote",
		produto, dominio,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var datas []time.Time
	for rows.Next() {
		var d time.Time
		if err := rows.Scan(&d); err != nil {
			return nil, err
		}
		datas = append(datas, d)
	}
	return datas, rows.Err()
}

// DatasComMovimento retorna as datas distintas que geraram movimento (qtd_lancamentos > 0)
// para (produto, domínio). Execuções sem lançamentos não entram.
func (r *ExecucaoRepo) DatasComMovimento(ctx context.Context, produto, dominio string) ([]time.Time, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT DISTINCT data_lote FROM movimento_execucao WHERE produto = @p1 AND dominio = @p2 AND ISNULL(qtd_movimento, 0) > 0 ORDER BY data_lote",
		produto, dominio,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var datas []time.Time
	for rows.Next() {
		var d time.Time
		if err := rows.Scan(&d); err != nil {
			return nil, err
		}
		datas = append(datas, d)
	}
	return datas, rows.Err()
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
