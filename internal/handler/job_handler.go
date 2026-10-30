package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"srcoff/internal/model"
)

type jobSvc interface {
	Enfileirar(ctx context.Context, tipo, descricao, payload string, maxTentativas int) (int64, error)
	Listar(ctx context.Context, status string, pagina, tamanho int) (*model.JobPagina, error)
	Obter(ctx context.Context, id int64) (*model.Job, error)
	Retry(ctx context.Context, id int64) error
}

// JobHandler expõe a fila de jobs (RF-121). svc pode ser nil quando o backend atual
// não suporta a fila (apenas SQLite por enquanto) — nesse caso responde 501.
type JobHandler struct {
	svc jobSvc
}

func NewJobHandler(svc jobSvc) *JobHandler { return &JobHandler{svc: svc} }

func (h *JobHandler) indisponivel(w http.ResponseWriter) bool {
	if h.svc == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"erro": "fila de jobs disponível apenas no backend SQLite"})
		return true
	}
	return false
}

// Raiz trata /api/v1/jobs — GET lista, POST enfileira.
func (h *JobHandler) Raiz(w http.ResponseWriter, r *http.Request) {
	if h.indisponivel(w) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		q := r.URL.Query()
		pagina, _ := strconv.Atoi(q.Get("pagina"))
		tamanho, _ := strconv.Atoi(q.Get("tamanho"))
		pag, err := h.svc.Listar(r.Context(), strings.TrimSpace(q.Get("status")), pagina, tamanho)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"erro": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, pag)
	case http.MethodPost:
		var body struct {
			Tipo    string `json:"tipo"`
			Data    string `json:"data"`
			Produto string `json:"produto"`
			Dominio string `json:"dominio"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"erro": "corpo inválido"})
			return
		}
		tipo := strings.TrimSpace(body.Tipo)
		if tipo == "" {
			tipo = "geracao_movimento"
		}
		if tipo == "geracao_movimento" && strings.TrimSpace(body.Data) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"erro": "informe a data (AAAA-MM-DD)"})
			return
		}
		payload, _ := json.Marshal(map[string]string{"data": body.Data, "produto": body.Produto, "dominio": body.Dominio})
		desc := descricaoJob(tipo, body.Data, body.Produto, body.Dominio)
		id, err := h.svc.Enfileirar(r.Context(), tipo, desc, string(payload), 3)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"erro": err.Error()})
			return
		}
		writeJSON(w, http.StatusCreated, map[string]interface{}{"id": id})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// Item trata /api/v1/jobs/{id} (GET) e /api/v1/jobs/{id}/retry (POST).
func (h *JobHandler) Item(w http.ResponseWriter, r *http.Request) {
	if h.indisponivel(w) {
		return
	}
	resto := strings.TrimPrefix(r.URL.Path, "/api/v1/jobs/")
	resto = strings.Trim(resto, "/")
	partes := strings.Split(resto, "/")
	id, err := strconv.ParseInt(partes[0], 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"erro": "id inválido"})
		return
	}
	if len(partes) >= 2 && partes[1] == "retry" {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if err := h.svc.Retry(r.Context(), id); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"erro": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "reagendado"})
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	j, err := h.svc.Obter(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"erro": err.Error()})
		return
	}
	if j == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"erro": "job não encontrado"})
		return
	}
	writeJSON(w, http.StatusOK, j)
}

func descricaoJob(tipo, data, produto, dominio string) string {
	if tipo == "expurgo_movimento" {
		return "Expurgo de movimentos antigos (retenção)"
	}
	if tipo == "geracao_movimento" {
		d := "Geração de movimento " + data
		escopo := strings.TrimSpace(produto + " " + dominio)
		if escopo != "" {
			d += " (" + strings.TrimSpace(produto+"/"+dominio) + ")"
		}
		return d
	}
	return tipo
}
