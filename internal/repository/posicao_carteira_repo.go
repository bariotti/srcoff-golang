package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"srcoff/internal/model"
)

// PosicaoCarteiraRepo persiste a posição de carteira no SQL Server usando um
// schema dinâmico: os metadados do lote ficam em colunas fixas e todos os campos
// de negócio ficam serializados como JSON na coluna `campos` (NVARCHAR(MAX)).
// Ver migration 002_posicao_dinamica.sql.
type PosicaoCarteiraRepo struct {
	db *sql.DB
}

func NewPosicaoCarteiraRepo(db *sql.DB) *PosicaoCarteiraRepo {
	return &PosicaoCarteiraRepo{db: db}
}

func (r *PosicaoCarteiraRepo) BuscarPorDataEVersaoMaxima(ctx context.Context, data time.Time) ([]model.PosicaoCarteira, error) {
	dataStr := data.Format("2006-01-02")

	// Busca todas as linhas da data e computa, por produto, a maior versão em Go —
	// o produto está serializado dentro do JSON `campos`, então o filtro por versão
	// máxima por produto não é expresso trivialmente em SQL.
	rows, err := r.db.QueryContext(ctx,
		"SELECT id, data_posicao_carteira, codigo_versao_conteudo, campos FROM posicao_carteira WHERE data_posicao_carteira = @p1 ORDER BY id",
		dataStr,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	doDia, err := scanPosicoes(rows)
	if err != nil {
		return nil, err
	}

	maxPorProduto := map[string]int{}
	for _, p := range doDia {
		prod := produtoDaPosicao(p)
		if p.CodigoVersaoConteudo > maxPorProduto[prod] {
			maxPorProduto[prod] = p.CodigoVersaoConteudo
		}
	}
	var result []model.PosicaoCarteira
	for _, p := range doDia {
		if p.CodigoVersaoConteudo == maxPorProduto[produtoDaPosicao(p)] {
			result = append(result, p)
		}
	}
	return result, nil
}

// produtoDaPosicao lê o campo `produto` de uma posição.
func produtoDaPosicao(p model.PosicaoCarteira) string {
	if v, ok := p.Campos["produto"]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func (r *PosicaoCarteiraRepo) ListarPorData(ctx context.Context, data time.Time) ([]model.PosicaoCarteira, error) {
	dataStr := data.Format("2006-01-02")
	rows, err := r.db.QueryContext(ctx,
		"SELECT id, data_posicao_carteira, codigo_versao_conteudo, campos FROM posicao_carteira WHERE data_posicao_carteira = @p1 ORDER BY codigo_versao_conteudo, id",
		dataStr,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanPosicoes(rows)
}

func (r *PosicaoCarteiraRepo) ListarPorPeriodo(ctx context.Context, dataInicio, dataFim time.Time) ([]model.PosicaoCarteira, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT id, data_posicao_carteira, codigo_versao_conteudo, campos FROM posicao_carteira WHERE data_posicao_carteira >= @p1 AND data_posicao_carteira <= @p2 ORDER BY data_posicao_carteira, codigo_versao_conteudo, id",
		dataInicio.Format("2006-01-02"), dataFim.Format("2006-01-02"),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanPosicoes(rows)
}

// ImportarLote insere um lote de posição (data/versão) serializando os campos
// dinâmicos de cada registro como JSON.
func (r *PosicaoCarteiraRepo) ImportarLote(ctx context.Context, data time.Time, versao int, registros []map[string]interface{}) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	dataStr := data.Format("2006-01-02")
	for _, reg := range registros {
		camposJSON, err := json.Marshal(reg)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO posicao_carteira (data_posicao_carteira, codigo_versao_conteudo, campos) VALUES (@p1, @p2, @p3)",
			dataStr, versao, string(camposJSON),
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// scanPosicoes lê as linhas do novo schema (id, data, versão, campos JSON).
func scanPosicoes(rows *sql.Rows) ([]model.PosicaoCarteira, error) {
	var result []model.PosicaoCarteira
	for rows.Next() {
		var (
			id         int64
			data       time.Time
			versao     int
			camposJSON sql.NullString
		)
		if err := rows.Scan(&id, &data, &versao, &camposJSON); err != nil {
			return nil, err
		}
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
