package handler

import (
	"context"
	"encoding/json"
	"net/http"

	"srcoff/internal/model"
)

type simulacaoWhatIfSvc interface {
	SimularRegra(ctx context.Context, in model.SimulacaoRegraInput) (*model.SimulacaoWhatIf, error)
}

// SimulacaoWhatIfHandler simula uma regra candidata sobre a posição (RF-122).
type SimulacaoWhatIfHandler struct {
	svc simulacaoWhatIfSvc
}

func NewSimulacaoWhatIfHandler(svc simulacaoWhatIfSvc) *SimulacaoWhatIfHandler {
	return &SimulacaoWhatIfHandler{svc: svc}
}

// Simular trata POST /api/v1/simulacao-whatif.
// Body: SimulacaoRegraInput (data + escopo + regra candidata com condições).
func (h *SimulacaoWhatIfHandler) Simular(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var in model.SimulacaoRegraInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"erro": "corpo inválido"})
		return
	}
	res, err := h.svc.SimularRegra(r.Context(), in)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"erro": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, res)
}
