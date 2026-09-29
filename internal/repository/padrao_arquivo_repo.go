package repository

import (
	"context"
	"database/sql"

	"srcoff/internal/model"
)

// PadraoArquivoRepo persiste os padrões nome-de-arquivo→produto no SQL Server.
// Ver migration 006_padrao_arquivo.sql.
type PadraoArquivoRepo struct {
	db *sql.DB
}

func NewPadraoArquivoRepo(db *sql.DB) *PadraoArquivoRepo {
	return &PadraoArquivoRepo{db: db}
}

func (r *PadraoArquivoRepo) Listar(ctx context.Context) ([]model.PadraoArquivo, error) {
	rows, err := r.db.QueryContext(ctx, "SELECT id, padrao, produto, ISNULL(dominio, ''), ISNULL(delimitador, ''), ISNULL(separador_decimal, ''), ISNULL(separador_milhar, ''), ISNULL(formato_data, '') FROM padrao_arquivo ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []model.PadraoArquivo{}
	for rows.Next() {
		var p model.PadraoArquivo
		if err := rows.Scan(&p.ID, &p.Padrao, &p.Produto, &p.Dominio, &p.Delimitador, &p.SeparadorDecimal, &p.SeparadorMilhar, &p.FormatoData); err != nil {
			return nil, err
		}
		result = append(result, p)
	}
	return result, rows.Err()
}

func (r *PadraoArquivoRepo) Criar(ctx context.Context, p model.PadraoArquivo) (int64, error) {
	var id int64
	err := r.db.QueryRowContext(ctx,
		"INSERT INTO padrao_arquivo (padrao, produto, dominio, delimitador, separador_decimal, separador_milhar, formato_data) VALUES (@p1, @p2, @p3, @p4, @p5, @p6, @p7); SELECT SCOPE_IDENTITY()",
		p.Padrao, p.Produto, p.Dominio, p.Delimitador, p.SeparadorDecimal, p.SeparadorMilhar, p.FormatoData,
	).Scan(&id)
	return id, err
}

func (r *PadraoArquivoRepo) Excluir(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM padrao_arquivo WHERE id = @p1", id)
	return err
}
