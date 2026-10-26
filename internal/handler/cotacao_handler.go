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
	"time"

	"srcoff/internal/model"
)

type cotacaoSvc interface {
	Listar(ctx context.Context) ([]model.Cotacao, error)
	Criar(ctx context.Context, c model.Cotacao) (int64, error)
	Excluir(ctx context.Context, id int64) error
}

// CotacaoHandler expõe o CRUD das cotações (taxa de conversão entre moedas) — RF-037.
type CotacaoHandler struct {
	svc cotacaoSvc
}

func NewCotacaoHandler(svc cotacaoSvc) *CotacaoHandler { return &CotacaoHandler{svc: svc} }

// cotacaoJSON é a forma de entrada/saída (data como string AAAA-MM-DD).
type cotacaoJSON struct {
	ID      int64   `json:"id"`
	Data    string  `json:"data"`
	Moeda1  string  `json:"moeda1"`
	Moeda2  string  `json:"moeda2"`
	Direcao string  `json:"direcao"`
	Taxa    float64 `json:"taxa"`
}

// Cotacoes trata /api/v1/parametrizacoes/cotacoes (GET lista, POST cria, DELETE remove).
// Spec: RF-037 (docs/especificacao.md §8.5, §11.4).
func (h *CotacaoHandler) Cotacoes(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		itens, err := h.svc.Listar(r.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"erro": err.Error()})
			return
		}
		out := make([]cotacaoJSON, 0, len(itens))
		for _, c := range itens {
			out = append(out, cotacaoJSON{c.ID, c.Data.Format("2006-01-02"), c.Moeda1, c.Moeda2, c.Direcao, c.Taxa})
		}
		writeJSON(w, http.StatusOK, out)

	case http.MethodPost:
		var in cotacaoJSON
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"erro": err.Error()})
			return
		}
		data, err := time.Parse("2006-01-02", strings.TrimSpace(in.Data))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"erro": "data inválida: use AAAA-MM-DD"})
			return
		}
		id, err := h.svc.Criar(r.Context(), model.Cotacao{Data: data, Moeda1: in.Moeda1, Moeda2: in.Moeda2, Direcao: in.Direcao, Taxa: in.Taxa})
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
		writeJSON(w, http.StatusOK, map[string]string{"mensagem": "cotação removida"})

	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// ImportarCSV trata POST /api/v1/parametrizacoes/cotacoes/import (multipart, campo "arquivo").
// Colunas (delimitador ';'): data;moeda1;moeda2;direcao;taxa — todas obrigatórias.
func (h *CotacaoHandler) ImportarCSV(w http.ResponseWriter, r *http.Request) {
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
	itens, err := parseCotacoesCSV(file)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"erro": err.Error()})
		return
	}
	inseridos := 0
	for i, c := range itens {
		if _, err := h.svc.Criar(r.Context(), c); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"erro": fmt.Sprintf("linha %d: %v", i+1, err)})
			return
		}
		inseridos++
	}
	writeJSON(w, http.StatusCreated, map[string]interface{}{"inseridos": inseridos, "mensagem": fmt.Sprintf("%d cotação(ões) importada(s)", inseridos)})
}

func parseCotacoesCSV(r io.Reader) ([]model.Cotacao, error) {
	cr := csv.NewReader(r)
	cr.Comma = ';'
	cr.FieldsPerRecord = -1
	cr.TrimLeadingSpace = true
	registros, err := cr.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("erro ao ler CSV: %w", err)
	}
	if len(registros) < 2 {
		return nil, fmt.Errorf("CSV sem linhas de dados")
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
	var itens []model.Cotacao
	for _, linha := range registros[1:] {
		if len(linha) == 0 || strings.TrimSpace(strings.Join(linha, "")) == "" {
			continue
		}
		data, err := time.Parse("2006-01-02", col(linha, "data"))
		if err != nil {
			return nil, fmt.Errorf("data inválida %q (use AAAA-MM-DD)", col(linha, "data"))
		}
		taxa, _ := strconv.ParseFloat(strings.ReplaceAll(col(linha, "taxa"), ",", "."), 64)
		itens = append(itens, model.Cotacao{
			Data:    data,
			Moeda1:  col(linha, "moeda1"),
			Moeda2:  col(linha, "moeda2"),
			Direcao: col(linha, "direcao"),
			Taxa:    taxa,
		})
	}
	return itens, nil
}
