package handler

// [LAB] Features experimentais (Diagnóstico de dupla contabilização + Linhagem).
// Gated pela flag FEATURES_LAB (lida no main). Desabilitado → status {ativo:false} e 501
// nos endpoints de dados, para a UI esconder o menu.

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"srcoff/internal/model"
)

type labSvc interface {
	Diagnosticar(ctx context.Context, data, produto, dominio string) (*model.DiagnosticoRegras, error)
	Linhagem(ctx context.Context, in model.LinhagemInput) (*model.Linhagem, error)
}

// LabHandler expõe as rotas experimentais do Lab.
type LabHandler struct {
	svc   labSvc
	ativo bool
}

func NewLabHandler(svc labSvc, ativo bool) *LabHandler {
	return &LabHandler{svc: svc, ativo: ativo}
}

func (h *LabHandler) habilitado() bool { return h.ativo && h.svc != nil }

// Status trata GET /api/v1/lab/status → {"ativo": bool}. A UI usa para mostrar/esconder o menu.
func (h *LabHandler) Status(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]bool{"ativo": h.habilitado()})
}

// Diagnostico trata GET /api/v1/lab/diagnostico (data?, produto?, dominio?).
func (h *LabHandler) Diagnostico(w http.ResponseWriter, r *http.Request) {
	if !h.habilitado() {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"erro": "feature Lab desabilitada (FEATURES_LAB)"})
		return
	}
	q := r.URL.Query()
	res, err := h.svc.Diagnosticar(r.Context(), strings.TrimSpace(q.Get("data")), strings.TrimSpace(q.Get("produto")), strings.TrimSpace(q.Get("dominio")))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"erro": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// Linhagem trata POST /api/v1/lab/linhagem (corpo = LinhagemInput).
func (h *LabHandler) Linhagem(w http.ResponseWriter, r *http.Request) {
	if !h.habilitado() {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"erro": "feature Lab desabilitada (FEATURES_LAB)"})
		return
	}
	var in model.LinhagemInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"erro": "corpo inválido"})
		return
	}
	res, err := h.svc.Linhagem(r.Context(), in)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"erro": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, res)
}
