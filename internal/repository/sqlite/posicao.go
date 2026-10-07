package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"srcoff/internal/model"
)

// PosicaoCarteiraRepo implementa repository.PosicaoCarteiraRepository em SQLite.
// Schema dinâmico: metadados em colunas fixas e campos de negócio em JSON (coluna `campos`).
type PosicaoCarteiraRepo struct{ db *sql.DB }

func NewPosicaoCarteiraRepo(db *sql.DB) *PosicaoCarteiraRepo { return &PosicaoCarteiraRepo{db: db} }

func (r *PosicaoCarteiraRepo) BuscarPorDataEVersaoMaxima(ctx context.Context, data time.Time) ([]model.PosicaoCarteira, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT id, data_posicao_carteira, codigo_versao_conteudo, campos FROM posicao_carteira WHERE data_posicao_carteira = ? ORDER BY id",
		fmtData(data),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	doDia, err := scanPosicoes(rows)
	if err != nil {
		return nil, err
	}
	// Maior versão por (produto, domínio) — o produto está dentro do JSON, então o
	// filtro de versão máxima por combinação é computado em Go.
	maxPorCombo := map[string]int{}
	for _, p := range doDia {
		k := comboKeyPos(p)
		if p.CodigoVersaoConteudo > maxPorCombo[k] {
			maxPorCombo[k] = p.CodigoVersaoConteudo
		}
	}
	var result []model.PosicaoCarteira
	for _, p := range doDia {
		if p.CodigoVersaoConteudo == maxPorCombo[comboKeyPos(p)] {
			result = append(result, p)
		}
	}
	return result, nil
}

func campoStr(p model.PosicaoCarteira, campo string) string {
	if v, ok := p.Campos[campo]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func comboKeyPos(p model.PosicaoCarteira) string {
	return campoStr(p, "produto") + "\x00" + campoStr(p, "dominio")
}

func (r *PosicaoCarteiraRepo) ListarPorData(ctx context.Context, data time.Time) ([]model.PosicaoCarteira, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT id, data_posicao_carteira, codigo_versao_conteudo, campos FROM posicao_carteira WHERE data_posicao_carteira = ? ORDER BY codigo_versao_conteudo, id",
		fmtData(data),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanPosicoes(rows)
}

func (r *PosicaoCarteiraRepo) ListarPorPeriodo(ctx context.Context, dataInicio, dataFim time.Time) ([]model.PosicaoCarteira, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT id, data_posicao_carteira, codigo_versao_conteudo, campos FROM posicao_carteira WHERE data_posicao_carteira >= ? AND data_posicao_carteira <= ? ORDER BY data_posicao_carteira, codigo_versao_conteudo, id",
		fmtData(dataInicio), fmtData(dataFim),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanPosicoes(rows)
}

func (r *PosicaoCarteiraRepo) ImportarLote(ctx context.Context, data time.Time, versao int, registros []map[string]interface{}) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	dataStr := fmtData(data)
	for _, reg := range registros {
		camposJSON, err := json.Marshal(reg)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO posicao_carteira (data_posicao_carteira, codigo_versao_conteudo, campos) VALUES (?,?,?)",
			dataStr, versao, string(camposJSON),
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func scanPosicoes(rows *sql.Rows) ([]model.PosicaoCarteira, error) {
	var result []model.PosicaoCarteira
	for rows.Next() {
		var (
			id         int64
			dataStr    string
			versao     int
			camposJSON sql.NullString
		)
		if err := rows.Scan(&id, &dataStr, &versao, &camposJSON); err != nil {
			return nil, err
		}
		data := parseData(dataStr)
		campos := map[string]interface{}{}
		if camposJSON.Valid && camposJSON.String != "" {
			if err := json.Unmarshal([]byte(camposJSON.String), &campos); err != nil {
				return nil, err
			}
		}
		campos["id"] = float64(id)
		campos["codigo_versao_conteudo"] = float64(versao)
		campos["data_posicao_carteira"] = data
		result = append(result, model.PosicaoCarteira{
			ID:                   id,
			DataPosicaoCarteira:  data,
			CodigoVersaoConteudo: versao,
			Campos:               campos,
		})
	}
	return result, rows.Err()
}
