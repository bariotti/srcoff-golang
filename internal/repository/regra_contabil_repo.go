package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"srcoff/internal/model"
)

type RegraContabilRepo struct {
	db *sql.DB
}

func NewRegraContabilRepo(db *sql.DB) *RegraContabilRepo {
	return &RegraContabilRepo{db: db}
}

func (r *RegraContabilRepo) ListarRegrasAtivas(ctx context.Context) ([]model.RegraContabil, error) {
	rows, err := r.db.QueryContext(ctx, "SELECT id, descricao, codigo_produto_corporativo, ativo FROM regra_contabil WHERE ativo = 1")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var regras []model.RegraContabil
	for rows.Next() {
		var reg model.RegraContabil
		if err := rows.Scan(&reg.ID, &reg.Descricao, &reg.CodigoProdutoCorporativo, &reg.Ativo); err != nil {
			return nil, err
		}
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

func esc(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

func (r *RegraContabilRepo) CriarRegra(ctx context.Context, regra model.RegraContabil) (int64, error) {
	var id int64
	err := r.db.QueryRowContext(ctx,
		"INSERT INTO regra_contabil (descricao, codigo_produto_corporativo, ativo) VALUES ('"+esc(regra.Descricao)+"', '"+esc(regra.CodigoProdutoCorporativo)+"', 1); SELECT SCOPE_IDENTITY()",
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
	_, err := r.db.ExecContext(ctx,
		fmt.Sprintf("UPDATE regra_contabil SET descricao = '%s', codigo_produto_corporativo = '%s', ativo = %d WHERE id = %d",
			esc(regra.Descricao), esc(regra.CodigoProdutoCorporativo), ativo, regra.ID),
	)
	return err
}

func (r *RegraContabilRepo) ListarCondicoes(ctx context.Context, idRegra int64) ([]model.CondicaoRegra, error) {
	rows, err := r.db.QueryContext(ctx,
		fmt.Sprintf("SELECT id, id_regra, condicao, conta_debito, conta_credito, campo_valor, campo_moeda, ativo FROM condicao_regra WHERE id_regra = %d AND ativo = 1", idRegra),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var condicoes []model.CondicaoRegra
	for rows.Next() {
		var c model.CondicaoRegra
		if err := rows.Scan(&c.ID, &c.IDRegra, &c.Condicao, &c.ContaDebito, &c.ContaCredito, &c.CampoValor, &c.CampoMoeda, &c.Ativo); err != nil {
			return nil, err
		}
		condicoes = append(condicoes, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return condicoes, nil
}

func (r *RegraContabilRepo) CriarCondicao(ctx context.Context, condicao model.CondicaoRegra) (int64, error) {
	var id int64
	err := r.db.QueryRowContext(ctx,
		fmt.Sprintf("INSERT INTO condicao_regra (id_regra, condicao, conta_debito, conta_credito, campo_valor, campo_moeda, ativo) VALUES (%d, '%s', '%s', '%s', '%s', '%s', 1); SELECT SCOPE_IDENTITY()",
			condicao.IDRegra, esc(condicao.Condicao), esc(condicao.ContaDebito), esc(condicao.ContaCredito), esc(condicao.CampoValor), esc(condicao.CampoMoeda)),
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
	_, err := r.db.ExecContext(ctx,
		fmt.Sprintf("UPDATE condicao_regra SET condicao = '%s', conta_debito = '%s', conta_credito = '%s', campo_valor = '%s', campo_moeda = '%s', ativo = %d WHERE id = %d",
			esc(condicao.Condicao), esc(condicao.ContaDebito), esc(condicao.ContaCredito), esc(condicao.CampoValor), esc(condicao.CampoMoeda), ativo, condicao.ID),
	)
	return err
}

func (r *RegraContabilRepo) ExcluirCondicao(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx,
		fmt.Sprintf("UPDATE condicao_regra SET ativo = 0 WHERE id = %d", id),
	)
	return err
}
