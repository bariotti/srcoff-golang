package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"srcoff/internal/model"
)

// SaldoContaRepo persiste a projeção de saldo (RN-170) no SQL Server.
type SaldoContaRepo struct {
	db *sql.DB
}

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
	for _, d := range deltas {
		res, err := tx.ExecContext(ctx,
			`UPDATE saldo_conta SET saldo = saldo + @p7, moeda_lancamento=@p8, descricao_regra=@p9, descricao_condicao=@p10
			 WHERE produto=@p1 AND dominio=@p2 AND codigo_identificador_boleto=@p3 AND id_regra_contabil=@p4 AND conta_debito=@p5 AND conta_credito=@p6`,
			d.Produto, d.Dominio, d.CodigoIdentificadorBoleto, d.IDRegraContabil, d.ContaDebito, d.ContaCredito, d.Saldo, d.MoedaLancamento, d.DescricaoRegra, d.DescricaoCondicao)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO saldo_conta (produto, dominio, codigo_identificador_boleto, id_regra_contabil, conta_debito, conta_credito, saldo, moeda_lancamento, descricao_regra, descricao_condicao)
				 VALUES (@p1,@p2,@p3,@p4,@p5,@p6,@p7,@p8,@p9,@p10)`,
				d.Produto, d.Dominio, d.CodigoIdentificadorBoleto, d.IDRegraContabil, d.ContaDebito, d.ContaCredito, d.Saldo, d.MoedaLancamento, d.DescricaoRegra, d.DescricaoCondicao); err != nil {
				return err
			}
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
	for _, s := range saldos {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO saldo_conta (produto, dominio, codigo_identificador_boleto, id_regra_contabil, conta_debito, conta_credito, saldo, moeda_lancamento, descricao_regra, descricao_condicao)
			 VALUES (@p1,@p2,@p3,@p4,@p5,@p6,@p7,@p8,@p9,@p10)`,
			s.Produto, s.Dominio, s.CodigoIdentificadorBoleto, s.IDRegraContabil, s.ContaDebito, s.ContaCredito, s.Saldo, s.MoedaLancamento, s.DescricaoRegra, s.DescricaoCondicao); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *SaldoContaRepo) Listar(ctx context.Context, f model.SaldoFiltro) ([]model.SaldoConta, error) {
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
	q := "SELECT produto, dominio, codigo_identificador_boleto, id_regra_contabil, conta_debito, conta_credito, saldo, ISNULL(moeda_lancamento,''), ISNULL(descricao_regra,''), ISNULL(descricao_condicao,'') FROM saldo_conta"
	if len(where) > 0 {
		q += " WHERE " + strings.Join(where, " AND ")
	}
	q += " ORDER BY produto, dominio, codigo_identificador_boleto, id_regra_contabil"
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
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

func (r *SaldoContaRepo) EstaVazio(ctx context.Context) (bool, error) {
	var n int
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM saldo_conta").Scan(&n); err != nil {
		return false, err
	}
	return n == 0, nil
}
