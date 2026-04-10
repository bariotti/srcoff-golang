package handler

import (
	"context"
	"net/http"
	"time"

	"srcoff/internal/model"
)

type conciliacaoSvc interface {
	Conciliar(ctx context.Context, data time.Time) (*model.ResultadoConciliacao, error)
}

type ConciliacaoHandler struct {
	svc conciliacaoSvc
}

func NewConciliacaoHandler(svc conciliacaoSvc) *ConciliacaoHandler {
	return &ConciliacaoHandler{svc: svc}
}

func (h *ConciliacaoHandler) Conciliar(w http.ResponseWriter, r *http.Request) {
	dataStr := r.URL.Query().Get("data")
	data, err := time.Parse("2006-01-02", dataStr)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"erro": "data inválida: use YYYY-MM-DD"})
		return
	}

	resultado, err := h.svc.Conciliar(r.Context(), data)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"erro": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, resultado)
}
