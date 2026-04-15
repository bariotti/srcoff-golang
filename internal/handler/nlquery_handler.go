package handler

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

// NLQueryHandler processa perguntas em linguagem natural e as converte em SQL via Gemini.
type NLQueryHandler struct {
	db *sql.DB
}

func NewNLQueryHandler(db *sql.DB) *NLQueryHandler {
	return &NLQueryHandler{db: db}
}

const dbSchema = `
Tabelas disponíveis no banco de dados srcoff:

1. posicao_carteira
   - id (BIGINT), data_posicao_carteira (DATE), codigo_versao_conteudo (INT)
   - codigo_identificador_boleto (VARCHAR), descricao_veiculo (VARCHAR)
   - indicador_contraparte_afiliada (BIT), valor_mtm (DECIMAL)
   - principal_remanescente (DECIMAL), moeda_principal_remanescente (VARCHAR)

2. regra_contabil
   - id (BIGINT), descricao (VARCHAR), codigo_produto_corporativo (VARCHAR), ativo (BIT)

3. condicao_regra
   - id (BIGINT), id_regra (BIGINT), condicao (VARCHAR)
   - conta_debito (VARCHAR), conta_credito (VARCHAR)
   - campo_valor (VARCHAR), campo_moeda (VARCHAR), ativo (BIT)

4. movimento_contabil
   - id (BIGINT), data_lote_contabil (DATE), codigo_versao_conteudo (INT)
   - codigo_identificador_boleto (VARCHAR), valor_lancamento_contabil (DECIMAL)
   - moeda_lancamento_contabil (VARCHAR), conta_debito (VARCHAR), conta_credito (VARCHAR)
   - indicador_reversao (BIT), descricao_regra_contabil (VARCHAR)
   - descricao_condicao_contabil (VARCHAR), id_regra_contabil (BIGINT)
`

func (h *NLQueryHandler) Query(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Pergunta string `json:"pergunta"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Pergunta == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"erro": "informe o campo 'pergunta'"})
		return
	}

	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"erro": "GEMINI_API_KEY não configurada"})
		return
	}

	// 1. Gerar SQL via Gemini
	sqlQuery, err := gerarSQL(r.Context(), apiKey, req.Pergunta)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"erro": "erro ao gerar SQL: " + err.Error()})
		return
	}

	// 2. Executar SQL no banco
	rows, err := h.db.QueryContext(r.Context(), sqlQuery)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{
			"erro":  "erro ao executar SQL: " + err.Error(),
			"sql":   sqlQuery,
		})
		return
	}
	defer rows.Close()

	cols, _ := rows.Columns()
	var resultado []map[string]interface{}
	for rows.Next() {
		vals := make([]interface{}, len(cols))
		ptrs := make([]interface{}, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		rows.Scan(ptrs...)
		row := make(map[string]interface{}, len(cols))
		for i, col := range cols {
			v := vals[i]
			if b, ok := v.([]byte); ok {
				row[col] = string(b)
			} else {
				row[col] = v
			}
		}
		resultado = append(resultado, row)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"sql":       sqlQuery,
		"total":     len(resultado),
		"resultado": resultado,
	})
}

func gerarSQL(ctx context.Context, apiKey, pergunta string) (string, error) {
	prompt := fmt.Sprintf(`Você é um especialista em SQL Server. Com base no schema abaixo, gere APENAS a query SQL para responder à pergunta do usuário.

REGRAS:
- Retorne SOMENTE o SQL, sem explicações, sem markdown, sem blocos de código
- Use apenas SELECT (nunca INSERT, UPDATE, DELETE, DROP, etc.)
- Use TOP 1000 para limitar resultados quando não houver filtro específico
- Para datas use o formato 'YYYY-MM-DD'
- Sempre use aliases descritivos nas colunas agregadas

SCHEMA:
%s

PERGUNTA: %s

SQL:`, dbSchema, pergunta)

	body := map[string]interface{}{
		"contents": []map[string]interface{}{
			{"parts": []map[string]string{{"text": prompt}}},
		},
		"generationConfig": map[string]interface{}{
			"temperature": 0,
			"maxOutputTokens": 1024,
		},
	}

	bodyBytes, _ := json.Marshal(body)
	url := "https://generativelanguage.googleapis.com/v1beta/models/gemini-1.5-flash:generateContent?key=" + apiKey

	resp, err := http.Post(url, "application/json", bytes.NewReader(bodyBytes))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	var geminiResp struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &geminiResp); err != nil {
		return "", fmt.Errorf("resposta inválida do Gemini: %s", string(raw))
	}
	if geminiResp.Error.Message != "" {
		return "", fmt.Errorf("Gemini: %s", geminiResp.Error.Message)
	}
	if len(geminiResp.Candidates) == 0 || len(geminiResp.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("Gemini não retornou SQL")
	}

	sqlResult := strings.TrimSpace(geminiResp.Candidates[0].Content.Parts[0].Text)
	// Remover possíveis blocos markdown
	sqlResult = strings.TrimPrefix(sqlResult, "```sql")
	sqlResult = strings.TrimPrefix(sqlResult, "```")
	sqlResult = strings.TrimSuffix(sqlResult, "```")
	return strings.TrimSpace(sqlResult), nil
}
