package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"srcoff/internal/model"
)

type regraContabilSvc interface {
	ListarRegras(ctx context.Context) ([]model.RegraContabil, error)
	CriarRegra(ctx context.Context, regra model.RegraContabil) (int64, error)
	EditarRegra(ctx context.Context, regra model.RegraContabil) error
	ListarCondicoes(ctx context.Context, idRegra int64) ([]model.CondicaoRegra, error)
	CriarCondicao(ctx context.Context, condicao model.CondicaoRegra) (int64, error)
	EditarCondicao(ctx context.Context, condicao model.CondicaoRegra) error
	ExcluirCondicao(ctx context.Context, id int64) error
}

// RegraContabilHandler expõe os endpoints de regras contábeis.
type RegraContabilHandler struct {
	svc regraContabilSvc
}

// NewRegraContabilHandler cria uma nova instância do handler com a dependência injetada.
func NewRegraContabilHandler(svc regraContabilSvc) *RegraContabilHandler {
	return &RegraContabilHandler{svc: svc}
}

// ListarRegras trata GET /api/v1/regras.
func (h *RegraContabilHandler) ListarRegras(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	regras, err := h.svc.ListarRegras(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"erro": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, regras)
}

// CriarRegra trata POST /api/v1/regras.
func (h *RegraContabilHandler) CriarRegra(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var regra model.RegraContabil
	if err := json.NewDecoder(r.Body).Decode(&regra); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"erro": err.Error()})
		return
	}

	newID, err := h.svc.CriarRegra(r.Context(), regra)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"erro": err.Error()})
		return
	}

	writeJSON(w, http.StatusCreated, map[string]int64{"id": newID})
}

// EditarRegra trata PUT /api/v1/regras/{id}.
func (h *RegraContabilHandler) EditarRegra(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	idStr := strings.TrimPrefix(r.URL.Path, "/api/v1/regras/")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"erro": "id inválido"})
		return
	}

	var regra model.RegraContabil
	if err := json.NewDecoder(r.Body).Decode(&regra); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"erro": err.Error()})
		return
	}
	regra.ID = id

	if err := h.svc.EditarRegra(r.Context(), regra); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"erro": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"mensagem": "regra atualizada com sucesso"})
}

// ListarCondicoes trata GET /api/v1/regras/{id}/condicoes.
func (h *RegraContabilHandler) ListarCondicoes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// path: /api/v1/regras/42/condicoes
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v1/regras/"), "/")
	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || id == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"erro": "id inválido"})
		return
	}

	condicoes, err := h.svc.ListarCondicoes(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"erro": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, condicoes)
}

// CriarCondicao trata POST /api/v1/regras/{id}/condicoes.
func (h *RegraContabilHandler) CriarCondicao(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// path: /api/v1/regras/42/condicoes
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v1/regras/"), "/")
	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || id == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"erro": "id inválido"})
		return
	}

	var condicao model.CondicaoRegra
	if err := json.NewDecoder(r.Body).Decode(&condicao); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"erro": err.Error()})
		return
	}
	condicao.IDRegra = id

	newID, err := h.svc.CriarCondicao(r.Context(), condicao)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"erro": err.Error()})
		return
	}

	writeJSON(w, http.StatusCreated, map[string]int64{"id": newID})
}

// EditarCondicao trata PUT /api/v1/condicoes/{id}.
func (h *RegraContabilHandler) EditarCondicao(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	idStr := strings.TrimPrefix(r.URL.Path, "/api/v1/condicoes/")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"erro": "id inválido"})
		return
	}

	var condicao model.CondicaoRegra
	if err := json.NewDecoder(r.Body).Decode(&condicao); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"erro": err.Error()})
		return
	}
	condicao.ID = id

	if err := h.svc.EditarCondicao(r.Context(), condicao); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"erro": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"mensagem": "condição atualizada com sucesso"})
}

// ExcluirCondicao trata DELETE /api/v1/condicoes/{id}.
func (h *RegraContabilHandler) ExcluirCondicao(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	idStr := strings.TrimPrefix(r.URL.Path, "/api/v1/condicoes/")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"erro": "id inválido"})
		return
	}
	if err := h.svc.ExcluirCondicao(r.Context(), id); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"erro": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"mensagem": "condição excluída com sucesso"})
}
