package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
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

// SubstituirPorEscopo remove as inconsistências das combinações informadas na data e
// insere as novas em transação.
func (r *InconsistenciaRepo) SubstituirPorEscopo(ctx context.Context, data time.Time, combos []model.ProdutoDominio, itens []model.InconsistenciaProcessamento) error {
	dataStr := data.Format("2006-01-02")
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, c := range combos {
		if _, err := tx.ExecContext(ctx,
			"DELETE FROM inconsistencia_processamento WHERE data_lote_contabil = @p1 AND ISNULL(produto,'') = @p2 AND ISNULL(dominio,'') = @p3",
			dataStr, c.Produto, c.Dominio,
		); err != nil {
			return err
		}
	}
	// Insert em LOTE (multi-row VALUES) em vez de linha a linha — evita milhares de
	// round-trips ao SQL Server quando há muitas inconsistências (ex.: campo ausente numa
	// regra aplicada a dezenas de milhares de posições).
	const maxLinhas = 500 // SQL Server aceita até 1000 linhas por INSERT ... VALUES
	for ini := 0; ini < len(itens); ini += maxLinhas {
		fim := ini + maxLinhas
		if fim > len(itens) {
			fim = len(itens)
		}
		var sb strings.Builder
		sb.WriteString("INSERT INTO inconsistencia_processamento (data_lote_contabil, codigo_identificador_boleto, produto, dominio, id_regra_contabil, descricao_regra_contabil, tipo, expressao, campos_faltantes, detalhe, criado_em) VALUES ")
		for j, i := range itens[ini:fim] {
			if j > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(fmt.Sprintf("('%s','%s','%s','%s',%d,'%s','%s','%s','%s','%s',GETDATE())",
				dataStr, esc(i.CodigoIdentificadorBoleto), esc(i.Produto), esc(i.Dominio), i.IDRegraContabil,
				esc(i.DescricaoRegraContabil), esc(i.Tipo), esc(i.Expressao), esc(i.CamposFaltantes), esc(i.Detalhe)))
		}
		if _, err := tx.ExecContext(ctx, sb.String()); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *InconsistenciaRepo) ListarPorData(ctx context.Context, data time.Time) ([]model.InconsistenciaProcessamento, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, data_lote_contabil, codigo_identificador_boleto, produto, ISNULL(dominio,''), id_regra_contabil,
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
		if err := rows.Scan(&i.ID, &i.DataLoteContabil, &i.CodigoIdentificadorBoleto, &i.Produto, &i.Dominio, &i.IDRegraContabil,
			&i.DescricaoRegraContabil, &i.Tipo, &i.Expressao, &i.CamposFaltantes, &i.Detalhe, &i.CriadoEm); err != nil {
			return nil, err
		}
		result = append(result, i)
	}
	return result, rows.Err()
}
