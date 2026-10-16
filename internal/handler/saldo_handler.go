package handler

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"srcoff/internal/model"
)

// saldoSvc expõe a consulta da projeção de saldo (RN-170).
type saldoSvc interface {
	ListarSaldos(ctx context.Context, filtro model.SaldoFiltro) ([]model.SaldoConta, error)
}

type SaldoHandler struct {
	svc saldoSvc
}

func NewSaldoHandler(svc saldoSvc) *SaldoHandler { return &SaldoHandler{svc: svc} }

// Listar trata GET /api/v1/saldos.
// Spec: RF-065 (docs/especificacao.md §4.10, §11.5b).
// Query: produto, dominio, boleto, id_regra, com_zero=1 (inclui saldo 0).
func (h *SaldoHandler) Listar(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	q := r.URL.Query()
	filtro := model.SaldoFiltro{
		Produto:    strings.TrimSpace(q.Get("produto")),
		Dominio:    strings.TrimSpace(q.Get("dominio")),
		Boleto:     strings.TrimSpace(q.Get("boleto")),
		IncluiZero: q.Get("com_zero") == "1",
	}
	if v := q.Get("id_regra"); v != "" {
		if id, err := strconv.ParseInt(v, 10, 64); err == nil {
			filtro.IDRegra = id
		}
	}
	saldos, err := h.svc.ListarSaldos(r.Context(), filtro)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"erro": err.Error()})
		return
	}
	if saldos == nil {
		saldos = []model.SaldoConta{}
	}
	writeJSON(w, http.StatusOK, saldos)
}
