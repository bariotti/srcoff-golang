package handler

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"srcoff/internal/model"
)

type diffVersoesSvc interface {
	DiffVersoes(ctx context.Context, in model.DiffVersoesInput) (*model.DiffVersoes, error)
}

// DiffVersoesHandler compara duas versões do movimento de um (data, produto, domínio) — RF-124.
type DiffVersoesHandler struct{ svc diffVersoesSvc }

func NewDiffVersoesHandler(svc diffVersoesSvc) *DiffVersoesHandler { return &DiffVersoesHandler{svc: svc} }

// Diff trata GET /api/v1/movimento-contabil/diff.
// Query: data (AAAA-MM-DD), produto, dominio, versao_a?, versao_b? (<=0 = padrão vigente×anterior).
func (h *DiffVersoesHandler) Diff(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	q := r.URL.Query()
	va, _ := strconv.Atoi(q.Get("versao_a"))
	vb, _ := strconv.Atoi(q.Get("versao_b"))
	res, err := h.svc.DiffVersoes(r.Context(), model.DiffVersoesInput{
		Data:    strings.TrimSpace(q.Get("data")),
		Produto: strings.TrimSpace(q.Get("produto")),
		Dominio: strings.TrimSpace(q.Get("dominio")),
		VersaoA: va, VersaoB: vb,
	})
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"erro": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, res)
}
