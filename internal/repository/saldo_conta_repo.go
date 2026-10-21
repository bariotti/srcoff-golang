package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"

	"srcoff/internal/model"
)

// SaldoContaRepo persiste a projeção de saldo (RN-170) no SQL Server.
type SaldoContaRepo struct {
	db *sql.DB
}

func NewSaldoContaRepo(db *sql.DB) *SaldoContaRepo { return &SaldoContaRepo{db: db} }

const colunasSaldo = "produto,dominio,codigo_identificador_boleto,id_regra_contabil,conta_debito,conta_credito,saldo,moeda_lancamento,descricao_regra,descricao_condicao"

// vrowSaldo monta uma linha VALUES (...) para a projeção de saldo (string-building para
// evitar o limite de parâmetros e permitir lotes grandes).
func vrowSaldo(s model.SaldoConta) string {
	return fmt.Sprintf("('%s','%s','%s',%d,'%s','%s',%s,'%s','%s','%s')",
		esc(s.Produto), esc(s.Dominio), esc(s.CodigoIdentificadorBoleto), s.IDRegraContabil,
		esc(s.ContaDebito), esc(s.ContaCredito), strconv.FormatFloat(s.Saldo, 'f', -1, 64),
		esc(s.MoedaLancamento), esc(s.DescricaoRegra), esc(s.DescricaoCondicao))
}

const joinChaveSaldo = "t.produto=v.produto AND t.dominio=v.dominio AND t.codigo_identificador_boleto=v.codigo_identificador_boleto AND t.id_regra_contabil=v.id_regra_contabil AND t.conta_debito=v.conta_debito AND t.conta_credito=v.conta_credito"

// AplicarDeltas faz o upsert em LOTE (UPDATE+soma dos existentes e INSERT dos novos) via
// tabela derivada VALUES — em vez de um round-trip por chave. Isso evita a lentidão de
// gravar o saldo linha a linha após gerar muitos lançamentos.
func (r *SaldoContaRepo) AplicarDeltas(ctx context.Context, deltas []model.SaldoConta) error {
	if len(deltas) == 0 {
		return nil
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	const maxLinhas = 500
	for ini := 0; ini < len(deltas); ini += maxLinhas {
		fim := ini + maxLinhas
		if fim > len(deltas) {
			fim = len(deltas)
		}
		rows := make([]string, 0, fim-ini)
		for _, d := range deltas[ini:fim] {
			rows = append(rows, vrowSaldo(d))
		}
		values := strings.Join(rows, ",")
		// Soma nos existentes.
		if _, err := tx.ExecContext(ctx,
			"UPDATE t SET t.saldo=t.saldo+v.saldo, t.moeda_lancamento=v.moeda_lancamento, t.descricao_regra=v.descricao_regra, t.descricao_condicao=v.descricao_condicao "+
				"FROM saldo_conta t JOIN (VALUES "+values+") AS v("+colunasSaldo+") ON "+joinChaveSaldo); err != nil {
			return err
		}
		// Insere os que ainda não existem.
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO saldo_conta ("+colunasSaldo+") "+
				"SELECT v.produto,v.dominio,v.codigo_identificador_boleto,v.id_regra_contabil,v.conta_debito,v.conta_credito,v.saldo,v.moeda_lancamento,v.descricao_regra,v.descricao_condicao "+
				"FROM (VALUES "+values+") AS v("+colunasSaldo+") WHERE NOT EXISTS (SELECT 1 FROM saldo_conta t WHERE "+joinChaveSaldo+")"); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *SaldoContaRepo) SubstituirCombos(ctx context.Context, combos []model.ProdutoDominio, saldos []model.SaldoConta) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, c := range combos {
		if _, err := tx.ExecContext(ctx, "DELETE FROM saldo_conta WHERE produto=@p1 AND dominio=@p2", c.Produto, c.Dominio); err != nil {
			return err
		}
	}
	// Insert em lote (multi-row VALUES).
	const maxLinhas = 500
	for ini := 0; ini < len(saldos); ini += maxLinhas {
		fim := ini + maxLinhas
		if fim > len(saldos) {
			fim = len(saldos)
		}
		rows := make([]string, 0, fim-ini)
		for _, s := range saldos[ini:fim] {
			rows = append(rows, vrowSaldo(s))
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO saldo_conta ("+colunasSaldo+") VALUES "+strings.Join(rows, ",")); err != nil {
			return err
		}
	}
	return tx.Commit()
}

