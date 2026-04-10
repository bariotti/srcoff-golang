package repository

import (
	"context"
	"database/sql"

	"srcoff/internal/model"
)

type RegraContabilRepo struct {
	db *sql.DB
}

func NewRegraContabilRepo(db *sql.DB) *RegraContabilRepo {
	return &RegraContabilRepo{db: db}
}

func (r *RegraContabilRepo) ListarRegrasAtivas(ctx context.Context) ([]model.RegraContabil, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, descricao, codigo_produto_corporativo, ativo
		FROM regra_contabil
		WHERE ativo = 1
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var regras []model.RegraContabil
	for rows.Next() {
		var reg model.RegraContabil
		var ativo int
		if err := rows.Scan(&reg.ID, &reg.Descricao, &reg.CodigoProdutoCorporativo, &ativo); err != nil {
			return nil, err
		}
		reg.Ativo = ativo == 1
		regras = append(regras, reg)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for i := range regras {
		condicoes, err := r.ListarCondicoes(ctx, regras[i].ID)
		if err != nil {
			return nil, err
		}
		regras[i].Condicoes = condicoes
	}

	return regras, nil
}

func (r *RegraContabilRepo) CriarRegra(ctx context.Context, regra model.RegraContabil) (int64, error) {
	var id int64
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO regra_contabil (descricao, codigo_produto_corporativo, ativo)
		VALUES (@descricao, @codigo, 1);
		SELECT SCOPE_IDENTITY()
	`,
		sql.Named("descricao", regra.Descricao),
		sql.Named("codigo", regra.CodigoProdutoCorporativo),
	).Scan(&id)
	if err != nil {
		return 0, err
	}
	return id, nil
}

func (r *RegraContabilRepo) EditarRegra(ctx context.Context, regra model.RegraContabil) error {
	ativo := 0
	if regra.Ativo {
		ativo = 1
	}
	_, err := r.db.ExecContext(ctx, `
		UPDATE regra_contabil
		SET descricao = @descricao,
		    codigo_produto_corporativo = @codigo,
		    ativo = @ativo
		WHERE id = @id
	`,
		sql.Named("descricao", regra.Descricao),
		sql.Named("codigo", regra.CodigoProdutoCorporativo),
		sql.Named("ativo", ativo),
		sql.Named("id", regra.ID),
	)
	return err
}

func (r *RegraContabilRepo) ListarCondicoes(ctx context.Context, idRegra int64) ([]model.CondicaoRegra, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, id_regra, condicao, conta_debito, conta_credito, campo_valor, campo_moeda, ativo
		FROM condicao_regra
		WHERE id_regra = @idRegra AND ativo = 1
	`, sql.Named("idRegra", idRegra))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var condicoes []model.CondicaoRegra
	for rows.Next() {
		var c model.CondicaoRegra
		var ativo int
		if err := rows.Scan(&c.ID, &c.IDRegra, &c.Condicao, &c.ContaDebito, &c.ContaCredito, &c.CampoValor, &c.CampoMoeda, &ativo); err != nil {
			return nil, err
		}
		c.Ativo = ativo == 1
		condicoes = append(condicoes, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return condicoes, nil
}

func (r *RegraContabilRepo) CriarCondicao(ctx context.Context, condicao model.CondicaoRegra) (int64, error) {
	var id int64
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO condicao_regra (id_regra, condicao, conta_debito, conta_credito, campo_valor, campo_moeda, ativo)
		VALUES (@idRegra, @condicao, @contaDebito, @contaCredito, @campoValor, @campoMoeda, 1);
		SELECT SCOPE_IDENTITY()
	`,
		sql.Named("idRegra", condicao.IDRegra),
		sql.Named("condicao", condicao.Condicao),
		sql.Named("contaDebito", condicao.ContaDebito),
		sql.Named("contaCredito", condicao.ContaCredito),
		sql.Named("campoValor", condicao.CampoValor),
		sql.Named("campoMoeda", condicao.CampoMoeda),
	).Scan(&id)
	if err != nil {
		return 0, err
	}
	return id, nil
}

func (r *RegraContabilRepo) EditarCondicao(ctx context.Context, condicao model.CondicaoRegra) error {
	ativo := 0
	if condicao.Ativo {
		ativo = 1
	}
	_, err := r.db.ExecContext(ctx, `
		UPDATE condicao_regra
		SET condicao = @condicao,
		    conta_debito = @contaDebito,
		    conta_credito = @contaCredito,
		    campo_valor = @campoValor,
		    campo_moeda = @campoMoeda,
		    ativo = @ativo
		WHERE id = @id
	`,
		sql.Named("condicao", condicao.Condicao),
		sql.Named("contaDebito", condicao.ContaDebito),
		sql.Named("contaCredito", condicao.ContaCredito),
		sql.Named("campoValor", condicao.CampoValor),
		sql.Named("campoMoeda", condicao.CampoMoeda),
		sql.Named("ativo", ativo),
		sql.Named("id", condicao.ID),
	)
	return err
}
