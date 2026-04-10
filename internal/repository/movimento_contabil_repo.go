package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"srcoff/internal/model"
)

type MovimentoContabilRepo struct {
	db *sql.DB
}

func NewMovimentoContabilRepo(db *sql.DB) *MovimentoContabilRepo {
	return &MovimentoContabilRepo{db: db}
}

// BulkInsert insere múltiplos lançamentos contábeis em uma única instrução INSERT.
func (r *MovimentoContabilRepo) BulkInsert(ctx context.Context, lancamentos []model.LancamentoContabil) error {
	if len(lancamentos) == 0 {
		return nil
	}

	const cols = `(data_lote_contabil, codigo_versao_conteudo, codigo_identificador_boleto,
		valor_lancamento_contabil, moeda_lancamento_contabil, conta_debito, conta_credito,
		indicador_reversao, descricao_regra_contabil, descricao_condicao_contabil, id_regra_contabil)`

	var sb strings.Builder
	sb.WriteString("INSERT INTO movimento_contabil ")
	sb.WriteString(cols)
	sb.WriteString(" VALUES ")

	args := make([]interface{}, 0, len(lancamentos)*11)
	for i, l := range lancamentos {
		if i > 0 {
			sb.WriteString(", ")
		}
		base := i * 11
		sb.WriteString(fmt.Sprintf(
			"(@p%d_0, @p%d_1, @p%d_2, @p%d_3, @p%d_4, @p%d_5, @p%d_6, @p%d_7, @p%d_8, @p%d_9, @p%d_10)",
			i, i, i, i, i, i, i, i, i, i, i,
		))
		_ = base
		args = append(args,
			sql.Named(fmt.Sprintf("p%d_0", i), l.DataLoteContabil),
			sql.Named(fmt.Sprintf("p%d_1", i), l.CodigoVersaoConteudo),
			sql.Named(fmt.Sprintf("p%d_2", i), l.CodigoIdentificadorBoleto),
			sql.Named(fmt.Sprintf("p%d_3", i), l.ValorLancamentoContabil),
			sql.Named(fmt.Sprintf("p%d_4", i), l.MoedaLancamentoContabil),
			sql.Named(fmt.Sprintf("p%d_5", i), l.ContaDebito),
			sql.Named(fmt.Sprintf("p%d_6", i), l.ContaCredito),
			sql.Named(fmt.Sprintf("p%d_7", i), l.IndicadorReversao),
			sql.Named(fmt.Sprintf("p%d_8", i), l.DescricaoRegraContabil),
			sql.Named(fmt.Sprintf("p%d_9", i), l.DescricaoCondicaoContabil),
			sql.Named(fmt.Sprintf("p%d_10", i), l.IDRegraContabil),
		)
	}

	_, err := r.db.ExecContext(ctx, sb.String(), args...)
	return err
}

// BuscarPorDataEIndicador retorna lançamentos filtrados por data e indicador de reversão.
func (r *MovimentoContabilRepo) BuscarPorDataEIndicador(ctx context.Context, data time.Time, indicadorReversao bool) ([]model.LancamentoContabil, error) {
	query := `
		SELECT
			id,
			data_lote_contabil,
			codigo_versao_conteudo,
			codigo_identificador_boleto,
			valor_lancamento_contabil,
			moeda_lancamento_contabil,
			conta_debito,
			conta_credito,
			indicador_reversao,
			descricao_regra_contabil,
			descricao_condicao_contabil,
			id_regra_contabil
		FROM movimento_contabil
		WHERE data_lote_contabil = @data
		  AND indicador_reversao = @indicador
	`

	rows, err := r.db.QueryContext(ctx, query,
		sql.Named("data", data),
		sql.Named("indicador", indicadorReversao),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []model.LancamentoContabil
	for rows.Next() {
		var l model.LancamentoContabil
		if err := rows.Scan(
			&l.ID,
			&l.DataLoteContabil,
			&l.CodigoVersaoConteudo,
			&l.CodigoIdentificadorBoleto,
			&l.ValorLancamentoContabil,
			&l.MoedaLancamentoContabil,
			&l.ContaDebito,
			&l.ContaCredito,
			&l.IndicadorReversao,
			&l.DescricaoRegraContabil,
			&l.DescricaoCondicaoContabil,
			&l.IDRegraContabil,
		); err != nil {
			return nil, err
		}
		result = append(result, l)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return result, nil
}

// ObterProximaVersao retorna MAX(codigo_versao_conteudo)+1 para a data, ou 1 se não houver registros.
func (r *MovimentoContabilRepo) ObterProximaVersao(ctx context.Context, data time.Time) (int, error) {
	var versao int
	err := r.db.QueryRowContext(ctx,
		`SELECT ISNULL(MAX(codigo_versao_conteudo), 0) + 1 FROM movimento_contabil WHERE data_lote_contabil = @data`,
		sql.Named("data", data),
	).Scan(&versao)
	if err != nil {
		return 0, err
	}
	return versao, nil
}

// ConsultarPaginado retorna lançamentos paginados para uma data.
func (r *MovimentoContabilRepo) ConsultarPaginado(ctx context.Context, data time.Time, pagina, tamanho int) (*model.PaginaLancamentos, error) {
	var total int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM movimento_contabil WHERE data_lote_contabil = @data`,
		sql.Named("data", data),
	).Scan(&total)
	if err != nil {
		return nil, err
	}

	offset := (pagina - 1) * tamanho

	rows, err := r.db.QueryContext(ctx, `
		SELECT
			id,
			data_lote_contabil,
			codigo_versao_conteudo,
			codigo_identificador_boleto,
			valor_lancamento_contabil,
			moeda_lancamento_contabil,
			conta_debito,
			conta_credito,
			indicador_reversao,
			descricao_regra_contabil,
			descricao_condicao_contabil,
			id_regra_contabil
		FROM movimento_contabil
		WHERE data_lote_contabil = @data
		ORDER BY id
		OFFSET @offset ROWS FETCH NEXT @tamanho ROWS ONLY
	`,
		sql.Named("data", data),
		sql.Named("offset", offset),
		sql.Named("tamanho", tamanho),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	lancamentos := []model.LancamentoContabil{}
	for rows.Next() {
		var l model.LancamentoContabil
		if err := rows.Scan(
			&l.ID,
			&l.DataLoteContabil,
			&l.CodigoVersaoConteudo,
			&l.CodigoIdentificadorBoleto,
			&l.ValorLancamentoContabil,
			&l.MoedaLancamentoContabil,
			&l.ContaDebito,
			&l.ContaCredito,
			&l.IndicadorReversao,
			&l.DescricaoRegraContabil,
			&l.DescricaoCondicaoContabil,
			&l.IDRegraContabil,
		); err != nil {
			return nil, err
		}
		lancamentos = append(lancamentos, l)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return &model.PaginaLancamentos{
		Total:       total,
		Pagina:      pagina,
		Tamanho:     tamanho,
		Lancamentos: lancamentos,
	}, nil
}
