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

func NewMovimentoContabilRepo(db *sql.DB) *MovimentoContabilRepo {
	return &MovimentoContabilRepo{db: db}
}

// colsMov é a lista de colunas (com alias opcional) usada nos SELECTs de lançamento.
func colsMov(alias string) string {
	a := ""
	if alias != "" {
		a = alias + "."
	}
	return a + "id, " + a + "data_lote_contabil, " + a + "codigo_versao_conteudo, " + a + "codigo_identificador_boleto, " +
		a + "valor_lancamento_contabil, " + a + "moeda_lancamento_contabil, " + a + "conta_debito, " + a + "conta_credito, " +
		a + "produto, " + a + "dominio, " + a + "indicador_reversao, " +
		"IFNULL(" + a + "descricao_regra_contabil,''), IFNULL(" + a + "descricao_condicao_contabil,''), IFNULL(" + a + "id_regra_contabil,0), " +
		"IFNULL(" + a + "origem,'automatico'), IFNULL(" + a + "veiculo,''), IFNULL(" + a + "valor_moeda_base_veiculo,0), IFNULL(" + a + "moeda_base_veiculo,'')"
}

func scanLancamentos(rows *sql.Rows) ([]model.LancamentoContabil, error) {
	out := []model.LancamentoContabil{}
	for rows.Next() {
		var l model.LancamentoContabil
		var dataStr string
		var rev int
		if err := rows.Scan(&l.ID, &dataStr, &l.CodigoVersaoConteudo, &l.CodigoIdentificadorBoleto,
			&l.ValorLancamentoContabil, &l.MoedaLancamentoContabil, &l.ContaDebito, &l.ContaCredito,
			&l.Produto, &l.Dominio, &rev, &l.DescricaoRegraContabil, &l.DescricaoCondicaoContabil, &l.IDRegraContabil, &l.Origem, &l.Veiculo, &l.ValorMoedaBaseVeiculo, &l.MoedaBaseVeiculo); err != nil {
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
		"INSERT INTO movimento_contabil (data_lote_contabil, codigo_versao_conteudo, codigo_identificador_boleto, valor_lancamento_contabil, moeda_lancamento_contabil, conta_debito, conta_credito, produto, dominio, indicador_reversao, descricao_regra_contabil, descricao_condicao_contabil, id_regra_contabil, origem, veiculo, valor_moeda_base_veiculo, moeda_base_veiculo) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)")
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, l := range lancamentos {
		if _, err := stmt.ExecContext(ctx,
			fmtData(l.DataLoteContabil), l.CodigoVersaoConteudo, l.CodigoIdentificadorBoleto,
			l.ValorLancamentoContabil, l.MoedaLancamentoContabil, l.ContaDebito, l.ContaCredito,
			l.Produto, l.Dominio, b2i(l.IndicadorReversao), l.DescricaoRegraContabil, l.DescricaoCondicaoContabil, l.IDRegraContabil, l.OrigemOuPadrao(), l.Veiculo, l.ValorMoedaBaseVeiculo, l.MoedaBaseVeiculo,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *MovimentoContabilRepo) BuscarPorDataEIndicador(ctx context.Context, data time.Time, indicadorReversao bool) ([]model.LancamentoContabil, error) {
	dataStr := fmtData(data)
	query := "SELECT " + colsMov("m") + " FROM movimento_contabil m WHERE m.data_lote_contabil = ? AND m.indicador_reversao = ? AND m.codigo_versao_conteudo = (SELECT MAX(m2.codigo_versao_conteudo) FROM movimento_contabil m2 WHERE m2.data_lote_contabil = m.data_lote_contabil AND m2.produto = m.produto AND m2.dominio = m.dominio)"
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

func (r *MovimentoContabilRepo) ConsultarPaginadoFiltrado(ctx context.Context, f model.FiltroMovimento) (*model.PaginaLancamentos, error) {
	return r.consultarFiltrado(ctx, f, false)
}

func (r *MovimentoContabilRepo) ConsultarPaginadoFiltradoSemCancelados(ctx context.Context, f model.FiltroMovimento) (*model.PaginaLancamentos, error) {
	return r.consultarFiltrado(ctx, f, true)
}

func (r *MovimentoContabilRepo) consultarFiltrado(ctx context.Context, f model.FiltroMovimento, excluirSaldoZero bool) (*model.PaginaLancamentos, error) {
	ini := fmtData(f.DataInicio)
	fim := fmtData(f.DataFim)

	where := "WHERE m.data_lote_contabil >= '" + ini + "' AND m.data_lote_contabil <= '" + fim + "'"
	if f.Boleto != "" {
		if f.BoletoExato {
			where += " AND m.codigo_identificador_boleto = '" + strings.ReplaceAll(f.Boleto, "'", "''") + "'"
		} else {
			where += " AND m.codigo_identificador_boleto LIKE '%" + strings.ReplaceAll(f.Boleto, "'", "''") + "%'"
		}
	}
	if f.Veiculo != "" {
		where += " AND IFNULL(m.veiculo,'') = '" + strings.ReplaceAll(f.Veiculo, "'", "''") + "'"
	}
	if f.Produto != "" {
		where += " AND m.produto = '" + strings.ReplaceAll(f.Produto, "'", "''") + "'"
	}
	if f.Dominio != "" {
		where += " AND m.dominio = '" + strings.ReplaceAll(f.Dominio, "'", "''") + "'"
	}
	if f.VersaoModo == "especifica" {
		where += fmt.Sprintf(" AND m.codigo_versao_conteudo = %d", f.Versao)
	}

	// Subquery de vigência correlacionada: versão vigente (MAX) por (data, produto, domínio).
	// produto/domínio são NOT NULL (sem IFNULL), o que permite o uso do índice IX_mov_data_prod_dom_ver.
	const vigSub = " AND m.codigo_versao_conteudo = (SELECT MAX(m2.codigo_versao_conteudo) FROM movimento_contabil m2 WHERE m2.data_lote_contabil = m.data_lote_contabil AND m2.produto = m.produto AND m2.dominio = m.dominio)"

	// Caminho leve (sem exclusão de saldo-zero): os filtros costumam ser seletivos
	// (ex.: razonete por boleto exato), então a subquery de vigência correlacionada
	// é avaliada para poucas linhas e sai barata.
	if !excluirSaldoZero {
		w := where
		if f.VersaoModo == "vigente" {
			w += vigSub
		}
		var total int
		if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM movimento_contabil m "+w).Scan(&total); err != nil {
			return nil, err
		}
		offset := (f.Pagina - 1) * f.Tamanho
		query := "SELECT " + colsMov("m") + " FROM movimento_contabil m " + w +
			fmt.Sprintf(" ORDER BY m.data_lote_contabil, m.codigo_versao_conteudo, m.id LIMIT %d OFFSET %d", f.Tamanho, offset)
		rows, err := r.db.QueryContext(ctx, query)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		lancamentos, err := scanLancamentos(rows)
		if err != nil {
			return nil, err
		}
		return &model.PaginaLancamentos{Total: total, Pagina: f.Pagina, Tamanho: f.Tamanho, Lancamentos: lancamentos}, nil
	}

	// Caminho pesado ("sem cancelados"): a consulta ampla pode varrer centenas de
	// milhares de linhas. Em vez de reavaliar as subqueries correlacionadas de
	// vigência e saldo-zero linha a linha (O(n^2)) a cada COUNT e SELECT, pré-agrega
	// vigência e saldo líquido UMA vez em tabelas temporárias indexadas e as reusa
	// nos dois statements (O(n)).
	//
	// nd/nc normalizam débito/crédito pela reversão (estorno troca as contas),
	// reproduzindo exatamente a chave da antiga subquery de saldo.
	const nd = "(CASE WHEN m.indicador_reversao=1 THEN m.conta_credito ELSE m.conta_debito END)"
	const nc = "(CASE WHEN m.indicador_reversao=1 THEN m.conta_debito ELSE m.conta_credito END)"

	conn, err := r.db.Conn(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	// Limpa resíduos de uma chamada anterior na mesma conexão do pool (MaxOpenConns=1).
	drop := func() {
		conn.ExecContext(ctx, "DROP TABLE IF EXISTS _saldo")
		conn.ExecContext(ctx, "DROP TABLE IF EXISTS _vig")
	}
	drop()
	defer drop()

	// _vig: versão vigente (MAX) por (data, produto, domínio) no período.
	// Usa produto/domínio crus (sem IFNULL): o gerador sempre grava string (nunca NULL),
	// e colunas cruas mantêm a ordem do índice IX_mov_data_prod_dom_ver, permitindo um
	// GROUP BY por índice de cobertura SEM b-tree temporária (~10x mais rápido que IFNULL).
	if _, err := conn.ExecContext(ctx, `CREATE TEMP TABLE _vig AS
		SELECT data_lote_contabil AS d, produto AS p, dominio AS dom, MAX(codigo_versao_conteudo) AS maxv
		FROM movimento_contabil
		WHERE data_lote_contabil >= '`+ini+`' AND data_lote_contabil <= '`+fim+`'
		GROUP BY data_lote_contabil, produto, dominio`); err != nil {
		return nil, err
	}
	if _, err := conn.ExecContext(ctx, "CREATE INDEX _vig_ix ON _vig (d, p, dom)"); err != nil {
		return nil, err
	}

	// _saldo: saldo líquido por (data, boleto, regra, conta normalizada) sobre as linhas
	// vigentes; mantém só as chaves com saldo != 0 (lançamentos efetivos, sem cancelados).
	// O filtro de boleto pode ser empurrado para cá sem alterar o resultado, pois o boleto
	// faz parte da chave do saldo.
	saldoWhere := "WHERE m.data_lote_contabil >= '" + ini + "' AND m.data_lote_contabil <= '" + fim + "'"
	if f.Boleto != "" {
		if f.BoletoExato {
			saldoWhere += " AND m.codigo_identificador_boleto = '" + strings.ReplaceAll(f.Boleto, "'", "''") + "'"
		} else {
			saldoWhere += " AND m.codigo_identificador_boleto LIKE '%" + strings.ReplaceAll(f.Boleto, "'", "''") + "%'"
		}
	}
	if _, err := conn.ExecContext(ctx, `CREATE TEMP TABLE _saldo AS
		SELECT m.data_lote_contabil AS d, m.codigo_identificador_boleto AS bol, m.id_regra_contabil AS reg, `+nd+` AS nd, `+nc+` AS nc,
		       SUM(CASE WHEN m.indicador_reversao=0 THEN m.valor_lancamento_contabil ELSE -m.valor_lancamento_contabil END) AS net
		FROM movimento_contabil m
		JOIN _vig ON _vig.d = m.data_lote_contabil AND _vig.p = m.produto AND _vig.dom = m.dominio
		`+saldoWhere+` AND m.codigo_versao_conteudo = _vig.maxv
		GROUP BY m.data_lote_contabil, m.codigo_identificador_boleto, m.id_regra_contabil, `+nd+`, `+nc+`
		HAVING SUM(CASE WHEN m.indicador_reversao=0 THEN m.valor_lancamento_contabil ELSE -m.valor_lancamento_contabil END) <> 0`); err != nil {
		return nil, err
	}
	if _, err := conn.ExecContext(ctx, "CREATE INDEX _saldo_ix ON _saldo (d, bol, reg, nd, nc)"); err != nil {
		return nil, err
	}

	joins := " JOIN _saldo s ON s.d = m.data_lote_contabil AND s.bol = m.codigo_identificador_boleto AND s.reg = m.id_regra_contabil AND s.nd = " + nd + " AND s.nc = " + nc
	if f.VersaoModo == "vigente" {
		joins += " JOIN _vig v ON v.d = m.data_lote_contabil AND v.p = m.produto AND v.dom = m.dominio"
		where += " AND m.codigo_versao_conteudo = v.maxv"
	}

	from := "FROM movimento_contabil m" + joins + " " + where

	var total int
	if err := conn.QueryRowContext(ctx, "SELECT COUNT(*) "+from).Scan(&total); err != nil {
		return nil, err
	}
	offset := (f.Pagina - 1) * f.Tamanho
	query := "SELECT " + colsMov("m") + " " + from +
		fmt.Sprintf(" ORDER BY m.data_lote_contabil, m.codigo_versao_conteudo, m.id LIMIT %d OFFSET %d", f.Tamanho, offset)
	rows, err := conn.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	lancamentos, err := scanLancamentos(rows)
	rows.Close()
	if err != nil {
		return nil, err
	}
	return &model.PaginaLancamentos{Total: total, Pagina: f.Pagina, Tamanho: f.Tamanho, Lancamentos: lancamentos}, nil
}

func (r *MovimentoContabilRepo) ExcluirPorDataEVersao(ctx context.Context, data time.Time, versao int) error {
	if versao > 0 {
		_, err := r.db.ExecContext(ctx, "DELETE FROM movimento_contabil WHERE data_lote_contabil = ? AND codigo_versao_conteudo = ?", fmtData(data), versao)
		return err
	}
	_, err := r.db.ExecContext(ctx, "DELETE FROM movimento_contabil WHERE data_lote_contabil = ?", fmtData(data))
	return err
}
