package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"

	"srcoff/internal/model"
)

type veiculoDeParaSvc interface {
	Listar(ctx context.Context) ([]model.VeiculoDePara, error)
	Criar(ctx context.Context, v model.VeiculoDePara) (int64, error)
	Excluir(ctx context.Context, id int64) error
}

// VeiculoDeParaHandler expõe o CRUD do De/Para de veículo → código (RF-035).
type VeiculoDeParaHandler struct {
	svc veiculoDeParaSvc
}

func NewVeiculoDeParaHandler(svc veiculoDeParaSvc) *VeiculoDeParaHandler {
	return &VeiculoDeParaHandler{svc: svc}
}

// VeiculoDePara trata /api/v1/parametrizacoes/veiculo-depara.
//
//	GET    → lista
//	POST   {veiculo, codigo} → cria
//	DELETE ?id=N → remove
//
// Spec: RF-035 (docs/especificacao.md §8.4, §11.4).
func (h *VeiculoDeParaHandler) VeiculoDePara(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		itens, err := h.svc.Listar(r.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"erro": err.Error()})
			return
		}
		if itens == nil {
			itens = []model.VeiculoDePara{}
		}
		writeJSON(w, http.StatusOK, itens)

	case http.MethodPost:
		var v model.VeiculoDePara
		if err := json.NewDecoder(r.Body).Decode(&v); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"erro": err.Error()})
			return
		}
		id, err := h.svc.Criar(r.Context(), v)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"erro": err.Error()})
			return
		}
		writeJSON(w, http.StatusCreated, map[string]int64{"id": id})

	case http.MethodDelete:
		id, err := strconv.ParseInt(r.URL.Query().Get("id"), 10, 64)
		if err != nil || id == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"erro": "id inválido"})
			return
		}
		if err := h.svc.Excluir(r.Context(), id); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"erro": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"mensagem": "de/para removido"})

	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}
