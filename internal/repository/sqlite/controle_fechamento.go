package sqlite

import (
	"context"
	"database/sql"
	"time"

	"srcoff/internal/model"
)

// ControleFechamentoRepo persiste o controle de fechamento de competência (RF-132): o modo, os
// escopos selecionados e as competências liberadas (mês/ano).
type ControleFechamentoRepo struct{ db *sql.DB }

func NewControleFechamentoRepo(db *sql.DB) *ControleFechamentoRepo {
	return &ControleFechamentoRepo{db: db}
}

// Carregar lê o estado completo do controle (modo + escopos + liberações).
func (r *ControleFechamentoRepo) Carregar(ctx context.Context) (model.FechamentoConfig, error) {
	cfg := model.FechamentoConfig{Modo: model.FechamentoOff, Escopos: []model.FechamentoEscopo{}, Liberacoes: []model.FechamentoLiberacao{}}

	var modo string
	switch err := r.db.QueryRowContext(ctx, `SELECT modo FROM fechamento_config WHERE id=1`).Scan(&modo); err {
	case nil:
		cfg.Modo = modo
	case sql.ErrNoRows:
		// sem configuração → off
	default:
		return cfg, err
	}

	rowsE, err := r.db.QueryContext(ctx, `SELECT id, produto, dominio FROM fechamento_escopo ORDER BY produto, dominio`)
	if err != nil {
		return cfg, err
	}
	defer rowsE.Close()
	for rowsE.Next() {
		var e model.FechamentoEscopo
		if err := rowsE.Scan(&e.ID, &e.Produto, &e.Dominio); err != nil {
			return cfg, err
		}
		cfg.Escopos = append(cfg.Escopos, e)
	}
	if err := rowsE.Err(); err != nil {
		return cfg, err
	}

	rowsL, err := r.db.QueryContext(ctx,
		`SELECT id, ano, mes, produto, dominio, IFNULL(criado_em,'') FROM fechamento_liberacao ORDER BY ano DESC, mes DESC, produto, dominio`)
	if err != nil {
		return cfg, err
	}
	defer rowsL.Close()
	for rowsL.Next() {
		var l model.FechamentoLiberacao
		if err := rowsL.Scan(&l.ID, &l.Ano, &l.Mes, &l.Produto, &l.Dominio, &l.CriadoEm); err != nil {
			return cfg, err
		}
		cfg.Liberacoes = append(cfg.Liberacoes, l)
	}
	return cfg, rowsL.Err()
}

// DefinirModo grava o modo do controle (off | todos | selecionados).
func (r *ControleFechamentoRepo) DefinirModo(ctx context.Context, modo string) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO fechamento_config (id, modo) VALUES (1, ?) ON CONFLICT(id) DO UPDATE SET modo=excluded.modo`, modo)
	return err
}

// AdicionarEscopo inclui um escopo (produto/domínio) na lista controlada (ignora duplicata).
func (r *ControleFechamentoRepo) AdicionarEscopo(ctx context.Context, produto, dominio string) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO fechamento_escopo (produto, dominio) VALUES (?, ?) ON CONFLICT(produto, dominio) DO NOTHING`, produto, dominio)
	return err
}

func (r *ControleFechamentoRepo) RemoverEscopo(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM fechamento_escopo WHERE id=?`, id)
	return err
}

// Liberar registra o OK de uma competência (mês/ano) para um escopo (ignora duplicata).
func (r *ControleFechamentoRepo) Liberar(ctx context.Context, ano, mes int, produto, dominio string) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO fechamento_liberacao (ano, mes, produto, dominio, criado_em) VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(ano, mes, produto, dominio) DO NOTHING`,
		ano, mes, produto, dominio, fmtTS(time.Now()))
	return err
}

func (r *ControleFechamentoRepo) RemoverLiberacao(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM fechamento_liberacao WHERE id=?`, id)
	return err
}
