package sqlite

import (
	"context"
	"database/sql"

	"srcoff/internal/model"
)

// PadraoArquivoRepo implementa repository.PadraoArquivoRepository em SQLite.
type PadraoArquivoRepo struct{ db *sql.DB }

func NewPadraoArquivoRepo(db *sql.DB) *PadraoArquivoRepo { return &PadraoArquivoRepo{db: db} }

func (r *PadraoArquivoRepo) Listar(ctx context.Context) ([]model.PadraoArquivo, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT id, padrao, produto, IFNULL(dominio,''), IFNULL(delimitador,''), IFNULL(separador_decimal,''), IFNULL(separador_milhar,''), IFNULL(formato_data,''), IFNULL(coluna_data,''), IFNULL(coluna_boleto,''), IFNULL(obrigatorio_mov_d1,1) FROM padrao_arquivo ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []model.PadraoArquivo{}
	for rows.Next() {
		var p model.PadraoArquivo
		var obrigD1 int
		if err := rows.Scan(&p.ID, &p.Padrao, &p.Produto, &p.Dominio, &p.Delimitador, &p.SeparadorDecimal, &p.SeparadorMilhar, &p.FormatoData, &p.ColunaData, &p.ColunaBoleto, &obrigD1); err != nil {
			return nil, err
		}
		b := obrigD1 != 0
		p.ObrigatorioMovD1 = &b
		result = append(result, p)
	}
	return result, rows.Err()
}

func (r *PadraoArquivoRepo) Criar(ctx context.Context, p model.PadraoArquivo) (int64, error) {
	obrigD1 := 1
	if p.ObrigatorioMovD1 != nil && !*p.ObrigatorioMovD1 {
		obrigD1 = 0
	}
	res, err := r.db.ExecContext(ctx,
		"INSERT INTO padrao_arquivo (padrao, produto, dominio, delimitador, separador_decimal, separador_milhar, formato_data, coluna_data, coluna_boleto, obrigatorio_mov_d1) VALUES (?,?,?,?,?,?,?,?,?,?)",
		p.Padrao, p.Produto, p.Dominio, p.Delimitador, p.SeparadorDecimal, p.SeparadorMilhar, p.FormatoData, p.ColunaData, p.ColunaBoleto, obrigD1,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (r *PadraoArquivoRepo) Excluir(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM padrao_arquivo WHERE id = ?", id)
	return err
}
