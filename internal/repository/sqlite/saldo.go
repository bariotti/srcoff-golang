package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"srcoff/internal/model"
)

// SaldoContaRepo implementa a projeção materializada de saldo (RN-170) em SQLite.
type SaldoContaRepo struct{ db *sql.DB }

func NewSaldoContaRepo(db *sql.DB) *SaldoContaRepo { return &SaldoContaRepo{db: db} }

func (r *SaldoContaRepo) AplicarDeltas(ctx context.Context, deltas []model.SaldoConta) error {
	if len(deltas) == 0 {
		return nil
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO saldo_conta (produto, dominio, codigo_identificador_boleto, id_regra_contabil, conta_debito, conta_credito, saldo, moeda_lancamento, descricao_regra, descricao_condicao)
		 VALUES (?,?,?,?,?,?,?,?,?,?)
		 ON CONFLICT (produto, dominio, codigo_identificador_boleto, id_regra_contabil, conta_debito, conta_credito)
		 DO UPDATE SET saldo = saldo + excluded.saldo, moeda_lancamento = excluded.moeda_lancamento, descricao_regra = excluded.descricao_regra, descricao_condicao = excluded.descricao_condicao`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, d := range deltas {
		if _, err := stmt.ExecContext(ctx, d.Produto, d.Dominio, d.CodigoIdentificadorBoleto, d.IDRegraContabil, d.ContaDebito, d.ContaCredito, d.Saldo, d.MoedaLancamento, d.DescricaoRegra, d.DescricaoCondicao); err != nil {
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
		if _, err := tx.ExecContext(ctx, "DELETE FROM saldo_conta WHERE produto = ? AND dominio = ?", c.Produto, c.Dominio); err != nil {
			return err
		}
	}
	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO saldo_conta (produto, dominio, codigo_identificador_boleto, id_regra_contabil, conta_debito, conta_credito, saldo, moeda_lancamento, descricao_regra, descricao_condicao)
		 VALUES (?,?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, s := range saldos {
		if _, err := stmt.ExecContext(ctx, s.Produto, s.Dominio, s.CodigoIdentificadorBoleto, s.IDRegraContabil, s.ContaDebito, s.ContaCredito, s.Saldo, s.MoedaLancamento, s.DescricaoRegra, s.DescricaoCondicao); err != nil {
			return err
		}
	}
	return tx.Commit()
}

const colsSaldoSel = "produto, dominio, codigo_identificador_boleto, id_regra_contabil, conta_debito, conta_credito, saldo, IFNULL(moeda_lancamento,''), IFNULL(descricao_regra,''), IFNULL(descricao_condicao,'')"

// saldoWhere monta a cláusula WHERE (com os placeholders e args) a partir do filtro.
func saldoWhere(f model.SaldoFiltro) (string, []interface{}) {
	var where []string
	var args []interface{}
	if f.Produto != "" {
		where = append(where, "produto = ?")
		args = append(args, f.Produto)
	}
	if f.Dominio != "" {
		where = append(where, "dominio = ?")
		args = append(args, f.Dominio)
	}
	if f.Boleto != "" {
		where = append(where, "codigo_identificador_boleto = ?")
		args = append(args, f.Boleto)
	}
	if f.IDRegra != 0 {
		where = append(where, "id_regra_contabil = ?")
		args = append(args, f.IDRegra)
	}
	if f.ContaDebito != "" {
		where = append(where, "conta_debito = ?")
		args = append(args, f.ContaDebito)
	}
	if f.ContaCredito != "" {
		where = append(where, "conta_credito = ?")
		args = append(args, f.ContaCredito)
	}
	if f.Conta != "" {
		where = append(where, "(conta_debito = ? OR conta_credito = ?)")
		args = append(args, f.Conta, f.Conta)
	}
	if len(f.RegrasIn) > 0 {
		ph := make([]string, len(f.RegrasIn))
		for i, id := range f.RegrasIn {
			ph[i] = "?"
			args = append(args, id)
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

func scanSaldos(rows *sql.Rows) ([]model.SaldoConta, error) {
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
	w, args := saldoWhere(f)
	rows, err := r.db.QueryContext(ctx, "SELECT "+colsSaldoSel+" FROM saldo_conta"+w+" ORDER BY produto, dominio, codigo_identificador_boleto, id_regra_contabil", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSaldos(rows)
}

func (r *SaldoContaRepo) ListarPaginado(ctx context.Context, f model.SaldoFiltro, pagina, tamanho int) (*model.SaldoPagina, error) {
	w, args := saldoWhere(f)
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
	q := "SELECT " + colsSaldoSel + " FROM saldo_conta" + w + " ORDER BY produto, dominio, codigo_identificador_boleto, id_regra_contabil LIMIT ? OFFSET ?"
	pArgs := append(append([]interface{}{}, args...), tamanho, (pagina-1)*tamanho)
	rows, err := r.db.QueryContext(ctx, q, pArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out, err := scanSaldos(rows)
	if err != nil {
		return nil, err
	}
	return &model.SaldoPagina{Total: total, Pagina: pagina, Tamanho: tamanho, Saldos: out}, nil
}

func (r *SaldoContaRepo) EstaVazio(ctx context.Context) (bool, error) {
	var n int
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM saldo_conta").Scan(&n); err != nil {
		return false, fmt.Errorf("saldo EstaVazio: %w", err)
	}
	return n == 0, nil
}
