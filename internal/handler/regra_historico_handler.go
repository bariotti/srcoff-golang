package handler

import (
	"context"
	"net/http"
	"strconv"

	"srcoff/internal/model"
)

type regraHistoricoSvc interface {
	HistoricoDisponivel() bool
	Historico(ctx context.Context, idRegra int64) ([]model.RegraVersao, error)
	ResumoHistorico(ctx context.Context) (map[int64]model.RegraVersao, error)
	DiffVersoes(ctx context.Context, idRegra int64, versaoA, versaoB int) (model.DiffRegraVersoes, error)
	RestaurarVersao(ctx context.Context, idRegra int64, versao int) error
}

// RegraHistoricoHandler expõe o histórico de versões do cadastro de regras (RF-133).
// Recurso sqlite-first: quando o serviço não tem o repositório de versões, responde 501.
type RegraHistoricoHandler struct {
	svc regraHistoricoSvc
}

func NewRegraHistoricoHandler(svc regraHistoricoSvc) *RegraHistoricoHandler {
	return &RegraHistoricoHandler{svc: svc}
}

func (h *RegraHistoricoHandler) indisponivel(w http.ResponseWriter) bool {
	if h.svc == nil || !h.svc.HistoricoDisponivel() {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"erro": "histórico de versões indisponível neste backend (disponível apenas no SQLite)"})
		return true
	}
	return false
}

// Raiz trata GET /api/v1/regra-historico. Sem id_regra → resumo (última versão de cada regra);
// com id_regra → a linha do tempo completa daquela regra.
func (h *RegraHistoricoHandler) Raiz(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if h.indisponivel(w) {
		return
	}
	idStr := r.URL.Query().Get("id_regra")
	if idStr == "" {
		resumo, err := h.svc.ResumoHistorico(r.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"erro": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, resumo)
		return
	}
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"erro": "id_regra inválido"})
		return
	}
	versoes, err := h.svc.Historico(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"erro": err.Error()})
		return
	}
	if versoes == nil {
		versoes = []model.RegraVersao{}
	}
	writeJSON(w, http.StatusOK, versoes)
}

// Diff trata GET /api/v1/regra-historico/diff?id_regra=&a=&b=.
func (h *RegraHistoricoHandler) Diff(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if h.indisponivel(w) {
		return
	}
	id, err := strconv.ParseInt(r.URL.Query().Get("id_regra"), 10, 64)
	if err != nil || id == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"erro": "id_regra inválido"})
		return
	}
	a, _ := strconv.Atoi(r.URL.Query().Get("a"))
	b, _ := strconv.Atoi(r.URL.Query().Get("b"))
	diff, err := h.svc.DiffVersoes(r.Context(), id, a, b)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"erro": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, diff)
}

// Restaurar trata POST /api/v1/regra-historico/restaurar?id_regra=&versao=.
func (h *RegraHistoricoHandler) Restaurar(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if h.indisponivel(w) {
		return
	}
	id, err := strconv.ParseInt(r.URL.Query().Get("id_regra"), 10, 64)
	if err != nil || id == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"erro": "id_regra inválido"})
		return
	}
	versao, err := strconv.Atoi(r.URL.Query().Get("versao"))
	if err != nil || versao <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"erro": "versao inválida"})
		return
	}
	if err := h.svc.RestaurarVersao(r.Context(), id, versao); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"erro": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"mensagem": "versão restaurada com sucesso"})
}
