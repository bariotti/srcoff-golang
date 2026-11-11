package handler

import (
	"context"
	"encoding/json"
	"net/http"

	"srcoff/internal/model"
)

type copilotSvc interface {
	Disponivel() bool
	Modelo() string
	Provedor() string
	Conversar(ctx context.Context, mensagens []model.CopilotMensagem) (model.CopilotResposta, error)
	ExecutarAcao(ctx context.Context, operacao string, args map[string]interface{}) (string, error)
}

// CopilotHandler expõe o assistente conversacional (RF-138). O chat só é habilitado quando a
// chave da Claude está parametrizada (ANTHROPIC_API_KEY) — ver Status.
type CopilotHandler struct {
	svc copilotSvc
}

func NewCopilotHandler(svc copilotSvc) *CopilotHandler {
	return &CopilotHandler{svc: svc}
}

// Status trata GET /api/v1/copilot/status → {disponivel, modelo}. A UI usa para habilitar o chat.
func (h *CopilotHandler) Status(w http.ResponseWriter, r *http.Request) {
	disp := h.svc != nil && h.svc.Disponivel()
	modelo, provedor := "", ""
	if h.svc != nil {
		modelo = h.svc.Modelo()
		provedor = h.svc.Provedor()
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"disponivel": disp, "modelo": modelo, "provedor": provedor})
}

// Chat trata POST /api/v1/copilot. Body: {mensagens:[{role,texto}...]}.
func (h *CopilotHandler) Chat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var in struct {
		Mensagens []model.CopilotMensagem `json:"mensagens"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || len(in.Mensagens) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"erro": "informe 'mensagens'"})
		return
	}
	resp, err := h.svc.Conversar(r.Context(), in.Mensagens)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"erro": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

// Acao trata POST /api/v1/copilot/acao — executa uma ação confirmada pelo usuário.
// Body: {operacao, args}.
func (h *CopilotHandler) Acao(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var in struct {
		Operacao string                 `json:"operacao"`
		Args     map[string]interface{} `json:"args"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.Operacao == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"erro": "informe 'operacao'"})
		return
	}
	texto, err := h.svc.ExecutarAcao(r.Context(), in.Operacao, in.Args)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"erro": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"texto": texto})
}
