package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"srcoff/internal/model"
)

// NotificacaoRepo persiste as notificações no SQL Server.
type NotificacaoRepo struct {
	db *sql.DB
}

func NewNotificacaoRepo(db *sql.DB) *NotificacaoRepo {
	return &NotificacaoRepo{db: db}
}

func (r *NotificacaoRepo) Criar(ctx context.Context, n model.Notificacao) (int64, error) {
	var id int64
	lida := 0
	if n.Lida {
		lida = 1
	}
	var dataLote interface{}
	if n.DataLote != "" {
		dataLote = n.DataLote
	}
	err := r.db.QueryRowContext(ctx,
		"INSERT INTO notificacao (tipo, data_lote, produto, dominio, mensagem, lida, criado_em) VALUES (@p1,@p2,@p3,@p4,@p5,@p6,@p7); SELECT SCOPE_IDENTITY()",
		n.Tipo, dataLote, n.Produto, n.Dominio, n.Mensagem, lida, time.Now(),
	).Scan(&id)
	return id, err
}

func (r *NotificacaoRepo) Listar(ctx context.Context, limite int) ([]model.Notificacao, error) {
	top := ""
	if limite > 0 {
		top = fmt.Sprintf("TOP %d ", limite)
	}
	rows, err := r.db.QueryContext(ctx,
		"SELECT "+top+"id, tipo, ISNULL(CONVERT(VARCHAR(10), data_lote, 120),''), ISNULL(produto,''), ISNULL(dominio,''), mensagem, lida, criado_em FROM notificacao ORDER BY id DESC",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []model.Notificacao{}
	for rows.Next() {
		var n model.Notificacao
		if err := rows.Scan(&n.ID, &n.Tipo, &n.DataLote, &n.Produto, &n.Dominio, &n.Mensagem, &n.Lida, &n.CriadoEm); err != nil {
			return nil, err
		}
		result = append(result, n)
	}
	return result, rows.Err()
}

func (r *NotificacaoRepo) ContarNaoLidas(ctx context.Context) (int, error) {
	var n int
	err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM notificacao WHERE lida = 0").Scan(&n)
	return n, err
}

func (r *NotificacaoRepo) MarcarTodasLidas(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, "UPDATE notificacao SET lida = 1 WHERE lida = 0")
	return err
}
