package sqlite

import (
	"context"
	"database/sql"

	"srcoff/internal/model"
)

// RegraContabilRepo implementa repository.RegraContabilRepository em SQLite.
type RegraContabilRepo struct{ db *sql.DB }

func NewRegraContabilRepo(db *sql.DB) *RegraContabilRepo { return &RegraContabilRepo{db: db} }

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

func (r *RegraContabilRepo) ListarRegrasAtivas(ctx context.Context) ([]model.RegraContabil, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT id, descricao, IFNULL(codigo_produto_corporativo,''), IFNULL(dominio,''), IFNULL(campo_produto,''), IFNULL(pre_condicao,''), ativo, IFNULL(tipo_lancamento,'reverte') FROM regra_contabil WHERE ativo = 1 ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var regras []model.RegraContabil
	for rows.Next() {
		var reg model.RegraContabil
		if err := rows.Scan(&reg.ID, &reg.Descricao, &reg.CodigoProdutoCorporativo, &reg.Dominio, &reg.CampoProduto, &reg.PreCondicao, &reg.Ativo, &reg.TipoLancamento); err != nil {
			return nil, err
		}
		regras = append(regras, reg)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range regras {
		cond, err := r.ListarCondicoes(ctx, regras[i].ID)
		if err != nil {
			return nil, err
		}
		regras[i].Condicoes = cond
	}
	return regras, nil
}

func (r *RegraContabilRepo) CriarRegra(ctx context.Context, regra model.RegraContabil) (int64, error) {
	res, err := r.db.ExecContext(ctx,
		"INSERT INTO regra_contabil (descricao, codigo_produto_corporativo, dominio, campo_produto, pre_condicao, ativo, tipo_lancamento) VALUES (?,?,?,?,?,1,?)",
		regra.Descricao, regra.CodigoProdutoCorporativo, regra.Dominio, regra.CampoProduto, regra.PreCondicao, model.NormalizaTipoLancamento(regra.TipoLancamento),
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (r *RegraContabilRepo) EditarRegra(ctx context.Context, regra model.RegraContabil) error {
	_, err := r.db.ExecContext(ctx,
		"UPDATE regra_contabil SET descricao=?, codigo_produto_corporativo=?, dominio=?, campo_produto=?, pre_condicao=?, ativo=?, tipo_lancamento=? WHERE id=?",
		regra.Descricao, regra.CodigoProdutoCorporativo, regra.Dominio, regra.CampoProduto, regra.PreCondicao, b2i(regra.Ativo), model.NormalizaTipoLancamento(regra.TipoLancamento), regra.ID,
	)
	return err
}

func (r *RegraContabilRepo) ExcluirRegra(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, "UPDATE regra_contabil SET ativo = 0 WHERE id = ?", id)
	return err
}

func (r *RegraContabilRepo) ListarCondicoes(ctx context.Context, idRegra int64) ([]model.CondicaoRegra, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT id, id_regra, condicao, conta_debito, conta_credito, campo_valor, campo_moeda, IFNULL(campo_boleto,''), ativo FROM condicao_regra WHERE id_regra = ? AND ativo = 1 ORDER BY id",
		idRegra,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var condicoes []model.CondicaoRegra
	for rows.Next() {
		var c model.CondicaoRegra
		if err := rows.Scan(&c.ID, &c.IDRegra, &c.Condicao, &c.ContaDebito, &c.ContaCredito, &c.CampoValor, &c.CampoMoeda, &c.CampoBoleto, &c.Ativo); err != nil {
			return nil, err
		}
		condicoes = append(condicoes, c)
	}
	return condicoes, rows.Err()
}

func (r *RegraContabilRepo) CriarCondicao(ctx context.Context, condicao model.CondicaoRegra) (int64, error) {
	res, err := r.db.ExecContext(ctx,
		"INSERT INTO condicao_regra (id_regra, condicao, conta_debito, conta_credito, campo_valor, campo_moeda, campo_boleto, ativo) VALUES (?,?,?,?,?,?,?,1)",
		condicao.IDRegra, condicao.Condicao, condicao.ContaDebito, condicao.ContaCredito, condicao.CampoValor, condicao.CampoMoeda, condicao.CampoBoleto,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (r *RegraContabilRepo) EditarCondicao(ctx context.Context, condicao model.CondicaoRegra) error {
	// Ativo é preservado: editar atualiza os campos, não (des)ativa a condição.
	_, err := r.db.ExecContext(ctx,
		"UPDATE condicao_regra SET condicao=?, conta_debito=?, conta_credito=?, campo_valor=?, campo_moeda=?, campo_boleto=? WHERE id=?",
		condicao.Condicao, condicao.ContaDebito, condicao.ContaCredito, condicao.CampoValor, condicao.CampoMoeda, condicao.CampoBoleto, condicao.ID,
	)
	return err
}

func (r *RegraContabilRepo) ExcluirCondicao(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, "UPDATE condicao_regra SET ativo = 0 WHERE id = ?", id)
	return err
}
