package sqlite

import (
	"context"
	"database/sql"
	"time"

	"srcoff/internal/model"
)

// ExecucaoRepo implementa repository.ExecucaoRepository em SQLite.
type ExecucaoRepo struct{ db *sql.DB }

func NewExecucaoRepo(db *sql.DB) *ExecucaoRepo { return &ExecucaoRepo{db: db} }

// RegistrarExecucao substitui a execução de (data, produto, domínio) pela nova.
func (r *ExecucaoRepo) RegistrarExecucao(ctx context.Context, e model.MovimentoExecucao) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	dataStr := fmtData(e.DataLote)
	if _, err := tx.ExecContext(ctx,
		"DELETE FROM movimento_execucao WHERE data_lote = ? AND produto = ? AND dominio = ?",
		dataStr, e.Produto, e.Dominio); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		"INSERT INTO movimento_execucao (data_lote, produto, dominio, qtd_lancamentos, qtd_estornos, qtd_movimento, criado_em) VALUES (?,?,?,?,?,?,?)",
		dataStr, e.Produto, e.Dominio, e.QtdLancamentos, e.QtdEstornos, e.QtdMovimento, fmtTS(time.Now())); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *ExecucaoRepo) ListarPorData(ctx context.Context, data time.Time) ([]model.MovimentoExecucao, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT id, data_lote, produto, dominio, qtd_lancamentos, qtd_estornos, IFNULL(qtd_movimento,0), IFNULL(criado_em,'') FROM movimento_execucao WHERE data_lote = ? ORDER BY produto, dominio",
		fmtData(data),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []model.MovimentoExecucao{}
	for rows.Next() {
		var e model.MovimentoExecucao
		var dataStr, criadoStr string
		if err := rows.Scan(&e.ID, &dataStr, &e.Produto, &e.Dominio, &e.QtdLancamentos, &e.QtdEstornos, &e.QtdMovimento, &criadoStr); err != nil {
			return nil, err
		}
		e.DataLote = parseData(dataStr)
		e.CriadoEm = parseTS(criadoStr)
		result = append(result, e)
	}
	return result, rows.Err()
}

func (r *ExecucaoRepo) DatasExecutadas(ctx context.Context, produto, dominio string) ([]time.Time, error) {
	return r.datas(ctx, "SELECT DISTINCT data_lote FROM movimento_execucao WHERE produto = ? AND dominio = ? ORDER BY data_lote", produto, dominio)
}

func (r *ExecucaoRepo) DatasComMovimento(ctx context.Context, produto, dominio string) ([]time.Time, error) {
	return r.datas(ctx, "SELECT DISTINCT data_lote FROM movimento_execucao WHERE produto = ? AND dominio = ? AND IFNULL(qtd_movimento,0) > 0 ORDER BY data_lote", produto, dominio)
}

func (r *ExecucaoRepo) datas(ctx context.Context, query, produto, dominio string) ([]time.Time, error) {
	rows, err := r.db.QueryContext(ctx, query, produto, dominio)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var datas []time.Time
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		datas = append(datas, parseData(s))
	}
	return datas, rows.Err()
}
