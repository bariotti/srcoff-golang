package handler

import (
	"context"
	"encoding/json"
	"net/http"

	"srcoff/internal/model"
)

type backtestSvc interface {
	BacktestRegra(ctx context.Context, in model.BacktestInput) (*model.BacktestResultado, error)
}

// BacktestHandler roda o backtest de uma regra candidata sobre N dias históricos (RF-135).
type BacktestHandler struct {
	svc backtestSvc
}

func NewBacktestHandler(svc backtestSvc) *BacktestHandler {
	return &BacktestHandler{svc: svc}
}

// Backtest trata POST /api/v1/backtest-regra.
// Body: BacktestInput (dias + regra candidata, igual ao What-if, mas sem a data).
func (h *BacktestHandler) Backtest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var in model.BacktestInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"erro": "corpo inválido"})
		return
	}
	res, err := h.svc.BacktestRegra(r.Context(), in)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"erro": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, res)
}
