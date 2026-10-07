package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"srcoff/internal/model"
)

// MovimentoContabilRepo implementa repository.MovimentoContabilRepository em SQLite.
type MovimentoContabilRepo struct{ db *sql.DB }

func NewMovimentoContabilRepo(db *sql.DB) *MovimentoContabilRepo { return &MovimentoContabilRepo{db: db} }

// colsMov é a lista de colunas (com alias opcional) usada nos SELECTs de lançamento.
func colsMov(alias string) string {
	a := ""
	if alias != "" {
		a = alias + "."
	}
	return a + "id, " + a + "data_lote_contabil, " + a + "codigo_versao_conteudo, " + a + "codigo_identificador_boleto, " +
		a + "valor_lancamento_contabil, " + a + "moeda_lancamento_contabil, " + a + "conta_debito, " + a + "conta_credito, " +
		"IFNULL(" + a + "produto,''), IFNULL(" + a + "dominio,''), " + a + "indicador_reversao, " +
		"IFNULL(" + a + "descricao_regra_contabil,''), IFNULL(" + a + "descricao_condicao_contabil,''), IFNULL(" + a + "id_regra_contabil,0)"
}

func scanLancamentos(rows *sql.Rows) ([]model.LancamentoContabil, error) {
	out := []model.LancamentoContabil{}
	for rows.Next() {
		var l model.LancamentoContabil
		var dataStr string
		var rev int
		if err := rows.Scan(&l.ID, &dataStr, &l.CodigoVersaoConteudo, &l.CodigoIdentificadorBoleto,
			&l.ValorLancamentoContabil, &l.MoedaLancamentoContabil, &l.ContaDebito, &l.ContaCredito,
			&l.Produto, &l.Dominio, &rev, &l.DescricaoRegraContabil, &l.DescricaoCondicaoContabil, &l.IDRegraContabil); err != nil {
			return nil, err
		}
		l.DataLoteContabil = parseData(dataStr)
		l.IndicadorReversao = rev != 0
		out = append(out, l)
	}
	return out, rows.Err()
}

