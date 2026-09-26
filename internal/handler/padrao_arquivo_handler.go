package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"

	"srcoff/internal/model"
)

type padraoArquivoSvc interface {
	Listar(ctx context.Context) ([]model.PadraoArquivo, error)
	Criar(ctx context.Context, p model.PadraoArquivo) (int64, error)
	Excluir(ctx context.Context, id int64) error
}

// PadraoArquivoHandler expõe o CRUD dos padrões nome-de-arquivo → produto.
type PadraoArquivoHandler struct {
	svc padraoArquivoSvc
}

func NewPadraoArquivoHandler(svc padraoArquivoSvc) *PadraoArquivoHandler {
	return &PadraoArquivoHandler{svc: svc}
}

// Padroes trata /api/v1/parametrizacoes/padroes.
//   GET    → lista os padrões
//   POST   {padrao, produto} → cria
//   DELETE ?id=N → remove
func (h *PadraoArquivoHandler) Padroes(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		itens, err := h.svc.Listar(r.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"erro": err.Error()})
			return
		}
		if itens == nil {
			itens = []model.PadraoArquivo{}
		}
		writeJSON(w, http.StatusOK, itens)

	case http.MethodPost:
		var p model.PadraoArquivo
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"erro": err.Error()})
			return
		}
		id, err := h.svc.Criar(r.Context(), p)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"erro": err.Error()})
			return
		}
		writeJSON(w, http.StatusCreated, map[string]int64{"id": id})

	case http.MethodDelete:
		id, err := strconv.ParseInt(r.URL.Query().Get("id"), 10, 64)
		if err != nil || id == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"erro": "id inválido"})
			return
		}
		if err := h.svc.Excluir(r.Context(), id); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"erro": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"mensagem": "padrão removido"})

	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}
