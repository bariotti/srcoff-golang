package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"srcoff/internal/model"
)

type fechamentoSvc interface {
	Estado(ctx context.Context) (model.FechamentoConfig, error)
	DefinirModo(ctx context.Context, modo string) error
	AdicionarEscopo(ctx context.Context, produto, dominio string) error
	RemoverEscopo(ctx context.Context, id int64) error
	Liberar(ctx context.Context, ano, mes int, produto, dominio string) error
	RemoverLiberacao(ctx context.Context, id int64) error
}

// FechamentoHandler expõe o controle de fechamento de competência (RF-132). svc pode ser nil
// quando o backend atual não suporta a feature (apenas SQLite) — nesse caso responde 501.
type FechamentoHandler struct{ svc fechamentoSvc }

func NewFechamentoHandler(svc fechamentoSvc) *FechamentoHandler { return &FechamentoHandler{svc: svc} }

func (h *FechamentoHandler) indisponivel(w http.ResponseWriter) bool {
	if h.svc == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"erro": "controle de fechamento disponível apenas no backend SQLite"})
		return true
	}
	return false
}

// Raiz trata GET /api/v1/fechamento — devolve o estado (modo, escopos, liberações).
func (h *FechamentoHandler) Raiz(w http.ResponseWriter, r *http.Request) {
	if h.indisponivel(w) {
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	cfg, err := h.svc.Estado(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"erro": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, cfg)
}

// Modo trata POST /api/v1/fechamento/modo — define o modo do controle.
func (h *FechamentoHandler) Modo(w http.ResponseWriter, r *http.Request) {
	if h.indisponivel(w) {
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Modo string `json:"modo"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"erro": "corpo inválido"})
		return
	}
	if err := h.svc.DefinirModo(r.Context(), body.Modo); err != nil {
		writeJSON(w, http.StatusOK, map[string]string{"erro": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"mensagem": "modo salvo"})
}

// Escopo trata /api/v1/fechamento/escopo — POST adiciona; DELETE /{id} remove.
func (h *FechamentoHandler) Escopo(w http.ResponseWriter, r *http.Request) {
	if h.indisponivel(w) {
		return
	}
	switch r.Method {
	case http.MethodPost:
		var body struct {
			Produto string `json:"produto"`
			Dominio string `json:"dominio"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"erro": "corpo inválido"})
			return
		}
		if err := h.svc.AdicionarEscopo(r.Context(), body.Produto, body.Dominio); err != nil {
			writeJSON(w, http.StatusOK, map[string]string{"erro": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"mensagem": "escopo adicionado"})
	case http.MethodDelete:
		id, ok := idDoPath(w, r.URL.Path, "/api/v1/fechamento/escopo/")
		if !ok {
			return
		}
		if err := h.svc.RemoverEscopo(r.Context(), id); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"erro": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"mensagem": "escopo removido"})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// Liberar trata /api/v1/fechamento/liberar — POST libera uma competência; DELETE /{id} remove.
func (h *FechamentoHandler) Liberar(w http.ResponseWriter, r *http.Request) {
	if h.indisponivel(w) {
		return
	}
	switch r.Method {
	case http.MethodPost:
		var body struct {
			Ano     int    `json:"ano"`
			Mes     int    `json:"mes"`
			Produto string `json:"produto"`
			Dominio string `json:"dominio"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"erro": "corpo inválido"})
			return
		}
		if err := h.svc.Liberar(r.Context(), body.Ano, body.Mes, body.Produto, body.Dominio); err != nil {
			writeJSON(w, http.StatusOK, map[string]string{"erro": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"mensagem": "competência liberada"})
	case http.MethodDelete:
		id, ok := idDoPath(w, r.URL.Path, "/api/v1/fechamento/liberar/")
		if !ok {
			return
		}
		if err := h.svc.RemoverLiberacao(r.Context(), id); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"erro": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"mensagem": "liberação removida"})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// idDoPath extrai o id numérico ao final do path após o prefixo informado.
func idDoPath(w http.ResponseWriter, path, prefixo string) (int64, bool) {
	id, err := strconv.ParseInt(strings.TrimPrefix(path, prefixo), 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"erro": "id inválido"})
		return 0, false
	}
	return id, true
}
