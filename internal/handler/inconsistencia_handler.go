package handler

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"net/http"
	"time"

	"srcoff/internal/model"
)

type inconsistenciaSvc interface {
	ListarPorData(ctx context.Context, data time.Time) ([]model.InconsistenciaProcessamento, error)
}

// InconsistenciaHandler expõe a consulta e exportação das inconsistências de processamento.
type InconsistenciaHandler struct {
	svc inconsistenciaSvc
}

func NewInconsistenciaHandler(svc inconsistenciaSvc) *InconsistenciaHandler {
	return &InconsistenciaHandler{svc: svc}
}

// Listar trata GET /api/v1/inconsistencias?data=YYYY-MM-DD
func (h *InconsistenciaHandler) Listar(w http.ResponseWriter, r *http.Request) {
	data, err := time.Parse("2006-01-02", r.URL.Query().Get("data"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"erro": "data inválida: use YYYY-MM-DD"})
		return
	}
	itens, err := h.svc.ListarPorData(r.Context(), data)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"erro": err.Error()})
		return
	}
	if itens == nil {
		itens = []model.InconsistenciaProcessamento{}
	}
	writeJSON(w, http.StatusOK, itens)
}

// Export trata GET /api/v1/inconsistencias/export?data=YYYY-MM-DD → CSV.
func (h *InconsistenciaHandler) Export(w http.ResponseWriter, r *http.Request) {
	dataStr := r.URL.Query().Get("data")
	data, err := time.Parse("2006-01-02", dataStr)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"erro": "data inválida: use YYYY-MM-DD"})
		return
	}
	itens, err := h.svc.ListarPorData(r.Context(), data)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"erro": err.Error()})
		return
	}

	var buf bytes.Buffer
	buf.Write([]byte{0xEF, 0xBB, 0xBF}) // BOM UTF-8 para Excel
	writer := csv.NewWriter(&buf)
	writer.Comma = ';'
	writer.Write([]string{
		"Data Lote", "Boleto", "Produto", "ID Regra", "Regra",
		"Tipo", "Expressão", "Campos Faltantes", "Detalhe",
	})
	for _, i := range itens {
		writer.Write([]string{
			i.DataLoteContabil.Format("2006-01-02"),
			i.CodigoIdentificadorBoleto,
			i.Produto,
			fmt.Sprintf("%d", i.IDRegraContabil),
			i.DescricaoRegraContabil,
			i.Tipo,
			i.Expressao,
			i.CamposFaltantes,
			i.Detalhe,
		})
	}
	writer.Flush()

	filename := fmt.Sprintf("inconsistencias_%s.csv", data.Format("20060102"))
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
	w.WriteHeader(http.StatusOK)
	w.Write(buf.Bytes())
}