func (r *MovimentoContabilRepo) BulkInsert(ctx context.Context, lancamentos []model.LancamentoContabil) error {
	if len(lancamentos) == 0 {
		return nil
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.PrepareContext(ctx,
		"INSERT INTO movimento_contabil (data_lote_contabil, codigo_versao_conteudo, codigo_identificador_boleto, valor_lancamento_contabil, moeda_lancamento_contabil, conta_debito, conta_credito, produto, dominio, indicador_reversao, descricao_regra_contabil, descricao_condicao_contabil, id_regra_contabil) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)")
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, l := range lancamentos {
		if _, err := stmt.ExecContext(ctx,
			fmtData(l.DataLoteContabil), l.CodigoVersaoConteudo, l.CodigoIdentificadorBoleto,
			l.ValorLancamentoContabil, l.MoedaLancamentoContabil, l.ContaDebito, l.ContaCredito,
			l.Produto, l.Dominio, b2i(l.IndicadorReversao), l.DescricaoRegraContabil, l.DescricaoCondicaoContabil, l.IDRegraContabil,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *MovimentoContabilRepo) BuscarPorDataEIndicador(ctx context.Context, data time.Time, indicadorReversao bool) ([]model.LancamentoContabil, error) {
	dataStr := fmtData(data)
	query := "SELECT " + colsMov("m") + " FROM movimento_contabil m WHERE m.data_lote_contabil = ? AND m.indicador_reversao = ? AND m.codigo_versao_conteudo = (SELECT MAX(m2.codigo_versao_conteudo) FROM movimento_contabil m2 WHERE m2.data_lote_contabil = m.data_lote_contabil AND IFNULL(m2.produto,'') = IFNULL(m.produto,'') AND IFNULL(m2.dominio,'') = IFNULL(m.dominio,''))"
	rows, err := r.db.QueryContext(ctx, query, dataStr, b2i(indicadorReversao))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out, err := scanLancamentos(rows)
	if err != nil {
		return nil, err
	}
	var result []model.LancamentoContabil
	result = append(result, out...)
	return result, nil
}

func (r *MovimentoContabilRepo) ObterProximaVersao(ctx context.Context, data time.Time) (int, error) {
	var versao int
	err := r.db.QueryRowContext(ctx,
		"SELECT IFNULL(MAX(codigo_versao_conteudo),0)+1 FROM movimento_contabil WHERE data_lote_contabil = ?", fmtData(data),
	).Scan(&versao)
	return versao, err
}

func (r *MovimentoContabilRepo) ObterVersaoAtual(ctx context.Context, data time.Time) (int, error) {
	var versao int
	err := r.db.QueryRowContext(ctx,
		"SELECT IFNULL(MAX(codigo_versao_conteudo),1) FROM movimento_contabil WHERE data_lote_contabil = ?", fmtData(data),
	).Scan(&versao)
	return versao, err
}

func (r *MovimentoContabilRepo) ConsultarPaginado(ctx context.Context, data time.Time, pagina, tamanho int) (*model.PaginaLancamentos, error) {
	dataStr := fmtData(data)
	var total int
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM movimento_contabil WHERE data_lote_contabil = ?", dataStr).Scan(&total); err != nil {
		return nil, err
	}
	offset := (pagina - 1) * tamanho
	rows, err := r.db.QueryContext(ctx,
		"SELECT "+colsMov("")+" FROM movimento_contabil WHERE data_lote_contabil = ? ORDER BY id LIMIT ? OFFSET ?",
		dataStr, tamanho, offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	lancamentos, err := scanLancamentos(rows)
	if err != nil {
		return nil, err
	}
	return &model.PaginaLancamentos{Total: total, Pagina: pagina, Tamanho: tamanho, Lancamentos: lancamentos}, nil
}

func (r *MovimentoContabilRepo) ConsultarPaginadoFiltrado(ctx context.Context, dataInicio, dataFim time.Time, boleto string, versao int, versaoModo string, pagina, tamanho int) (*model.PaginaLancamentos, error) {
	return r.consultarFiltrado(ctx, dataInicio, dataFim, boleto, versao, versaoModo, pagina, tamanho, false)
}

func (r *MovimentoContabilRepo) ConsultarPaginadoFiltradoSemCancelados(ctx context.Context, dataInicio, dataFim time.Time, boleto string, versao int, versaoModo string, pagina, tamanho int) (*model.PaginaLancamentos, error) {
	return r.consultarFiltrado(ctx, dataInicio, dataFim, boleto, versao, versaoModo, pagina, tamanho, true)
}

func (r *MovimentoContabilRepo) consultarFiltrado(ctx context.Context, dataInicio, dataFim time.Time, boleto string, versao int, versaoModo string, pagina, tamanho int, excluirSaldoZero bool) (*model.PaginaLancamentos, error) {
	where := "WHERE m.data_lote_contabil >= '" + fmtData(dataInicio) + "' AND m.data_lote_contabil <= '" + fmtData(dataFim) + "'"
	if boleto != "" {
		where += " AND m.codigo_identificador_boleto LIKE '%" + strings.ReplaceAll(boleto, "'", "''") + "%'"
	}
	switch versaoModo {
	case "especifica":
		where += fmt.Sprintf(" AND m.codigo_versao_conteudo = %d", versao)
	case "vigente":
		where += " AND m.codigo_versao_conteudo = (SELECT MAX(m2.codigo_versao_conteudo) FROM movimento_contabil m2 WHERE m2.data_lote_contabil = m.data_lote_contabil AND IFNULL(m2.produto,'') = IFNULL(m.produto,'') AND IFNULL(m2.dominio,'') = IFNULL(m.dominio,''))"
	}

	filtroSaldoZero := ""
	if excluirSaldoZero {
		filtroSaldoZero = `AND (
			SELECT SUM(CASE WHEN m2.indicador_reversao = 0 THEN m2.valor_lancamento_contabil ELSE -m2.valor_lancamento_contabil END)
			FROM movimento_contabil m2
			WHERE m2.data_lote_contabil = m.data_lote_contabil
			  AND m2.codigo_identificador_boleto = m.codigo_identificador_boleto
			  AND m2.id_regra_contabil = m.id_regra_contabil
			  AND (CASE WHEN m2.indicador_reversao = 1 THEN m2.conta_credito ELSE m2.conta_debito END) = (CASE WHEN m.indicador_reversao = 1 THEN m.conta_credito ELSE m.conta_debito END)
			  AND (CASE WHEN m2.indicador_reversao = 1 THEN m2.conta_debito ELSE m2.conta_credito END) = (CASE WHEN m.indicador_reversao = 1 THEN m.conta_debito ELSE m.conta_credito END)
			  AND m2.codigo_versao_conteudo = (SELECT MAX(m3.codigo_versao_conteudo) FROM movimento_contabil m3 WHERE m3.data_lote_contabil = m.data_lote_contabil AND IFNULL(m3.produto,'') = IFNULL(m.produto,'') AND IFNULL(m3.dominio,'') = IFNULL(m.dominio,''))
		) <> 0`
	}

	var total int
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM movimento_contabil m "+where+" "+filtroSaldoZero).Scan(&total); err != nil {
		return nil, err
	}
	offset := (pagina - 1) * tamanho
	query := "SELECT " + colsMov("m") + " FROM movimento_contabil m " + where + " " + filtroSaldoZero +
		fmt.Sprintf(" ORDER BY m.data_lote_contabil, m.codigo_versao_conteudo, m.id LIMIT %d OFFSET %d", tamanho, offset)
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	lancamentos, err := scanLancamentos(rows)
	if err != nil {
		return nil, err
	}
	return &model.PaginaLancamentos{Total: total, Pagina: pagina, Tamanho: tamanho, Lancamentos: lancamentos}, nil
}

func (r *MovimentoContabilRepo) ExcluirPorDataEVersao(ctx context.Context, data time.Time, versao int) error {
	if versao > 0 {
		_, err := r.db.ExecContext(ctx, "DELETE FROM movimento_contabil WHERE data_lote_contabil = ? AND codigo_versao_conteudo = ?", fmtData(data), versao)
		return err
	}
	_, err := r.db.ExecContext(ctx, "DELETE FROM movimento_contabil WHERE data_lote_contabil = ?", fmtData(data))
	return err
}
