package repository

import (
	"context"
	"database/sql"
	"fmt"
	"log"
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

// maxLinhasInsert é o limite do SQL Server para expressões de linha em um único
// INSERT ... VALUES. O BulkInsert quebra em lotes de até esse tamanho.
const maxLinhasInsert = 1000

func (r *MovimentoContabilRepo) BulkInsert(ctx context.Context, lancamentos []model.LancamentoContabil) error {
	if len(lancamentos) == 0 {
		return nil
	}

	// Uma transação envolve todos os lotes para manter o "tudo ou nada".
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for ini := 0; ini < len(lancamentos); ini += maxLinhasInsert {
		fim := ini + maxLinhasInsert
		if fim > len(lancamentos) {
			fim = len(lancamentos)
		}
		if err := inserirLoteMovimento(ctx, tx, lancamentos[ini:fim]); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// inserirLoteMovimento persiste um lote de até maxLinhasInsert lançamentos em um
// único INSERT ... VALUES.
func inserirLoteMovimento(ctx context.Context, tx *sql.Tx, lancamentos []model.LancamentoContabil) error {
	var sb strings.Builder
	sb.WriteString("INSERT INTO movimento_contabil (data_lote_contabil, codigo_versao_conteudo, codigo_identificador_boleto, valor_lancamento_contabil, moeda_lancamento_contabil, conta_debito, conta_credito, produto, dominio, indicador_reversao, descricao_regra_contabil, descricao_condicao_contabil, id_regra_contabil, origem, veiculo, valor_moeda_base_veiculo, moeda_base_veiculo) VALUES ")

	for i, l := range lancamentos {
		if i > 0 {
			sb.WriteString(", ")
		}
		reversao := 0
		if l.IndicadorReversao {
			reversao = 1
		}
		sb.WriteString(fmt.Sprintf("('%s', %d, '%s', %f, '%s', '%s', '%s', '%s', '%s', %d, '%s', '%s', %d, '%s', '%s', %f, '%s')",
			l.DataLoteContabil.Format("2006-01-02"),
			l.CodigoVersaoConteudo,
			strings.ReplaceAll(l.CodigoIdentificadorBoleto, "'", "''"),
			l.ValorLancamentoContabil,
			l.MoedaLancamentoContabil,
			l.ContaDebito,
			l.ContaCredito,
			strings.ReplaceAll(l.Produto, "'", "''"),
			strings.ReplaceAll(l.Dominio, "'", "''"),
			reversao,
			strings.ReplaceAll(l.DescricaoRegraContabil, "'", "''"),
			strings.ReplaceAll(l.DescricaoCondicaoContabil, "'", "''"),
			l.IDRegraContabil,
			l.OrigemOuPadrao(),
			strings.ReplaceAll(l.Veiculo, "'", "''"),
			l.ValorMoedaBaseVeiculo,
			strings.ReplaceAll(l.MoedaBaseVeiculo, "'", "''"),
		))
	}

	_, err := tx.ExecContext(ctx, sb.String())
	return err
}

func (r *MovimentoContabilRepo) BuscarPorDataEIndicador(ctx context.Context, data time.Time, indicadorReversao bool) ([]model.LancamentoContabil, error) {
	dataStr := data.Format("2006-01-02")
	ind := 0
	if indicadorReversao {
		ind = 1
	}
	// Vigente (MAX) por (data, produto, domínio) — cada combinação versiona de forma independente.
	query := "SELECT m.id, m.data_lote_contabil, m.codigo_versao_conteudo, m.codigo_identificador_boleto, m.valor_lancamento_contabil, m.moeda_lancamento_contabil, m.conta_debito, m.conta_credito, ISNULL(m.produto,''), ISNULL(m.dominio,''), m.indicador_reversao, m.descricao_regra_contabil, m.descricao_condicao_contabil, m.id_regra_contabil, ISNULL(m.origem,'automatico'), ISNULL(m.veiculo,''), ISNULL(m.valor_moeda_base_veiculo,0), ISNULL(m.moeda_base_veiculo,'') FROM movimento_contabil m WHERE m.data_lote_contabil = '" + dataStr + "' AND m.indicador_reversao = " + fmt.Sprintf("%d", ind) + " AND m.codigo_versao_conteudo = (SELECT MAX(m2.codigo_versao_conteudo) FROM movimento_contabil m2 WHERE m2.data_lote_contabil = m.data_lote_contabil AND ISNULL(m2.produto,'') = ISNULL(m.produto,'') AND ISNULL(m2.dominio,'') = ISNULL(m.dominio,''))"

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []model.LancamentoContabil
	for rows.Next() {
		var l model.LancamentoContabil
		if err := rows.Scan(&l.ID, &l.DataLoteContabil, &l.CodigoVersaoConteudo, &l.CodigoIdentificadorBoleto, &l.ValorLancamentoContabil, &l.MoedaLancamentoContabil, &l.ContaDebito, &l.ContaCredito, &l.Produto, &l.Dominio, &l.IndicadorReversao, &l.DescricaoRegraContabil, &l.DescricaoCondicaoContabil, &l.IDRegraContabil, &l.Origem, &l.Veiculo, &l.ValorMoedaBaseVeiculo, &l.MoedaBaseVeiculo); err != nil {
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
		"SELECT id, data_lote_contabil, codigo_versao_conteudo, codigo_identificador_boleto, valor_lancamento_contabil, moeda_lancamento_contabil, conta_debito, conta_credito, ISNULL(produto,''), ISNULL(dominio,''), indicador_reversao, descricao_regra_contabil, descricao_condicao_contabil, id_regra_contabil, ISNULL(origem,'automatico'), ISNULL(veiculo,''), ISNULL(valor_moeda_base_veiculo,0), ISNULL(moeda_base_veiculo,'') FROM movimento_contabil WHERE data_lote_contabil = '%s' ORDER BY id OFFSET %d ROWS FETCH NEXT %d ROWS ONLY",
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
		if err := rows.Scan(&l.ID, &l.DataLoteContabil, &l.CodigoVersaoConteudo, &l.CodigoIdentificadorBoleto, &l.ValorLancamentoContabil, &l.MoedaLancamentoContabil, &l.ContaDebito, &l.ContaCredito, &l.Produto, &l.Dominio, &l.IndicadorReversao, &l.DescricaoRegraContabil, &l.DescricaoCondicaoContabil, &l.IDRegraContabil, &l.Origem, &l.Veiculo, &l.ValorMoedaBaseVeiculo, &l.MoedaBaseVeiculo); err != nil {
			return nil, err
		}
		lancamentos = append(lancamentos, l)
	}
	if err := rows.Err(); err != nil {
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
	inicioStr := f.DataInicio.Format("2006-01-02")
	fimStr := f.DataFim.Format("2006-01-02")

	where := "WHERE m.data_lote_contabil >= '" + inicioStr + "' AND m.data_lote_contabil <= '" + fimStr + "'"
	if f.Boleto != "" {
		where += " AND m.codigo_identificador_boleto LIKE '%" + strings.ReplaceAll(f.Boleto, "'", "''") + "%'"
	}
	if f.Veiculo != "" {
		where += " AND ISNULL(m.veiculo,'') = '" + strings.ReplaceAll(f.Veiculo, "'", "''") + "'"
	}
	if f.Produto != "" {
		where += " AND ISNULL(m.produto,'') = '" + strings.ReplaceAll(f.Produto, "'", "''") + "'"
	}
	if f.Dominio != "" {
		where += " AND ISNULL(m.dominio,'') = '" + strings.ReplaceAll(f.Dominio, "'", "''") + "'"
	}
	switch f.VersaoModo {
	case "especifica":
		where += fmt.Sprintf(" AND m.codigo_versao_conteudo = %d", f.Versao)
	case "vigente":
		where += " AND m.codigo_versao_conteudo = (SELECT MAX(m2.codigo_versao_conteudo) FROM movimento_contabil m2 WHERE m2.data_lote_contabil = m.data_lote_contabil AND ISNULL(m2.produto,'') = ISNULL(m.produto,'') AND ISNULL(m2.dominio,'') = ISNULL(m.dominio,''))"
	}

	filtroSaldoZero := ""
	if excluirSaldoZero {
		// Elimina grupos cujo saldo líquido é zero (par lançamento + estorno que se anulam).
		// A chave normaliza as contas do estorno (invertidas na geração) de volta à ordem do
		// lançamento normal, para que normal (débito A, crédito B) e estorno (débito B, crédito A)
		// caiam no MESMO grupo — igual ao backend de arquivo. Versão vigente por data/produto/domínio.
		filtroSaldoZero = `AND (
			SELECT SUM(CASE WHEN m2.indicador_reversao = 0
			                THEN  m2.valor_lancamento_contabil
			                ELSE -m2.valor_lancamento_contabil
			           END)
			FROM movimento_contabil m2
			WHERE m2.data_lote_contabil         = m.data_lote_contabil
			  AND m2.codigo_identificador_boleto = m.codigo_identificador_boleto
			  AND m2.id_regra_contabil           = m.id_regra_contabil
			  AND (CASE WHEN m2.indicador_reversao = 1 THEN m2.conta_credito ELSE m2.conta_debito END)
			    = (CASE WHEN m.indicador_reversao  = 1 THEN m.conta_credito  ELSE m.conta_debito  END)
			  AND (CASE WHEN m2.indicador_reversao = 1 THEN m2.conta_debito  ELSE m2.conta_credito END)
			    = (CASE WHEN m.indicador_reversao  = 1 THEN m.conta_debito   ELSE m.conta_credito  END)
			  AND m2.codigo_versao_conteudo      = (SELECT MAX(m3.codigo_versao_conteudo) FROM movimento_contabil m3 WHERE m3.data_lote_contabil = m.data_lote_contabil AND ISNULL(m3.produto,'') = ISNULL(m.produto,'') AND ISNULL(m3.dominio,'') = ISNULL(m.dominio,''))
		) <> 0`
	}

	var total int
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM movimento_contabil m "+where+" "+filtroSaldoZero).Scan(&total); err != nil {
		return nil, err
	}

	offset := (f.Pagina - 1) * f.Tamanho
	query := fmt.Sprintf(`
		SELECT m.id, m.data_lote_contabil, m.codigo_versao_conteudo, m.codigo_identificador_boleto,
		       m.valor_lancamento_contabil, m.moeda_lancamento_contabil, m.conta_debito, m.conta_credito,
		       ISNULL(m.produto,''), ISNULL(m.dominio,''),
		       m.indicador_reversao, m.descricao_regra_contabil, m.descricao_condicao_contabil, m.id_regra_contabil, ISNULL(m.origem,'automatico'), ISNULL(m.veiculo,''), ISNULL(m.valor_moeda_base_veiculo,0), ISNULL(m.moeda_base_veiculo,'')
		FROM movimento_contabil m
		%s %s
		ORDER BY m.data_lote_contabil, m.codigo_versao_conteudo, m.id
		OFFSET %d ROWS FETCH NEXT %d ROWS ONLY`,
		where, filtroSaldoZero, offset, f.Tamanho,
	)

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	lancamentos := []model.LancamentoContabil{}
	for rows.Next() {
		var l model.LancamentoContabil
		if err := rows.Scan(&l.ID, &l.DataLoteContabil, &l.CodigoVersaoConteudo, &l.CodigoIdentificadorBoleto, &l.ValorLancamentoContabil, &l.MoedaLancamentoContabil, &l.ContaDebito, &l.ContaCredito, &l.Produto, &l.Dominio, &l.IndicadorReversao, &l.DescricaoRegraContabil, &l.DescricaoCondicaoContabil, &l.IDRegraContabil, &l.Origem, &l.Veiculo, &l.ValorMoedaBaseVeiculo, &l.MoedaBaseVeiculo); err != nil {
			return nil, err
		}
		lancamentos = append(lancamentos, l)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return &model.PaginaLancamentos{Total: total, Pagina: f.Pagina, Tamanho: f.Tamanho, Lancamentos: lancamentos}, nil
}

func (r *MovimentoContabilRepo) ExcluirPorDataEVersao(ctx context.Context, data time.Time, versao int) error {
	dataStr := data.Format("2006-01-02")
	query := "DELETE FROM movimento_contabil WHERE data_lote_contabil = '" + dataStr + "'"
	if versao > 0 {
		query += fmt.Sprintf(" AND codigo_versao_conteudo = %d", versao)
	}
	log.Printf("[excluir-movimento] query: %s", query)
	result, err := r.db.ExecContext(ctx, query)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	log.Printf("[excluir-movimento] rows affected: %d", rows)
	return nil
}
