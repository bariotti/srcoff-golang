package handler

import (
	"context"
	"net/http"
	"time"

	"srcoff/internal/service"
)

type execucaoSvc interface {
	StatusPorData(ctx context.Context, data time.Time) ([]service.StatusCombinacao, error)
}

// ExecucaoHandler expõe a visão de processados/pendentes por data.
type ExecucaoHandler struct {
	svc execucaoSvc
}

func NewExecucaoHandler(svc execucaoSvc) *ExecucaoHandler {
	return &ExecucaoHandler{svc: svc}
}

// Status trata GET /api/v1/movimento-contabil/status?data=YYYY-MM-DD
func (h *ExecucaoHandler) Status(w http.ResponseWriter, r *http.Request) {
	data, err := time.Parse("2006-01-02", r.URL.Query().Get("data"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"erro": "data inválida: use YYYY-MM-DD"})
		return
	}
	status, err := h.svc.StatusPorData(r.Context(), data)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"erro": err.Error()})
		return
	}
	if status == nil {
		status = []service.StatusCombinacao{}
	}
	writeJSON(w, http.StatusOK, status)
}
