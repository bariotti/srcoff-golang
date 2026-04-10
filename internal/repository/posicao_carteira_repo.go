package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"srcoff/internal/model"
)

type PosicaoCarteiraRepo struct {
	db *sql.DB
}

func NewPosicaoCarteiraRepo(db *sql.DB) *PosicaoCarteiraRepo {
	return &PosicaoCarteiraRepo{db: db}
}

func (r *PosicaoCarteiraRepo) BuscarPorDataEVersaoMaxima(ctx context.Context, data time.Time) ([]model.PosicaoCarteira, error) {
	dataStr := data.Format("2006-01-02")

	var maxVersao int
	err := r.db.QueryRowContext(ctx,
		"SELECT ISNULL(MAX(codigo_versao_conteudo), 0) FROM posicao_carteira WHERE data_posicao_carteira = '"+dataStr+"'",
	).Scan(&maxVersao)
	if err != nil {
		return nil, err
	}

	rows, err := r.db.QueryContext(ctx,
		"SELECT id, data_posicao_carteira, codigo_versao_conteudo, codigo_identificador_boleto, descricao_veiculo, indicador_contraparte_afiliada, valor_mtm, principal_remanescente, moeda_principal_remanescente FROM posicao_carteira WHERE data_posicao_carteira = '"+dataStr+"' AND codigo_versao_conteudo = "+fmt.Sprintf("%d", maxVersao),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := []model.PosicaoCarteira{}
	for rows.Next() {
		var p model.PosicaoCarteira
		err := rows.Scan(
			&p.ID,
			&p.DataPosicaoCarteira,
			&p.CodigoVersaoConteudo,
			&p.CodigoIdentificadorBoleto,
			&p.DescricaoVeiculo,
			&p.IndicadorContraparteAfiliada,
			&p.ValorMTM,
			&p.PrincipalRemanescente,
			&p.MoedaPrincipalRemanescente,
		)
		if err != nil {
			return nil, err
		}
		result = append(result, p)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return result, nil
}
