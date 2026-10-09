package handler

import (
	"context"
	"encoding/json"
	"net/http"
)

type configuracaoSvc interface {
	Obter(ctx context.Context, chave string) (string, error)
	Definir(ctx context.Context, chave, valor string) error
	ListarTodas(ctx context.Context) (map[string]string, error)
}

// ConfiguracaoHandler expõe leitura e escrita das configurações chave→valor.
type ConfiguracaoHandler struct {
	svc configuracaoSvc
}

func NewConfiguracaoHandler(svc configuracaoSvc) *ConfiguracaoHandler {
	return &ConfiguracaoHandler{svc: svc}
}

// Configuracoes trata /api/v1/configuracoes.
//   GET → retorna todas as configurações {chave: valor}
//   PUT {chave, valor} → define uma configuração
// Spec: RF-044 (docs/especificacao.md §8.4, §11.4).
func (h *ConfiguracaoHandler) Configuracoes(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		m, err := h.svc.ListarTodas(r.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"erro": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, m)

	case http.MethodPut:
		var req struct {
			Chave string `json:"chave"`
			Valor string `json:"valor"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Chave == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"erro": "informe 'chave' e 'valor'"})
			return
		}
		if err := h.svc.Definir(r.Context(), req.Chave, req.Valor); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"erro": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"mensagem": "configuração salva"})

	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}
