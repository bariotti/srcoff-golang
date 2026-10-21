package handler

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"srcoff/internal/model"
)

// lancamentoManualSvc insere lançamentos manuais (RF-033).
type lancamentoManualSvc interface {
	InserirLancamentosManuais(ctx context.Context, itens []model.LancamentoManualInput) (int, error)
}

type LancamentoManualHandler struct {
	svc lancamentoManualSvc
}

func NewLancamentoManualHandler(svc lancamentoManualSvc) *LancamentoManualHandler {
	return &LancamentoManualHandler{svc: svc}
}

// Inserir trata POST /api/v1/lancamentos-manuais (JSON: { "lancamentos": [ ... ] }).
// Spec: RF-033 (docs/especificacao.md §5b, §11.1).
func (h *LancamentoManualHandler) Inserir(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Lancamentos []model.LancamentoManualInput `json:"lancamentos"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"erro": "JSON inválido: " + err.Error()})
		return
	}
	if len(req.Lancamentos) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"erro": "informe ao menos um lançamento"})
		return
	}
	n, err := h.svc.InserirLancamentosManuais(r.Context(), req.Lancamentos)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"erro": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]interface{}{"inseridos": n, "mensagem": fmt.Sprintf("%d lançamento(s) manual(is) inserido(s)", n)})
}

// ImportarCSV trata POST /api/v1/lancamentos-manuais/import (multipart, campo "arquivo").
// Colunas: data;produto;dominio;id_regra;id_condicao;boleto;veiculo;valor;moeda;indicador_reversao
// Spec: RF-033 (docs/especificacao.md §5b, §11.1).
func (h *LancamentoManualHandler) ImportarCSV(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	file, _, err := r.FormFile("arquivo")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"erro": "informe o arquivo no campo 'arquivo'"})
		return
	}
	defer file.Close()
	itens, err := parseLancamentosManuaisCSV(file)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"erro": err.Error()})
		return
	}
	if len(itens) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"erro": "CSV sem linhas de dados"})
		return
	}
	n, err := h.svc.InserirLancamentosManuais(r.Context(), itens)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"erro": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]interface{}{"inseridos": n, "mensagem": fmt.Sprintf("%d lançamento(s) manual(is) inserido(s) via CSV", n)})
}

// parseLancamentosManuaisCSV lê o CSV (delimitador ';', cabeçalho nomeado).
func parseLancamentosManuaisCSV(r io.Reader) ([]model.LancamentoManualInput, error) {
	cr := csv.NewReader(r)
	cr.Comma = ';'
	cr.FieldsPerRecord = -1
	cr.TrimLeadingSpace = true
	registros, err := cr.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("erro ao ler CSV: %w", err)
	}
	if len(registros) < 2 {
		return nil, nil
	}
	idx := map[string]int{}
	for i, h := range registros[0] {
		idx[strings.ToLower(strings.TrimSpace(h))] = i
	}
	col := func(linha []string, nome string) string {
		if i, ok := idx[nome]; ok && i < len(linha) {
			return strings.TrimSpace(linha[i])
		}
		return ""
	}
	var itens []model.LancamentoManualInput
	for _, linha := range registros[1:] {
		if len(linha) == 0 || strings.TrimSpace(strings.Join(linha, "")) == "" {
			continue
		}
		idRegra, _ := strconv.ParseInt(col(linha, "id_regra"), 10, 64)
		idCond, _ := strconv.ParseInt(col(linha, "id_condicao"), 10, 64)
		valorStr := strings.ReplaceAll(col(linha, "valor"), ",", ".")
		valor, _ := strconv.ParseFloat(valorStr, 64)
		rev := col(linha, "indicador_reversao")
		itens = append(itens, model.LancamentoManualInput{
			Data:              col(linha, "data"),
			Produto:           col(linha, "produto"),
			Dominio:           col(linha, "dominio"),
			Boleto:            col(linha, "boleto"),
			Veiculo:           col(linha, "veiculo"),
			Moeda:             col(linha, "moeda"),
			IDRegra:           idRegra,
			IDCondicao:        idCond,
			Valor:             valor,
			IndicadorReversao: ehVerdadeiro(rev),
		})
	}
	return itens, nil
}

func ehVerdadeiro(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "sim", "true", "s", "x", "estorno":
		return true
	}
	return false
}
