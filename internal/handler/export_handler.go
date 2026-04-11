package handler

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"srcoff/internal/model"
)

type exportMovimentoSvc interface {
	ConsultarLancamentosFiltrado(ctx context.Context, dataInicio, dataFim time.Time, boleto string, versao int, versaoModo string, pagina, tamanho int) (*model.PaginaLancamentos, error)
}

type ExportHandler struct {
	svc exportMovimentoSvc
}

func NewExportHandler(svc exportMovimentoSvc) *ExportHandler {
	return &ExportHandler{svc: svc}
}

// ExportMovimentoCSV trata GET /api/v1/movimento-contabil/export
// Retorna um arquivo CSV com todos os lançamentos do filtro informado.
func (h *ExportHandler) ExportMovimentoCSV(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	dataInicioStr := q.Get("data_inicio")
	dataFimStr := q.Get("data_fim")
	boleto := q.Get("boleto")
	versaoModo := q.Get("versao_modo")
	if versaoModo == "" {
		versaoModo = "vigente"
	}
	versao := 0
	if versaoModo == "especifica" {
		versao, _ = strconv.Atoi(q.Get("versao"))
	}
	if dataInicioStr == "" {
		dataInicioStr = "2000-01-01"
	}
	if dataFimStr == "" {
		dataFimStr = "2999-12-31"
	}

	dataInicio, err := time.Parse("2006-01-02", dataInicioStr)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"erro": "data_inicio inválida"})
		return
	}
	dataFim, err := time.Parse("2006-01-02", dataFimStr)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"erro": "data_fim inválida"})
		return
	}

	// Buscar todos os registros sem paginação
	resultado, err := h.svc.ConsultarLancamentosFiltrado(r.Context(), dataInicio, dataFim, boleto, versao, versaoModo, 1, 999999)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"erro": err.Error()})
		return
	}

	var buf bytes.Buffer
	// BOM UTF-8 para Excel reconhecer acentos
	buf.Write([]byte{0xEF, 0xBB, 0xBF})

	writer := csv.NewWriter(&buf)
	writer.Comma = ';'

	// Cabeçalho
	writer.Write([]string{
		"Data Lote",
		"Versão",
		"Boleto",
		"Conta Débito",
		"Conta Crédito",
		"Valor",
		"Moeda",
		"Reversão",
		"Regra",
		"Condição",
	})

	for _, l := range resultado.Lancamentos {
		reversao := "Não"
		if l.IndicadorReversao {
			reversao = "Sim"
		}
		writer.Write([]string{
			l.DataLoteContabil.Format("2006-01-02"),
			strconv.Itoa(l.CodigoVersaoConteudo),
			l.CodigoIdentificadorBoleto,
			l.ContaDebito,
			l.ContaCredito,
			fmt.Sprintf("%.6f", l.ValorLancamentoContabil),
			l.MoedaLancamentoContabil,
			reversao,
			l.DescricaoRegraContabil,
			l.DescricaoCondicaoContabil,
		})
	}
	writer.Flush()

	filename := fmt.Sprintf("movimento_contabil_%s_%s.csv", dataInicioStr, dataFimStr)
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
	w.WriteHeader(http.StatusOK)
	w.Write(buf.Bytes())
}

// ExportMovimentoTXT trata GET /api/v1/movimento-contabil/export-txt?data=YYYY-MM-DD
// Retorna arquivo TXT no formato específico com cabeçalho, detalhes e totalizador.
func (h *ExportHandler) ExportMovimentoTXT(w http.ResponseWriter, r *http.Request) {
	dataStr := r.URL.Query().Get("data")
	data, err := time.Parse("2006-01-02", dataStr)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"erro": "data inválida: use YYYY-MM-DD"})
		return
	}

	// Buscar lançamentos da data (versão vigente, sem cancelados)
	resultado, err := h.svc.ConsultarLancamentosFiltrado(r.Context(), data, data, "", 0, "vigente", 1, 999999)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"erro": err.Error()})
		return
	}

	if resultado.Total == 0 {
		writeJSON(w, http.StatusOK, map[string]string{"sem_dados": "sem dados para essa data de movimento contábil"})
		return
	}

	var buf bytes.Buffer

	// Linha de cabeçalho: C;AAAAMMDD
	buf.WriteString("C;" + data.Format("20060102") + "\n")

	// Linhas de detalhe e soma total
	var somaTotal float64
	for _, l := range resultado.Lancamentos {
		reversao := "N"
		if l.IndicadorReversao {
			reversao = "S"
		}
		somaTotal += l.ValorLancamentoContabil

		// Linha débito: D;conta_debito;D;moeda;regra;boleto;reversao;valor
		buf.WriteString(fmt.Sprintf("D;%s;D;%s;%s;%s;%s;%.6f\n",
			l.ContaDebito,
			l.MoedaLancamentoContabil,
			l.DescricaoRegraContabil,
			l.CodigoIdentificadorBoleto,
			reversao,
			l.ValorLancamentoContabil,
		))

		// Linha crédito: D;conta_credito;C;moeda;regra;boleto;reversao;valor
		buf.WriteString(fmt.Sprintf("D;%s;C;%s;%s;%s;%s;%.6f\n",
			l.ContaCredito,
			l.MoedaLancamentoContabil,
			l.DescricaoRegraContabil,
			l.CodigoIdentificadorBoleto,
			reversao,
			l.ValorLancamentoContabil,
		))
	}

	// Linha totalizador: T;soma
	buf.WriteString(fmt.Sprintf("T;%.6f\n", somaTotal))

	filename := fmt.Sprintf("movimento_contabil_%s.txt", data.Format("20060102"))
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
	w.WriteHeader(http.StatusOK)
	w.Write(buf.Bytes())
}
