package handler

import (
	"context"
	"net/http"
	"strconv"

	"srcoff/internal/model"
)

type notificacaoSvc interface {
	Listar(ctx context.Context, limite int) ([]model.Notificacao, error)
	ContarNaoLidas(ctx context.Context) (int, error)
	MarcarTodasLidas(ctx context.Context) error
}

// NotificacaoHandler expõe a leitura das notificações e a marcação como lidas.
type NotificacaoHandler struct {
	svc notificacaoSvc
}

func NewNotificacaoHandler(svc notificacaoSvc) *NotificacaoHandler {
	return &NotificacaoHandler{svc: svc}
}

// Notificacoes trata /api/v1/notificacoes.
//   GET  → { nao_lidas, itens: [...] }
//   POST → marca todas como lidas (ação de leitura ao abrir o sino)
// Spec: RF-071 (docs/especificacao.md §10, §11.5).
func (h *NotificacaoHandler) Notificacoes(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		limite := 30
		if v, err := strconv.Atoi(r.URL.Query().Get("limite")); err == nil && v > 0 {
			limite = v
		}
		itens, err := h.svc.Listar(r.Context(), limite)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"erro": err.Error()})
			return
		}
		naoLidas, _ := h.svc.ContarNaoLidas(r.Context())
		if itens == nil {
			itens = []model.Notificacao{}
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"nao_lidas": naoLidas, "itens": itens})

	case http.MethodPost:
		if err := h.svc.MarcarTodasLidas(r.Context()); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"erro": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"mensagem": "notificações marcadas como lidas"})

	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}
