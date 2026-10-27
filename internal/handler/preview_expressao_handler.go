package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"srcoff/internal/model"
)

type previewExpressaoSvc interface {
	PreviewExpressao(ctx context.Context, data time.Time, produto, dominio, boleto, expressao, tipo string) (*model.PreviewExpressao, error)
}

// PreviewExpressaoHandler avalia uma expressão contra uma posição real (RF-072).
type PreviewExpressaoHandler struct {
	svc previewExpressaoSvc
}

func NewPreviewExpressaoHandler(svc previewExpressaoSvc) *PreviewExpressaoHandler {
	return &PreviewExpressaoHandler{svc: svc}
}

// Preview trata POST /api/v1/preview-expressao.
// Body: { expressao, tipo ("condicao"|"valor"), data (AAAA-MM-DD), produto?, dominio?, boleto? }
func (h *PreviewExpressaoHandler) Preview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Expressao string `json:"expressao"`
		Tipo      string `json:"tipo"`
		Data      string `json:"data"`
		Produto   string `json:"produto"`
		Dominio   string `json:"dominio"`
		Boleto    string `json:"boleto"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Expressao) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"erro": "informe 'expressao'"})
		return
	}
	data, err := time.Parse("2006-01-02", strings.TrimSpace(req.Data))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"erro": "informe 'data' (AAAA-MM-DD)"})
		return
	}
	res, err := h.svc.PreviewExpressao(r.Context(), data, strings.TrimSpace(req.Produto), strings.TrimSpace(req.Dominio), strings.TrimSpace(req.Boleto), req.Expressao, req.Tipo)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"erro": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, res)
}
