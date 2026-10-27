package handler

import (
	"context"
	"net/http"
	"strings"
	"time"

	"srcoff/internal/model"
)

type dashboardSvc interface {
	Gerar(ctx context.Context, inicio, fim time.Time) (*model.Dashboard, error)
}

type DashboardHandler struct {
	svc dashboardSvc
}

func NewDashboardHandler(svc dashboardSvc) *DashboardHandler { return &DashboardHandler{svc: svc} }

// Dashboard trata GET /api/v1/dashboard — visão consolidada do período.
// Query: data_inicio, data_fim (AAAA-MM-DD). Sem parâmetros, usa os últimos 30 dias.
// Spec: RF-120.
func (h *DashboardHandler) Dashboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	q := r.URL.Query()
	hoje := time.Now()
	inicio := hoje.AddDate(0, 0, -30)
	fim := hoje

	if v := strings.TrimSpace(q.Get("data_inicio")); v != "" {
		t, err := time.Parse("2006-01-02", v)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"erro": "data_inicio inválida"})
			return
		}
		inicio = t
	}
	if v := strings.TrimSpace(q.Get("data_fim")); v != "" {
		t, err := time.Parse("2006-01-02", v)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"erro": "data_fim inválida"})
			return
		}
		fim = t
	}

	d, err := h.svc.Gerar(r.Context(), inicio, fim)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"erro": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, d)
}
