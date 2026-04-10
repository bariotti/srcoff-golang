package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"srcoff/internal/model"
)

type movimentoContabilSvc interface {
	GerarMovimento(ctx context.Context, data time.Time) error
	GerarEstorno(ctx context.Context, data time.Time) error
	ConsultarLancamentos(ctx context.Context, data time.Time, pagina, tamanho int) (*model.PaginaLancamentos, error)
}

// MovimentoContabilHandler expõe os endpoints de movimento contábil.
type MovimentoContabilHandler struct {
	svc movimentoContabilSvc
}

// NewMovimentoContabilHandler cria uma nova instância do handler com a dependência injetada.
func NewMovimentoContabilHandler(svc movimentoContabilSvc) *MovimentoContabilHandler {
	return &MovimentoContabilHandler{svc: svc}
}

// writeJSON serializa v como JSON e escreve na resposta com o status informado.
func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

type dataPayload struct {
	Data string `json:"data"`
}

// GerarMovimento trata POST /api/v1/movimento-contabil.
func (h *MovimentoContabilHandler) GerarMovimento(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var payload dataPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"erro": "data inválida: use o formato YYYY-MM-DD"})
		return
	}

	data, err := time.Parse("2006-01-02", payload.Data)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"erro": "data inválida: use o formato YYYY-MM-DD"})
		return
	}

	if err := h.svc.GerarMovimento(r.Context(), data); err != nil {
		writeJSON(w, http.StatusOK, map[string]string{"mensagem": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"mensagem": "movimento contábil gerado com sucesso"})
}

// GerarEstorno trata POST /api/v1/estorno.
func (h *MovimentoContabilHandler) GerarEstorno(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var payload dataPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"erro": "data inválida: use o formato YYYY-MM-DD"})
		return
	}

	data, err := time.Parse("2006-01-02", payload.Data)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"erro": "data inválida: use o formato YYYY-MM-DD"})
		return
	}

	if err := h.svc.GerarEstorno(r.Context(), data); err != nil {
		writeJSON(w, http.StatusOK, map[string]string{"mensagem": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"mensagem": "estorno gerado com sucesso"})
}

// ConsultarMovimento trata GET /api/v1/movimento-contabil.
func (h *MovimentoContabilHandler) ConsultarMovimento(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	q := r.URL.Query()

	data, err := time.Parse("2006-01-02", q.Get("data"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"erro": "data inválida: use o formato YYYY-MM-DD"})
		return
	}

	pagina := 1
	if v, err := strconv.Atoi(q.Get("pagina")); err == nil && v > 0 {
		pagina = v
	}

	tamanho := 100
	if v, err := strconv.Atoi(q.Get("tamanho")); err == nil && v > 0 {
		tamanho = v
	}

	resultado, err := h.svc.ConsultarLancamentos(r.Context(), data, pagina, tamanho)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"erro": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, resultado)
}
