package handler

import (
	"context"
	"encoding/json"
	"net/http"
)

type parametrizacaoSvc interface {
	ListarOpcoes(ctx context.Context, categoria string) ([]string, error)
	AdicionarOpcao(ctx context.Context, categoria, valor string) error
	RemoverOpcao(ctx context.Context, categoria, valor string) error
}

// ParametrizacaoHandler expõe o CRUD das opções parametrizáveis (combos de Produto/Domínio).
type ParametrizacaoHandler struct {
	svc parametrizacaoSvc
}

func NewParametrizacaoHandler(svc parametrizacaoSvc) *ParametrizacaoHandler {
	return &ParametrizacaoHandler{svc: svc}
}

// Opcoes trata /api/v1/parametrizacoes/opcoes.
//   GET    ?categoria=produto            → lista as opções
//   POST   {categoria, valor}            → adiciona opção
//   DELETE ?categoria=produto&valor=NDF  → remove opção
func (h *ParametrizacaoHandler) Opcoes(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		categoria := r.URL.Query().Get("categoria")
		opcoes, err := h.svc.ListarOpcoes(r.Context(), categoria)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"erro": err.Error()})
			return
		}
		if opcoes == nil {
			opcoes = []string{}
		}
		writeJSON(w, http.StatusOK, opcoes)

	case http.MethodPost:
		var req struct {
			Categoria string `json:"categoria"`
			Valor     string `json:"valor"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"erro": err.Error()})
			return
		}
		if err := h.svc.AdicionarOpcao(r.Context(), req.Categoria, req.Valor); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"erro": err.Error()})
			return
		}
		writeJSON(w, http.StatusCreated, map[string]string{"mensagem": "opção adicionada"})

	case http.MethodDelete:
		categoria := r.URL.Query().Get("categoria")
		valor := r.URL.Query().Get("valor")
		if err := h.svc.RemoverOpcao(r.Context(), categoria, valor); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"erro": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"mensagem": "opção removida"})

	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}
