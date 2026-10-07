package sqlite

import (
	"context"
	"database/sql"
	"time"

	"srcoff/internal/model"
)

// NotificacaoRepo implementa repository.NotificacaoRepository em SQLite.
type NotificacaoRepo struct{ db *sql.DB }

func NewNotificacaoRepo(db *sql.DB) *NotificacaoRepo { return &NotificacaoRepo{db: db} }

func (r *NotificacaoRepo) Criar(ctx context.Context, n model.Notificacao) (int64, error) {
	res, err := r.db.ExecContext(ctx,
		"INSERT INTO notificacao (tipo, data_lote, produto, dominio, mensagem, lida, criado_em) VALUES (?,?,?,?,?,?,?)",
		n.Tipo, n.DataLote, n.Produto, n.Dominio, n.Mensagem, b2i(n.Lida), fmtTS(time.Now()),
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (r *NotificacaoRepo) Listar(ctx context.Context, limite int) ([]model.Notificacao, error) {
	query := "SELECT id, IFNULL(tipo,''), IFNULL(data_lote,''), IFNULL(produto,''), IFNULL(dominio,''), IFNULL(mensagem,''), lida, IFNULL(criado_em,'') FROM notificacao ORDER BY id DESC"
	var rows *sql.Rows
	var err error
	if limite > 0 {
		rows, err = r.db.QueryContext(ctx, query+" LIMIT ?", limite)
	} else {
		rows, err = r.db.QueryContext(ctx, query)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []model.Notificacao{}
	for rows.Next() {
		var n model.Notificacao
		var criadoStr string
		if err := rows.Scan(&n.ID, &n.Tipo, &n.DataLote, &n.Produto, &n.Dominio, &n.Mensagem, &n.Lida, &criadoStr); err != nil {
			return nil, err
		}
		n.CriadoEm = parseTS(criadoStr)
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
