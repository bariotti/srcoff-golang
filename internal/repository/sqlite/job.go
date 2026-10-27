package sqlite

import (
	"context"
	"database/sql"
	"time"

	"srcoff/internal/model"
)

// JobRepo implementa repository.JobRepository em SQLite.
type JobRepo struct{ db *sql.DB }

func NewJobRepo(db *sql.DB) *JobRepo { return &JobRepo{db: db} }

const colsJob = "id, tipo, descricao, payload, status, tentativas, max_tentativas, log, IFNULL(criado_em,''), IFNULL(atualizado_em,'')"

func scanJob(sc interface{ Scan(...interface{}) error }) (model.Job, error) {
	var j model.Job
	var criado, atualizado string
	err := sc.Scan(&j.ID, &j.Tipo, &j.Descricao, &j.Payload, &j.Status, &j.Tentativas, &j.MaxTentativas, &j.Log, &criado, &atualizado)
	if err != nil {
		return j, err
	}
	j.CriadoEm = parseTS(criado)
	j.AtualizadoEm = parseTS(atualizado)
	return j, nil
}

func (r *JobRepo) Criar(ctx context.Context, j model.Job) (int64, error) {
	if j.Status == "" {
		j.Status = model.JobPendente
	}
	if j.MaxTentativas <= 0 {
		j.MaxTentativas = 3
	}
	now := fmtTS(time.Now())
	res, err := r.db.ExecContext(ctx,
		"INSERT INTO job (tipo, descricao, payload, status, tentativas, max_tentativas, log, criado_em, atualizado_em) VALUES (?,?,?,?,?,?,?,?,?)",
		j.Tipo, j.Descricao, j.Payload, j.Status, j.Tentativas, j.MaxTentativas, j.Log, now, now,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (r *JobRepo) Atualizar(ctx context.Context, j model.Job) error {
	_, err := r.db.ExecContext(ctx,
		"UPDATE job SET status=?, tentativas=?, log=?, atualizado_em=? WHERE id=?",
		j.Status, j.Tentativas, j.Log, fmtTS(time.Now()), j.ID,
	)
	return err
}

func (r *JobRepo) ObterPorID(ctx context.Context, id int64) (*model.Job, error) {
	row := r.db.QueryRowContext(ctx, "SELECT "+colsJob+" FROM job WHERE id=?", id)
	j, err := scanJob(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &j, nil
}

func (r *JobRepo) ProximoPendente(ctx context.Context) (*model.Job, error) {
	row := r.db.QueryRowContext(ctx, "SELECT "+colsJob+" FROM job WHERE status=? ORDER BY id LIMIT 1", model.JobPendente)
	j, err := scanJob(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &j, nil
}

func (r *JobRepo) ListarPaginado(ctx context.Context, status string, pagina, tamanho int) (*model.JobPagina, error) {
	if pagina <= 0 {
		pagina = 1
	}
	if tamanho <= 0 {
		tamanho = 20
	}
	where := ""
	args := []interface{}{}
	if status != "" {
		where = " WHERE status=?"
		args = append(args, status)
	}
	var total int
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM job"+where, args...).Scan(&total); err != nil {
		return nil, err
	}
	offset := (pagina - 1) * tamanho
	q := "SELECT " + colsJob + " FROM job" + where + " ORDER BY id DESC LIMIT ? OFFSET ?"
	rows, err := r.db.QueryContext(ctx, q, append(args, tamanho, offset)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	jobs := []model.Job{}
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, j)
	}
	return &model.JobPagina{Total: total, Pagina: pagina, Tamanho: tamanho, Jobs: jobs}, rows.Err()
}
