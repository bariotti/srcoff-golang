package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"srcoff/internal/model"
)

// RegraVersaoRepo persiste o histórico de versões do cadastro de regras (RF-133) em SQLite.
// Recurso sqlite-first: nos backends file/sqlserver não há este repositório (feature off).
type RegraVersaoRepo struct{ db *sql.DB }

func NewRegraVersaoRepo(db *sql.DB) *RegraVersaoRepo { return &RegraVersaoRepo{db: db} }

// Carregar lê o estado ATUAL de uma regra (qualquer status) com suas condições visíveis
// (ativa + rascunho). ok=false quando a regra não existe. É a base de cada snapshot.
func (r *RegraVersaoRepo) Carregar(ctx context.Context, idRegra int64) (model.RegraContabil, bool, error) {
	rows, err := r.db.QueryContext(ctx, "SELECT "+colsRegra+" FROM regra_contabil WHERE id = ?", idRegra)
	if err != nil {
		return model.RegraContabil{}, false, err
	}
	var reg model.RegraContabil
	ok := false
	if rows.Next() {
		reg, err = scanRegra(rows)
		if err != nil {
			rows.Close()
			return model.RegraContabil{}, false, err
		}
		ok = true
	}
	rows.Close()
	if !ok {
		return model.RegraContabil{}, false, nil
	}

	crows, err := r.db.QueryContext(ctx,
		"SELECT "+colsCond+" FROM condicao_regra WHERE id_regra = ? AND IFNULL(status,'ativa') <> 'excluida' ORDER BY id", idRegra)
	if err != nil {
		return model.RegraContabil{}, false, err
	}
	defer crows.Close()
	reg.Condicoes = []model.CondicaoRegra{}
	for crows.Next() {
		c, err := scanCond(crows)
		if err != nil {
			return model.RegraContabil{}, false, err
		}
		reg.Condicoes = append(reg.Condicoes, c)
	}
	return reg, true, crows.Err()
}

// UltimaVersao devolve a versão mais recente armazenada da regra. ok=false quando ainda não há
// nenhuma (regra sem histórico). O snapshot vem decodificado em RegraVersao.Regra.
func (r *RegraVersaoRepo) UltimaVersao(ctx context.Context, idRegra int64) (model.RegraVersao, bool, error) {
	row := r.db.QueryRowContext(ctx,
		"SELECT id, id_regra, versao, resumo, IFNULL(origem,'sistema'), snapshot, IFNULL(criado_em,'') FROM regra_versao WHERE id_regra = ? ORDER BY versao DESC LIMIT 1", idRegra)
	v, err := scanVersao(row)
	if err == sql.ErrNoRows {
		return model.RegraVersao{}, false, nil
	}
	if err != nil {
		return model.RegraVersao{}, false, err
	}
	return v, true, nil
}

// Snapshot grava uma nova versão da regra (numeração sequencial por regra) e devolve o número
// atribuído. O snapshot é a RegraContabil completa serializada em JSON.
func (r *RegraVersaoRepo) Snapshot(ctx context.Context, idRegra int64, regra model.RegraContabil, resumo, origem string) (int, error) {
	if origem == "" {
		origem = "sistema"
	}
	blob, err := json.Marshal(regra)
	if err != nil {
		return 0, err
	}
	var prox int
	if err := r.db.QueryRowContext(ctx, "SELECT IFNULL(MAX(versao),0)+1 FROM regra_versao WHERE id_regra = ?", idRegra).Scan(&prox); err != nil {
		return 0, err
	}
	_, err = r.db.ExecContext(ctx,
		"INSERT INTO regra_versao (id_regra, versao, resumo, origem, snapshot, criado_em) VALUES (?,?,?,?,?,?)",
		idRegra, prox, resumo, origem, string(blob), fmtTS(time.Now()),
	)
	if err != nil {
		return 0, err
	}
	return prox, nil
}

// Listar devolve as versões da regra (mais recente primeiro). A versão de maior número é
// marcada como Ativa (reflete o estado vigente — toda restauração anexa uma nova versão).
func (r *RegraVersaoRepo) Listar(ctx context.Context, idRegra int64) ([]model.RegraVersao, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT id, id_regra, versao, resumo, IFNULL(origem,'sistema'), snapshot, IFNULL(criado_em,'') FROM regra_versao WHERE id_regra = ? ORDER BY versao DESC", idRegra)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.RegraVersao
	maxVersao := 0
	for rows.Next() {
		v, err := scanVersao(rows)
		if err != nil {
			return nil, err
		}
		if v.Versao > maxVersao {
			maxVersao = v.Versao
		}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		out[i].Ativa = out[i].Versao == maxVersao
	}
	return out, nil
}

// ResumoTodos devolve a ÚLTIMA versão de cada regra (sem o snapshot pesado), para a lista de
// regras exibir um selo "vN · alterada em ...". Só vêm id_regra, versao, resumo e criado_em.
func (r *RegraVersaoRepo) ResumoTodos(ctx context.Context) ([]model.RegraVersao, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT rv.id_regra, rv.versao, rv.resumo, IFNULL(rv.criado_em,'')
		FROM regra_versao rv
		JOIN (SELECT id_regra, MAX(versao) mx FROM regra_versao GROUP BY id_regra) m
		  ON m.id_regra = rv.id_regra AND m.mx = rv.versao`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.RegraVersao
	for rows.Next() {
		var v model.RegraVersao
		if err := rows.Scan(&v.IDRegra, &v.Versao, &v.Resumo, &v.CriadoEm); err != nil {
			return nil, err
		}
		v.Ativa = true
		out = append(out, v)
	}
	return out, rows.Err()
}

// Obter devolve uma versão específica da regra (com o snapshot decodificado). ok=false se não existe.
func (r *RegraVersaoRepo) Obter(ctx context.Context, idRegra int64, versao int) (model.RegraVersao, bool, error) {
	row := r.db.QueryRowContext(ctx,
		"SELECT id, id_regra, versao, resumo, IFNULL(origem,'sistema'), snapshot, IFNULL(criado_em,'') FROM regra_versao WHERE id_regra = ? AND versao = ?", idRegra, versao)
	v, err := scanVersao(row)
	if err == sql.ErrNoRows {
		return model.RegraVersao{}, false, nil
	}
	if err != nil {
		return model.RegraVersao{}, false, err
	}
	return v, true, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanVersao(row rowScanner) (model.RegraVersao, error) {
	var v model.RegraVersao
	var blob string
	if err := row.Scan(&v.ID, &v.IDRegra, &v.Versao, &v.Resumo, &v.Origem, &blob, &v.CriadoEm); err != nil {
		return model.RegraVersao{}, err
	}
	_ = json.Unmarshal([]byte(blob), &v.Regra)
	return v, nil
}
