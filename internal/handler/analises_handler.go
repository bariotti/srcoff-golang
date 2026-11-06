package handler

import (
	"context"
	"net/http"
	"strings"
	"time"

	"srcoff/internal/model"
)

type analisesMovSvc interface {
	Balancete(ctx context.Context, dataInicio, dataFim time.Time, produto, dominio, versaoModo string) (*model.Balancete, error)
	CoberturaPosicoes(ctx context.Context, data time.Time, produto, dominio string) (*model.Cobertura, error)
	SerieSaldoConta(ctx context.Context, dataInicio, dataFim time.Time, conta, produto, dominio string) (*model.SerieSaldo, error)
	Conciliar(ctx context.Context, data time.Time, produto, dominio string) (*model.Conciliacao, error)
}

type analisesRegraSvc interface {
	Lint(ctx context.Context) (*model.LintResultado, error)
}

// AnalisesHandler expõe análises contábeis: balancete (RF-127), cobertura de posições
// (RF-128), lint de regras (RF-129) e série de saldo por conta (RF-130).
type AnalisesHandler struct {
	mov   analisesMovSvc
	regra analisesRegraSvc
}

func NewAnalisesHandler(mov analisesMovSvc, regra analisesRegraSvc) *AnalisesHandler {
	return &AnalisesHandler{mov: mov, regra: regra}
}

// periodo lê data_inicio/data_fim (com defaults amplos) da query.
func periodo(r *http.Request) (time.Time, time.Time, error) {
	q := r.URL.Query()
	ini := q.Get("data_inicio")
	fim := q.Get("data_fim")
	if ini == "" {
		ini = "2000-01-01"
	}
	if fim == "" {
		fim = "2999-12-31"
	}
	di, err := time.Parse("2006-01-02", ini)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	df, err := time.Parse("2006-01-02", fim)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	return di, df, nil
}

// Balancete trata GET /api/v1/balancete (data_inicio, data_fim, produto?, dominio?, versao_modo?).
func (h *AnalisesHandler) Balancete(w http.ResponseWriter, r *http.Request) {
	di, df, err := periodo(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"erro": "datas inválidas (AAAA-MM-DD)"})
		return
	}
	q := r.URL.Query()
	res, err := h.mov.Balancete(r.Context(), di, df, strings.TrimSpace(q.Get("produto")), strings.TrimSpace(q.Get("dominio")), q.Get("versao_modo"))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"erro": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// Cobertura trata GET /api/v1/cobertura (data, produto?, dominio?).
func (h *AnalisesHandler) Cobertura(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	data, err := time.Parse("2006-01-02", strings.TrimSpace(q.Get("data")))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"erro": "informe 'data' (AAAA-MM-DD)"})
		return
	}
	res, err := h.mov.CoberturaPosicoes(r.Context(), data, strings.TrimSpace(q.Get("produto")), strings.TrimSpace(q.Get("dominio")))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"erro": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// Serie trata GET /api/v1/serie-saldo (conta, data_inicio, data_fim, produto?, dominio?).
func (h *AnalisesHandler) Serie(w http.ResponseWriter, r *http.Request) {
	di, df, err := periodo(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"erro": "datas inválidas (AAAA-MM-DD)"})
		return
	}
	q := r.URL.Query()
	conta := strings.TrimSpace(q.Get("conta"))
	if conta == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"erro": "informe a 'conta'"})
		return
	}
	res, err := h.mov.SerieSaldoConta(r.Context(), di, df, conta, strings.TrimSpace(q.Get("produto")), strings.TrimSpace(q.Get("dominio")))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"erro": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// Conciliacao trata GET /api/v1/conciliacao (data, produto?, dominio?) — Saldo×Posição + órfãos.
func (h *AnalisesHandler) Conciliacao(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	data, err := time.Parse("2006-01-02", strings.TrimSpace(q.Get("data")))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"erro": "informe 'data' (AAAA-MM-DD)"})
		return
	}
	res, err := h.mov.Conciliar(r.Context(), data, strings.TrimSpace(q.Get("produto")), strings.TrimSpace(q.Get("dominio")))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"erro": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// Lint trata GET /api/v1/regras/lint.
func (h *AnalisesHandler) Lint(w http.ResponseWriter, r *http.Request) {
	res, err := h.regra.Lint(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"erro": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, res)
}
