package handler

import (
	"context"
	"net/http"
	"strings"
	"time"

	"srcoff/internal/model"
)

type razoneteSvc interface {
	Razonete(ctx context.Context, dataInicio, dataFim time.Time, boleto, conta, produto, dominio, versaoModo string) ([]model.RazoneteConta, error)
}

type RazoneteHandler struct {
	svc razoneteSvc
}

func NewRazoneteHandler(svc razoneteSvc) *RazoneteHandler { return &RazoneteHandler{svc: svc} }

// Razonete trata GET /api/v1/razonete — livro razão (T) por conta.
// Query: data_inicio, data_fim (obrigatórios), boleto, conta, produto, dominio, versao_modo.
// Spec: RF-070.
func (h *RazoneteHandler) Razonete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	q := r.URL.Query()
	di := strings.TrimSpace(q.Get("data_inicio"))
	df := strings.TrimSpace(q.Get("data_fim"))
	if di == "" || df == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"erro": "informe data_inicio e data_fim (AAAA-MM-DD)"})
		return
	}
	dataInicio, err := time.Parse("2006-01-02", di)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"erro": "data_inicio inválida"})
		return
	}
	dataFim, err := time.Parse("2006-01-02", df)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"erro": "data_fim inválida"})
		return
	}
	boleto := strings.TrimSpace(q.Get("boleto"))
	if boleto == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"erro": "informe o número do boleto"})
		return
	}
	versaoModo := q.Get("versao_modo")
	if versaoModo == "" {
		versaoModo = "vigente"
	}
	contas, err := h.svc.Razonete(r.Context(), dataInicio, dataFim,
		boleto, strings.TrimSpace(q.Get("conta")),
		strings.TrimSpace(q.Get("produto")), strings.TrimSpace(q.Get("dominio")), versaoModo)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"erro": err.Error()})
		return
	}
	if contas == nil {
		contas = []model.RazoneteConta{}
	}
	writeJSON(w, http.StatusOK, contas)
}