const selSaldoSQL = "SELECT produto, dominio, codigo_identificador_boleto, id_regra_contabil, conta_debito, conta_credito, saldo, ISNULL(moeda_lancamento,''), ISNULL(descricao_regra,''), ISNULL(descricao_condicao,'') FROM saldo_conta"

// saldoWhereSQL monta o WHERE (@pN) e os args a partir do filtro.
func saldoWhereSQL(f model.SaldoFiltro) (string, []interface{}) {
	var where []string
	var args []interface{}
	add := func(cond string, v interface{}) {
		args = append(args, v)
		where = append(where, fmt.Sprintf(cond, len(args)))
	}
	if f.Produto != "" {
		add("produto=@p%d", f.Produto)
	}
	if f.Dominio != "" {
		add("dominio=@p%d", f.Dominio)
	}
	if f.Boleto != "" {
		add("codigo_identificador_boleto=@p%d", f.Boleto)
	}
	if f.IDRegra != 0 {
		add("id_regra_contabil=@p%d", f.IDRegra)
	}
	if f.ContaDebito != "" {
		add("conta_debito=@p%d", f.ContaDebito)
	}
	if f.ContaCredito != "" {
		add("conta_credito=@p%d", f.ContaCredito)
	}
	if f.Conta != "" {
		args = append(args, f.Conta)
		p := len(args)
		where = append(where, fmt.Sprintf("(conta_debito=@p%d OR conta_credito=@p%d)", p, p))
	}
	if len(f.RegrasIn) > 0 {
		ph := make([]string, len(f.RegrasIn))
		for i, id := range f.RegrasIn {
			args = append(args, id)
			ph[i] = fmt.Sprintf("@p%d", len(args))
		}
		where = append(where, "id_regra_contabil IN ("+strings.Join(ph, ",")+")")
	}
	if !f.IncluiZero {
		where = append(where, "saldo <> 0")
	}
	if len(where) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(where, " AND "), args
}

func scanSaldosSQL(rows *sql.Rows) ([]model.SaldoConta, error) {
	var out []model.SaldoConta
	for rows.Next() {
		var s model.SaldoConta
		if err := rows.Scan(&s.Produto, &s.Dominio, &s.CodigoIdentificadorBoleto, &s.IDRegraContabil, &s.ContaDebito, &s.ContaCredito, &s.Saldo, &s.MoedaLancamento, &s.DescricaoRegra, &s.DescricaoCondicao); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *SaldoContaRepo) Listar(ctx context.Context, f model.SaldoFiltro) ([]model.SaldoConta, error) {
	w, args := saldoWhereSQL(f)
	rows, err := r.db.QueryContext(ctx, selSaldoSQL+w+" ORDER BY produto, dominio, codigo_identificador_boleto, id_regra_contabil", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSaldosSQL(rows)
}

func (r *SaldoContaRepo) ListarPaginado(ctx context.Context, f model.SaldoFiltro, pagina, tamanho int) (*model.SaldoPagina, error) {
	w, args := saldoWhereSQL(f)
	var total int
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM saldo_conta"+w, args...).Scan(&total); err != nil {
		return nil, err
	}
	if pagina < 1 {
		pagina = 1
	}
	if tamanho < 1 {
		tamanho = 50
	}
	args = append(args, (pagina-1)*tamanho, tamanho)
	q := selSaldoSQL + w + fmt.Sprintf(" ORDER BY produto, dominio, codigo_identificador_boleto, id_regra_contabil OFFSET @p%d ROWS FETCH NEXT @p%d ROWS ONLY", len(args)-1, len(args))
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out, err := scanSaldosSQL(rows)
	if err != nil {
		return nil, err
	}
	return &model.SaldoPagina{Total: total, Pagina: pagina, Tamanho: tamanho, Saldos: out}, nil
}

func (r *SaldoContaRepo) EstaVazio(ctx context.Context) (bool, error) {
	var n int
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM saldo_conta").Scan(&n); err != nil {
		return false, err
	}
	return n == 0, nil
}
