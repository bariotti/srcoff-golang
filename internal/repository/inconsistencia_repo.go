package repository

import (
	"context"
	"database/sql"
	"time"

	"srcoff/internal/model"
)

// InconsistenciaRepo persiste as inconsistências de processamento no SQL Server.
// Ver migration 005_inconsistencia.sql.
type InconsistenciaRepo struct {
	db *sql.DB
}

func NewInconsistenciaRepo(db *sql.DB) *InconsistenciaRepo {
	return &InconsistenciaRepo{db: db}
}

// SubstituirPorData remove as inconsistências da data e insere as novas em transação.
func (r *InconsistenciaRepo) SubstituirPorData(ctx context.Context, data time.Time, itens []model.InconsistenciaProcessamento) error {
	dataStr := data.Format("2006-01-02")
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, "DELETE FROM inconsistencia_processamento WHERE data_lote_contabil = @p1", dataStr); err != nil {
		return err
	}
	for _, i := range itens {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO inconsistencia_processamento
			 (data_lote_contabil, codigo_identificador_boleto, produto, id_regra_contabil, descricao_regra_contabil, tipo, expressao, campos_faltantes, detalhe, criado_em)
			 VALUES (@p1,@p2,@p3,@p4,@p5,@p6,@p7,@p8,@p9,@p10)`,
			dataStr, i.CodigoIdentificadorBoleto, i.Produto, i.IDRegraContabil, i.DescricaoRegraContabil,
			i.Tipo, i.Expressao, i.CamposFaltantes, i.Detalhe, time.Now(),
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *InconsistenciaRepo) ListarPorData(ctx context.Context, data time.Time) ([]model.InconsistenciaProcessamento, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, data_lote_contabil, codigo_identificador_boleto, produto, id_regra_contabil,
		        descricao_regra_contabil, tipo, expressao, campos_faltantes, detalhe, criado_em
		 FROM inconsistencia_processamento WHERE data_lote_contabil = @p1 ORDER BY id`,
		data.Format("2006-01-02"),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := []model.InconsistenciaProcessamento{}
	for rows.Next() {
		var i model.InconsistenciaProcessamento
		if err := rows.Scan(&i.ID, &i.DataLoteContabil, &i.CodigoIdentificadorBoleto, &i.Produto, &i.IDRegraContabil,
			&i.DescricaoRegraContabil, &i.Tipo, &i.Expressao, &i.CamposFaltantes, &i.Detalhe, &i.CriadoEm); err != nil {
			return nil, err
		}
		result = append(result, i)
	}
	return result, rows.Err()
}
