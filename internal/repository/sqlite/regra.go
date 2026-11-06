package sqlite

import (
	"context"
	"database/sql"
	"strings"

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

const colsRegra = "id, descricao, IFNULL(codigo_produto_corporativo,''), IFNULL(dominio,''), " +
	"IFNULL(campo_produto,''), IFNULL(pre_condicao,''), ativo, IFNULL(status,'ativa'), IFNULL(tipo_lancamento,'reverte')"

func scanRegra(rows *sql.Rows) (model.RegraContabil, error) {
	var reg model.RegraContabil
	err := rows.Scan(&reg.ID, &reg.Descricao, &reg.CodigoProdutoCorporativo, &reg.Dominio,
		&reg.CampoProduto, &reg.PreCondicao, &reg.Ativo, &reg.Status, &reg.TipoLancamento)
	return reg, err
}

// listarRegras carrega as regras do WHERE informado e embute as condições: as ATIVAS
// (geração) ou as VISÍVEIS/não-excluídas (UI), conforme comRascunho.
func (r *RegraContabilRepo) listarRegras(ctx context.Context, where string, comRascunho bool) ([]model.RegraContabil, error) {
	rows, err := r.db.QueryContext(ctx, "SELECT "+colsRegra+" FROM regra_contabil WHERE "+where+" ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var regras []model.RegraContabil
	for rows.Next() {
		reg, err := scanRegra(rows)
		if err != nil {
			return nil, err
		}
		regras = append(regras, reg)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range regras {
		var cond []model.CondicaoRegra
		if comRascunho {
			cond, err = r.ListarCondicoesVisiveis(ctx, regras[i].ID)
		} else {
			cond, err = r.ListarCondicoes(ctx, regras[i].ID)
		}
		if err != nil {
			return nil, err
		}
		regras[i].Condicoes = cond
	}
	return regras, nil
}

// ListarRegrasAtivas: só regras ATIVAS + condições ATIVAS (porta da geração).
func (r *RegraContabilRepo) ListarRegrasAtivas(ctx context.Context) ([]model.RegraContabil, error) {
	return r.listarRegras(ctx, "ativo = 1", false)
}

// ListarRegras: regras VISÍVEIS na UI (ativa + rascunho) + condições visíveis.
func (r *RegraContabilRepo) ListarRegras(ctx context.Context) ([]model.RegraContabil, error) {
	return r.listarRegras(ctx, "IFNULL(status,'ativa') <> 'excluida'", true)
}

func (r *RegraContabilRepo) CriarRegra(ctx context.Context, regra model.RegraContabil) (int64, error) {
	status := model.NormalizaStatus(regra.Status, true) // sem status informado → 'ativa'
	res, err := r.db.ExecContext(ctx,
		"INSERT INTO regra_contabil (descricao, codigo_produto_corporativo, dominio, campo_produto, pre_condicao, ativo, status, tipo_lancamento) VALUES (?,?,?,?,?,?,?,?)",
		regra.Descricao, regra.CodigoProdutoCorporativo, regra.Dominio, regra.CampoProduto, regra.PreCondicao,
		b2i(model.AtivoDoStatus(status)), status, model.NormalizaTipoLancamento(regra.TipoLancamento),
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (r *RegraContabilRepo) EditarRegra(ctx context.Context, regra model.RegraContabil) error {
	// Sem status informado → edição genérica de campos, preservando status/ativo atuais.
	if strings.TrimSpace(regra.Status) == "" {
		_, err := r.db.ExecContext(ctx,
			"UPDATE regra_contabil SET descricao=?, codigo_produto_corporativo=?, dominio=?, campo_produto=?, pre_condicao=?, tipo_lancamento=? WHERE id=?",
			regra.Descricao, regra.CodigoProdutoCorporativo, regra.Dominio, regra.CampoProduto, regra.PreCondicao, model.NormalizaTipoLancamento(regra.TipoLancamento), regra.ID,
		)
		return err
	}
	status := model.NormalizaStatus(regra.Status, regra.Ativo)
	_, err := r.db.ExecContext(ctx,
		"UPDATE regra_contabil SET descricao=?, codigo_produto_corporativo=?, dominio=?, campo_produto=?, pre_condicao=?, ativo=?, status=?, tipo_lancamento=? WHERE id=?",
		regra.Descricao, regra.CodigoProdutoCorporativo, regra.Dominio, regra.CampoProduto, regra.PreCondicao,
		b2i(model.AtivoDoStatus(status)), status, model.NormalizaTipoLancamento(regra.TipoLancamento), regra.ID,
	)
	return err
}

func (r *RegraContabilRepo) ExcluirRegra(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, "UPDATE regra_contabil SET ativo = 0, status = 'excluida' WHERE id = ?", id)
	return err
}

const colsCond = "id, id_regra, condicao, conta_debito, conta_credito, campo_valor, campo_moeda, IFNULL(campo_boleto,''), " +
	"ativo, IFNULL(status,'ativa'), IFNULL(base_na_posicao,0), IFNULL(campo_valor_moeda_base,''), IFNULL(campo_moeda_base,''), IFNULL(campo_data_conversao,'')"

func scanCond(rows *sql.Rows) (model.CondicaoRegra, error) {
	var c model.CondicaoRegra
	var base int
	err := rows.Scan(&c.ID, &c.IDRegra, &c.Condicao, &c.ContaDebito, &c.ContaCredito, &c.CampoValor, &c.CampoMoeda,
		&c.CampoBoleto, &c.Ativo, &c.Status, &base, &c.CampoValorMoedaBase, &c.CampoMoedaBase, &c.CampoDataConversao)
	c.BaseNaPosicao = base != 0
	return c, err
}

func (r *RegraContabilRepo) listarCondicoes(ctx context.Context, idRegra int64, where string) ([]model.CondicaoRegra, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT "+colsCond+" FROM condicao_regra WHERE id_regra = ? AND "+where+" ORDER BY id", idRegra)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var condicoes []model.CondicaoRegra
	for rows.Next() {
		c, err := scanCond(rows)
		if err != nil {
			return nil, err
		}
		condicoes = append(condicoes, c)
	}
	return condicoes, rows.Err()
}

// ListarCondicoes: só condições ATIVAS (usada pela geração, via ListarRegrasAtivas).
func (r *RegraContabilRepo) ListarCondicoes(ctx context.Context, idRegra int64) ([]model.CondicaoRegra, error) {
	return r.listarCondicoes(ctx, idRegra, "ativo = 1")
}

// ListarCondicoesVisiveis: condições VISÍVEIS na UI (ativa + rascunho).
func (r *RegraContabilRepo) ListarCondicoesVisiveis(ctx context.Context, idRegra int64) ([]model.CondicaoRegra, error) {
	return r.listarCondicoes(ctx, idRegra, "IFNULL(status,'ativa') <> 'excluida'")
}

func (r *RegraContabilRepo) CriarCondicao(ctx context.Context, condicao model.CondicaoRegra) (int64, error) {
	status := model.NormalizaStatus(condicao.Status, true) // sem status → 'ativa'
	res, err := r.db.ExecContext(ctx,
		"INSERT INTO condicao_regra (id_regra, condicao, conta_debito, conta_credito, campo_valor, campo_moeda, campo_boleto, ativo, status, base_na_posicao, campo_valor_moeda_base, campo_moeda_base, campo_data_conversao) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)",
		condicao.IDRegra, condicao.Condicao, condicao.ContaDebito, condicao.ContaCredito, condicao.CampoValor, condicao.CampoMoeda, condicao.CampoBoleto,
		b2i(model.AtivoDoStatus(status)), status, b2i(condicao.BaseNaPosicao), condicao.CampoValorMoedaBase, condicao.CampoMoedaBase, condicao.CampoDataConversao,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (r *RegraContabilRepo) EditarCondicao(ctx context.Context, condicao model.CondicaoRegra) error {
	// Sem status informado → edita só os campos, preservando status/ativo atuais.
	if strings.TrimSpace(condicao.Status) == "" {
		_, err := r.db.ExecContext(ctx,
			"UPDATE condicao_regra SET condicao=?, conta_debito=?, conta_credito=?, campo_valor=?, campo_moeda=?, campo_boleto=?, base_na_posicao=?, campo_valor_moeda_base=?, campo_moeda_base=?, campo_data_conversao=? WHERE id=?",
			condicao.Condicao, condicao.ContaDebito, condicao.ContaCredito, condicao.CampoValor, condicao.CampoMoeda, condicao.CampoBoleto, b2i(condicao.BaseNaPosicao), condicao.CampoValorMoedaBase, condicao.CampoMoedaBase, condicao.CampoDataConversao, condicao.ID,
		)
		return err
	}
	status := model.NormalizaStatus(condicao.Status, condicao.Ativo)
	_, err := r.db.ExecContext(ctx,
		"UPDATE condicao_regra SET condicao=?, conta_debito=?, conta_credito=?, campo_valor=?, campo_moeda=?, campo_boleto=?, ativo=?, status=?, base_na_posicao=?, campo_valor_moeda_base=?, campo_moeda_base=?, campo_data_conversao=? WHERE id=?",
		condicao.Condicao, condicao.ContaDebito, condicao.ContaCredito, condicao.CampoValor, condicao.CampoMoeda, condicao.CampoBoleto,
		b2i(model.AtivoDoStatus(status)), status, b2i(condicao.BaseNaPosicao), condicao.CampoValorMoedaBase, condicao.CampoMoedaBase, condicao.CampoDataConversao, condicao.ID,
	)
	return err
}

func (r *RegraContabilRepo) ExcluirCondicao(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, "UPDATE condicao_regra SET ativo = 0, status = 'excluida' WHERE id = ?", id)
	return err
}
