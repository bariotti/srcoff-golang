package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"srcoff/internal/model"
)

type reprocessamentoSvc interface {
	ReprocessarSeletivo(ctx context.Context, data time.Time, produto, dominio, boleto string) (*model.ReprocessamentoResultado, error)
}

// ReprocessamentoHandler expõe o reprocessamento seletivo do contábil (RF-137).
type ReprocessamentoHandler struct {
	svc reprocessamentoSvc
}

func NewReprocessamentoHandler(svc reprocessamentoSvc) *ReprocessamentoHandler {
	return &ReprocessamentoHandler{svc: svc}
}

// Reprocessar trata POST /api/v1/reprocessar. Body: {data, produto, dominio, boleto}.
func (h *ReprocessamentoHandler) Reprocessar(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var in struct {
		Data    string `json:"data"`
		Produto string `json:"produto"`
		Dominio string `json:"dominio"`
		Boleto  string `json:"boleto"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"erro": "corpo inválido"})
		return
	}
	data, err := time.Parse("2006-01-02", in.Data)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"erro": "data inválida (use AAAA-MM-DD)"})
		return
	}
	res, err := h.svc.ReprocessarSeletivo(r.Context(), data, in.Produto, in.Dominio, in.Boleto)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"erro": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, res)
}
