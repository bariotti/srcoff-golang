package handler

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"srcoff/internal/model"
)

type auditoriaSvc interface {
	Disponivel() bool
	Listar(ctx context.Context, f model.FiltroAuditoria) (*model.PaginaAuditoria, error)
}

// AuditoriaHandler expõe a trilha de auditoria global (RF-136): consulta paginada/filtrável e
// exportação (NDJSON estruturado ou CSV). Recurso sqlite-first: 501 quando indisponível.
type AuditoriaHandler struct {
	svc auditoriaSvc
}

func NewAuditoriaHandler(svc auditoriaSvc) *AuditoriaHandler {
	return &AuditoriaHandler{svc: svc}
}

func (h *AuditoriaHandler) indisponivel(w http.ResponseWriter) bool {
	if h.svc == nil || !h.svc.Disponivel() {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"erro": "trilha de auditoria indisponível neste backend (disponível apenas no SQLite)"})
		return true
	}
	return false
}

func filtroDaQuery(r *http.Request) model.FiltroAuditoria {
	q := r.URL.Query()
	pagina, _ := strconv.Atoi(q.Get("pagina"))
	tamanho, _ := strconv.Atoi(q.Get("tamanho"))
	return model.FiltroAuditoria{
		Categoria: q.Get("categoria"),
		Origem:    q.Get("origem"),
		De:        q.Get("de"),
		Ate:       q.Get("ate"),
		Texto:     q.Get("q"),
		Pagina:    pagina,
		Tamanho:   tamanho,
	}
}

// Listar trata GET /api/v1/auditoria.
func (h *AuditoriaHandler) Listar(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if h.indisponivel(w) {
		return
	}
	pag, err := h.svc.Listar(r.Context(), filtroDaQuery(r))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"erro": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, pag)
}

// Export trata GET /api/v1/auditoria/export — NDJSON (default) ou CSV (formato=csv). Aplica os
// mesmos filtros da consulta, sem paginação (limitado a uma página grande).
func (h *AuditoriaHandler) Export(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if h.indisponivel(w) {
		return
	}
	f := filtroDaQuery(r)
	f.Pagina = 1
	f.Tamanho = 1000 // teto por página do repo; export pega a página mais recente
	if n, _ := strconv.Atoi(r.URL.Query().Get("tamanho")); n > 0 && n <= 1000 {
		f.Tamanho = n
	}
	pag, err := h.svc.Listar(r.Context(), f)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"erro": err.Error()})
		return
	}

	if r.URL.Query().Get("formato") == "csv" {
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", "attachment; filename=\"auditoria.csv\"")
		cw := csv.NewWriter(w)
		cw.Comma = ';'
		_ = cw.Write([]string{"criado_em", "categoria", "acao", "entidade", "origem", "detalhe", "dados"})
		for _, ev := range pag.Eventos {
			dados := ""
			if len(ev.Dados) > 0 {
				if b, err := json.Marshal(ev.Dados); err == nil {
					dados = string(b)
				}
			}
			_ = cw.Write([]string{ev.CriadoEm, ev.Categoria, ev.Acao, ev.Entidade, ev.Origem, ev.Detalhe, dados})
		}
		cw.Flush()
		return
	}

	// NDJSON estruturado: um objeto JSON por linha.
	w.Header().Set("Content-Type", "application/x-ndjson; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=\"auditoria.ndjson\"")
	enc := json.NewEncoder(w)
	for _, ev := range pag.Eventos {
		if err := enc.Encode(ev); err != nil {
			fmt.Fprintf(w, "{\"erro\":%q}\n", err.Error())
			return
		}
	}
}
