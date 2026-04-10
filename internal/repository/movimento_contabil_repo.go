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

func (r *MovimentoContabilRepo) BulkInsert(ctx context.Context, lancamentos []model.LancamentoContabil) error {
	if len(lancamentos) == 0 {
		return nil
	}

	var sb strings.Builder
	sb.WriteString("INSERT INTO movimento_contabil (data_lote_contabil, codigo_versao_conteudo, codigo_identificador_boleto, valor_lancamento_contabil, moeda_lancamento_contabil, conta_debito, conta_credito, indicador_reversao, descricao_regra_contabil, descricao_condicao_contabil, id_regra_contabil) VALUES ")

	for i, l := range lancamentos {
		if i > 0 {
			sb.WriteString(", ")
		}
		reversao := 0
		if l.IndicadorReversao {
			reversao = 1
		}
		sb.WriteString(fmt.Sprintf("('%s', %d, '%s', %f, '%s', '%s', '%s', %d, '%s', '%s', %d)",
			l.DataLoteContabil.Format("2006-01-02"),
			l.CodigoVersaoConteudo,
			l.CodigoIdentificadorBoleto,
			l.ValorLancamentoContabil,
			l.MoedaLancamentoContabil,
			l.ContaDebito,
			l.ContaCredito,
			reversao,
			strings.ReplaceAll(l.DescricaoRegraContabil, "'", "''"),
			strings.ReplaceAll(l.DescricaoCondicaoContabil, "'", "''"),
			l.IDRegraContabil,
		))
	}

	_, err := r.db.ExecContext(ctx, sb.String())
	return err
}

func (r *MovimentoContabilRepo) BuscarPorDataEIndicador(ctx context.Context, data time.Time, indicadorReversao bool) ([]model.LancamentoContabil, error) {
	dataStr := data.Format("2006-01-02")
	ind := 0
	if indicadorReversao {
		ind = 1
	}
	query := "SELECT id, data_lote_contabil, codigo_versao_conteudo, codigo_identificador_boleto, valor_lancamento_contabil, moeda_lancamento_contabil, conta_debito, conta_credito, indicador_reversao, descricao_regra_contabil, descricao_condicao_contabil, id_regra_contabil FROM movimento_contabil WHERE data_lote_contabil = '" + dataStr + "' AND indicador_reversao = " + fmt.Sprintf("%d", ind)

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []model.LancamentoContabil
	for rows.Next() {
		var l model.LancamentoContabil
		if err := rows.Scan(&l.ID, &l.DataLoteContabil, &l.CodigoVersaoConteudo, &l.CodigoIdentificadorBoleto, &l.ValorLancamentoContabil, &l.MoedaLancamentoContabil, &l.ContaDebito, &l.ContaCredito, &l.IndicadorReversao, &l.DescricaoRegraContabil, &l.DescricaoCondicaoContabil, &l.IDRegraContabil); err != nil {
			return nil, err
		}
		result = append(result, l)
	}
	return result, rows.Err()
}

func (r *MovimentoContabilRepo) ObterProximaVersao(ctx context.Context, data time.Time) (int, error) {
	dataStr := data.Format("2006-01-02")
	var versao int
	err := r.db.QueryRowContext(ctx,
		"SELECT ISNULL(MAX(codigo_versao_conteudo), 0) + 1 FROM movimento_contabil WHERE data_lote_contabil = '"+dataStr+"'",
	).Scan(&versao)
	return versao, err
}

func (r *MovimentoContabilRepo) ObterVersaoAtual(ctx context.Context, data time.Time) (int, error) {
	dataStr := data.Format("2006-01-02")
	var versao int
	err := r.db.QueryRowContext(ctx,
		"SELECT ISNULL(MAX(codigo_versao_conteudo), 1) FROM movimento_contabil WHERE data_lote_contabil = '"+dataStr+"'",
	).Scan(&versao)
	return versao, err
}

