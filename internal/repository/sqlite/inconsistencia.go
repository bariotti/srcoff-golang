package sqlite

import (
	"context"
	"database/sql"
	"time"

	"srcoff/internal/model"
)

// InconsistenciaRepo implementa repository.InconsistenciaRepository em SQLite.
type InconsistenciaRepo struct{ db *sql.DB }

func NewInconsistenciaRepo(db *sql.DB) *InconsistenciaRepo { return &InconsistenciaRepo{db: db} }

func (r *InconsistenciaRepo) SubstituirPorEscopo(ctx context.Context, data time.Time, combos []model.ProdutoDominio, itens []model.InconsistenciaProcessamento) error {
	dataStr := fmtData(data)
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, c := range combos {
		if _, err := tx.ExecContext(ctx,
			"DELETE FROM inconsistencia_processamento WHERE data_lote_contabil = ? AND IFNULL(produto,'') = ? AND IFNULL(dominio,'') = ?",
			dataStr, c.Produto, c.Dominio); err != nil {
			return err
		}
	}
	for _, i := range itens {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO inconsistencia_processamento
			 (data_lote_contabil, codigo_identificador_boleto, produto, dominio, id_regra_contabil, descricao_regra_contabil, tipo, expressao, campos_faltantes, detalhe, criado_em)
			 VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
			dataStr, i.CodigoIdentificadorBoleto, i.Produto, i.Dominio, i.IDRegraContabil, i.DescricaoRegraContabil,
			i.Tipo, i.Expressao, i.CamposFaltantes, i.Detalhe, fmtTS(time.Now())); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *InconsistenciaRepo) ListarPorData(ctx context.Context, data time.Time) ([]model.InconsistenciaProcessamento, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, data_lote_contabil, IFNULL(codigo_identificador_boleto,''), IFNULL(produto,''), IFNULL(dominio,''), IFNULL(id_regra_contabil,0),
		        IFNULL(descricao_regra_contabil,''), IFNULL(tipo,''), IFNULL(expressao,''), IFNULL(campos_faltantes,''), IFNULL(detalhe,''), IFNULL(criado_em,'')
		 FROM inconsistencia_processamento WHERE data_lote_contabil = ? ORDER BY id`,
		fmtData(data),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []model.InconsistenciaProcessamento{}
	for rows.Next() {
		var i model.InconsistenciaProcessamento
		var dataStr, criadoStr string
		if err := rows.Scan(&i.ID, &dataStr, &i.CodigoIdentificadorBoleto, &i.Produto, &i.Dominio, &i.IDRegraContabil,
			&i.DescricaoRegraContabil, &i.Tipo, &i.Expressao, &i.CamposFaltantes, &i.Detalhe, &criadoStr); err != nil {
			return nil, err
		}
		i.DataLoteContabil = parseData(dataStr)
		i.CriadoEm = parseTS(criadoStr)
		result = append(result, i)
	}
	return result, rows.Err()
}
