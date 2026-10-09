package handler

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"srcoff/internal/service"
)

type execucaoSvc interface {
	StatusPorData(ctx context.Context, data time.Time) ([]service.StatusCombinacao, error)
	CalendarioMes(ctx context.Context, ano int, mes time.Month) ([]service.DiaCalendario, error)
}

// ExecucaoHandler expõe a visão de processados/pendentes por data.
type ExecucaoHandler struct {
	svc execucaoSvc
}

func NewExecucaoHandler(svc execucaoSvc) *ExecucaoHandler {
	return &ExecucaoHandler{svc: svc}
}

// Status trata GET /api/v1/movimento-contabil/status?data=YYYY-MM-DD
// Spec: RF-062 (docs/especificacao.md §10, §11.1).
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

// Calendario trata GET /api/v1/movimento-contabil/calendario?ano=YYYY&mes=MM
// Retorna o status agregado de cada dia do mês (completo/parcial/nenhum/não útil/futuro),
// considerando todas as combinações (produto, domínio) dos padrões de arquivo.
// Spec: RF-063 (docs/especificacao.md §10, §11.1).
func (h *ExecucaoHandler) Calendario(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	ano, err := strconv.Atoi(q.Get("ano"))
	if err != nil || ano < 1900 || ano > 3000 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"erro": "ano inválido"})
		return
	}
	mes, err := strconv.Atoi(q.Get("mes"))
	if err != nil || mes < 1 || mes > 12 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"erro": "mês inválido (1-12)"})
		return
	}
	dias, err := h.svc.CalendarioMes(r.Context(), ano, time.Month(mes))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"erro": err.Error()})
		return
	}
	if dias == nil {
		dias = []service.DiaCalendario{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ano": ano, "mes": mes, "dias": dias,
	})
}
