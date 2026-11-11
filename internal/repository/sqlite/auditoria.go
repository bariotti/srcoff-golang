package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"

	"srcoff/internal/model"
)

// AuditoriaRepo persiste e consulta a trilha de auditoria global (RF-136) em SQLite.
// Recurso sqlite-first: nos backends file/sqlserver não há persistência (só a linha de log JSON).
type AuditoriaRepo struct{ db *sql.DB }

func NewAuditoriaRepo(db *sql.DB) *AuditoriaRepo { return &AuditoriaRepo{db: db} }

// Inserir grava um evento. `Dados` é serializado em JSON.
func (r *AuditoriaRepo) Inserir(ctx context.Context, ev model.EventoAuditoria) error {
	dados := ""
	if len(ev.Dados) > 0 {
		if b, err := json.Marshal(ev.Dados); err == nil {
			dados = string(b)
		}
	}
	_, err := r.db.ExecContext(ctx,
		"INSERT INTO auditoria (criado_em, categoria, acao, entidade, origem, detalhe, dados) VALUES (?,?,?,?,?,?,?)",
		ev.CriadoEm, ev.Categoria, ev.Acao, ev.Entidade, ev.Origem, ev.Detalhe, dados,
	)
	return err
}

// Listar devolve uma página de eventos (mais recente primeiro) aplicando o filtro.
func (r *AuditoriaRepo) Listar(ctx context.Context, f model.FiltroAuditoria) (*model.PaginaAuditoria, error) {
	where, args := auditoriaWhere(f)

	var total int
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM auditoria "+where, args...).Scan(&total); err != nil {
		return nil, err
	}

	pagina := f.Pagina
	if pagina < 1 {
		pagina = 1
	}
	tam := f.Tamanho
	if tam <= 0 {
		tam = 50
	}
	if tam > 1000 {
		tam = 1000
	}
	offset := (pagina - 1) * tam

	rows, err := r.db.QueryContext(ctx,
		"SELECT id, criado_em, categoria, acao, entidade, origem, detalhe, IFNULL(dados,'') FROM auditoria "+where+" ORDER BY id DESC LIMIT ? OFFSET ?",
		append(append([]interface{}{}, args...), tam, offset)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := &model.PaginaAuditoria{Total: total, Pagina: pagina, Tamanho: tam, Eventos: []model.EventoAuditoria{}}
	for rows.Next() {
		var ev model.EventoAuditoria
		var dados string
		if err := rows.Scan(&ev.ID, &ev.CriadoEm, &ev.Categoria, &ev.Acao, &ev.Entidade, &ev.Origem, &ev.Detalhe, &dados); err != nil {
			return nil, err
		}
		if dados != "" {
			_ = json.Unmarshal([]byte(dados), &ev.Dados)
		}
		out.Eventos = append(out.Eventos, ev)
	}
	return out, rows.Err()
}

// auditoriaWhere monta a cláusula WHERE + argumentos a partir do filtro.
func auditoriaWhere(f model.FiltroAuditoria) (string, []interface{}) {
	var conds []string
	var args []interface{}
	if c := strings.TrimSpace(f.Categoria); c != "" {
		conds = append(conds, "categoria = ?")
		args = append(args, c)
	}
	if o := strings.TrimSpace(f.Origem); o != "" {
		conds = append(conds, "origem = ?")
		args = append(args, o)
	}
	if de := strings.TrimSpace(f.De); de != "" {
		conds = append(conds, "criado_em >= ?")
		args = append(args, de+" 00:00:00")
	}
	if ate := strings.TrimSpace(f.Ate); ate != "" {
		conds = append(conds, "criado_em <= ?")
		args = append(args, ate+" 23:59:59")
	}
	if t := strings.TrimSpace(f.Texto); t != "" {
		conds = append(conds, "(LOWER(acao) LIKE ? OR LOWER(entidade) LIKE ? OR LOWER(detalhe) LIKE ?)")
		like := "%" + strings.ToLower(t) + "%"
		args = append(args, like, like, like)
	}
	if len(conds) == 0 {
		return "", args
	}
	return "WHERE " + strings.Join(conds, " AND "), args
}