func (r *MovimentoContabilRepo) ConsultarPaginado(ctx context.Context, data time.Time, pagina, tamanho int) (*model.PaginaLancamentos, error) {
	dataStr := data.Format("2006-01-02")

	var total int
	if err := r.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM movimento_contabil WHERE data_lote_contabil = '"+dataStr+"'",
	).Scan(&total); err != nil {
		return nil, err
	}

	offset := (pagina - 1) * tamanho
	query := fmt.Sprintf(
		"SELECT id, data_lote_contabil, codigo_versao_conteudo, codigo_identificador_boleto, valor_lancamento_contabil, moeda_lancamento_contabil, conta_debito, conta_credito, indicador_reversao, descricao_regra_contabil, descricao_condicao_contabil, id_regra_contabil FROM movimento_contabil WHERE data_lote_contabil = '%s' ORDER BY id OFFSET %d ROWS FETCH NEXT %d ROWS ONLY",
		dataStr, offset, tamanho,
	)

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	lancamentos := []model.LancamentoContabil{}
	for rows.Next() {
		var l model.LancamentoContabil
		if err := rows.Scan(&l.ID, &l.DataLoteContabil, &l.CodigoVersaoConteudo, &l.CodigoIdentificadorBoleto, &l.ValorLancamentoContabil, &l.MoedaLancamentoContabil, &l.ContaDebito, &l.ContaCredito, &l.IndicadorReversao, &l.DescricaoRegraContabil, &l.DescricaoCondicaoContabil, &l.IDRegraContabil); err != nil {
			return nil, err
		}
		lancamentos = append(lancamentos, l)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return &model.PaginaLancamentos{Total: total, Pagina: pagina, Tamanho: tamanho, Lancamentos: lancamentos}, nil
}

func (r *MovimentoContabilRepo) ConsultarPaginadoFiltrado(ctx context.Context, dataInicio, dataFim time.Time, boleto string, versao int, versaoModo string, pagina, tamanho int) (*model.PaginaLancamentos, error) {
	where := "WHERE data_lote_contabil >= '" + dataInicio.Format("2006-01-02") + "' AND data_lote_contabil <= '" + dataFim.Format("2006-01-02") + "'"
	if boleto != "" {
		where += " AND codigo_identificador_boleto LIKE '%" + strings.ReplaceAll(boleto, "'", "''") + "%'"
	}
	switch versaoModo {
	case "especifica":
		where += fmt.Sprintf(" AND codigo_versao_conteudo = %d", versao)
	case "vigente":
		// maior versão por data dentro do período
		where += " AND codigo_versao_conteudo = (SELECT MAX(codigo_versao_conteudo) FROM movimento_contabil m2 WHERE m2.data_lote_contabil = movimento_contabil.data_lote_contabil)"
	// "todas" — sem filtro adicional
	}

	var total int
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM movimento_contabil "+where).Scan(&total); err != nil {
		return nil, err
	}

	offset := (pagina - 1) * tamanho
	query := fmt.Sprintf(
		"SELECT id, data_lote_contabil, codigo_versao_conteudo, codigo_identificador_boleto, valor_lancamento_contabil, moeda_lancamento_contabil, conta_debito, conta_credito, indicador_reversao, descricao_regra_contabil, descricao_condicao_contabil, id_regra_contabil FROM movimento_contabil %s ORDER BY data_lote_contabil, codigo_versao_conteudo, id OFFSET %d ROWS FETCH NEXT %d ROWS ONLY",
		where, offset, tamanho,
	)

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	lancamentos := []model.LancamentoContabil{}
	for rows.Next() {
		var l model.LancamentoContabil
		if err := rows.Scan(&l.ID, &l.DataLoteContabil, &l.CodigoVersaoConteudo, &l.CodigoIdentificadorBoleto, &l.ValorLancamentoContabil, &l.MoedaLancamentoContabil, &l.ContaDebito, &l.ContaCredito, &l.IndicadorReversao, &l.DescricaoRegraContabil, &l.DescricaoCondicaoContabil, &l.IDRegraContabil); err != nil {
			return nil, err
		}
		lancamentos = append(lancamentos, l)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return &model.PaginaLancamentos{Total: total, Pagina: pagina, Tamanho: tamanho, Lancamentos: lancamentos}, nil
}
