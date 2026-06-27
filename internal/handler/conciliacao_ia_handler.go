package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"srcoff/internal/model"
)

type conciliacaoIASvc interface {
	ConsultarLancamentosFiltrado(ctx context.Context, dataInicio, dataFim time.Time, boleto string, versao int, versaoModo string, pagina, tamanho int) (*model.PaginaLancamentos, error)
	BulkInsertAjuste(ctx context.Context, lancamentos []model.LancamentoContabil) error
}

type posicaoIASvc interface {
	ListarPorData(ctx context.Context, data time.Time) ([]model.PosicaoCarteira, error)
}

// ConciliacaoIAHandler processa conciliação em linguagem natural via Gemini.
type ConciliacaoIAHandler struct {
	movSvc posicaoIASvcWrapper
}

type posicaoIASvcWrapper struct {
	movSvc    conciliacaoIASvc
	posicaoSvc posicaoIASvc
}

func NewConciliacaoIAHandler(movSvc conciliacaoIASvc, posicaoSvc posicaoIASvc) *ConciliacaoIAHandler {
	return &ConciliacaoIAHandler{movSvc: posicaoIASvcWrapper{movSvc: movSvc, posicaoSvc: posicaoSvc}}
}

type ConciliacaoIARequest struct {
	Pergunta string `json:"pergunta"`
	Data     string `json:"data"`
}

type SugestaoAjuste struct {
	Descricao    string                   `json:"descricao"`
	Lancamentos  []model.LancamentoContabil `json:"lancamentos"`
}

type ConciliacaoIAResponse struct {
	Diagnostico string          `json:"diagnostico"`
	Sugestao    *SugestaoAjuste `json:"sugestao,omitempty"`
}

func (h *ConciliacaoIAHandler) Analisar(w http.ResponseWriter, r *http.Request) {
	var req ConciliacaoIARequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Pergunta == "" || req.Data == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"erro": "informe 'pergunta' e 'data'"})
		return
	}

	data, err := time.Parse("2006-01-02", req.Data)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"erro": "data inválida: use YYYY-MM-DD"})
		return
	}

	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"erro": "GEMINI_API_KEY não configurada"})
		return
	}

	// Buscar dados do dia
	posicoes, err := h.movSvc.posicaoSvc.ListarPorData(r.Context(), data)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"erro": "erro ao buscar posição: " + err.Error()})
		return
	}

	movimentos, err := h.movSvc.movSvc.ConsultarLancamentosFiltrado(r.Context(), data, data, "", 0, "vigente", 1, 999999)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"erro": "erro ao buscar movimentos: " + err.Error()})
		return
	}

	// Serializar dados para o Gemini
	posJson, _ := json.MarshalIndent(posicoes, "", "  ")
	movJson, _ := json.MarshalIndent(movimentos.Lancamentos, "", "  ")

	prompt := fmt.Sprintf(`Você é um especialista em contabilidade de tesouraria. Analise os dados abaixo e responda à pergunta do usuário.

PERGUNTA: %s
DATA: %s

POSIÇÃO DE CARTEIRA (versão máxima):
%s

MOVIMENTO CONTÁBIL (versão vigente):
%s

INSTRUÇÕES:
1. Analise se há inconsistência entre a posição e o movimento contábil com base na pergunta
2. Forneça um diagnóstico claro e objetivo
3. Se houver inconsistência, sugira lançamentos contábeis de ajuste no formato JSON abaixo
4. Se não houver inconsistência, diga que está conciliado

Responda OBRIGATORIAMENTE neste formato JSON (sem markdown):
{
  "diagnostico": "texto explicando o resultado da análise",
  "sugestao": {
    "descricao": "descrição do ajuste sugerido",
    "lancamentos": [
      {
        "CodigoIdentificadorBoleto": "BOL-001",
        "ValorLancamentoContabil": 1000.00,
        "MoedaLancamentoContabil": "BRL",
        "ContaDebito": "111111111",
        "ContaCredito": "222222222",
        "DescricaoRegraContabil": "Ajuste de conciliação",
        "DescricaoCondicaoContabil": "Ajuste manual via IA",
        "IDRegraContabil": 0
      }
    ]
  }
}

Se não houver sugestão de ajuste, omita o campo "sugestao" ou deixe como null.`,
		req.Pergunta, req.Data, string(posJson), string(movJson))

	geminiResp, err := chamarGemini(apiKey, prompt)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"erro": "erro ao chamar Gemini: " + err.Error()})
		return
	}

	// Tentar parsear resposta como JSON estruturado
	var resp ConciliacaoIAResponse
	cleaned := strings.TrimSpace(geminiResp)
	cleaned = strings.TrimPrefix(cleaned, "```json")
	cleaned = strings.TrimPrefix(cleaned, "```")
	cleaned = strings.TrimSuffix(cleaned, "```")
	cleaned = strings.TrimSpace(cleaned)

	if err := json.Unmarshal([]byte(cleaned), &resp); err != nil {
		// Fallback: retornar como texto puro
		resp = ConciliacaoIAResponse{Diagnostico: geminiResp}
	}

	writeJSON(w, http.StatusOK, resp)
}

func (h *ConciliacaoIAHandler) AplicarAjuste(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Data        string                   `json:"data"`
		Lancamentos []model.LancamentoContabil `json:"lancamentos"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"erro": err.Error()})
		return
	}

	data, err := time.Parse("2006-01-02", req.Data)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"erro": "data inválida"})
		return
	}

	// Preencher data e marcar como ajuste
	for i := range req.Lancamentos {
		req.Lancamentos[i].DataLoteContabil = data
		req.Lancamentos[i].IndicadorReversao = false
		if req.Lancamentos[i].DescricaoRegraContabil == "" {
			req.Lancamentos[i].DescricaoRegraContabil = "Ajuste de conciliação IA"
		}
	}

	if err := h.movSvc.movSvc.BulkInsertAjuste(r.Context(), req.Lancamentos); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"erro": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"mensagem": fmt.Sprintf("%d lançamento(s) de ajuste aplicado(s) com sucesso", len(req.Lancamentos))})
}

func chamarGemini(apiKey, prompt string) (string, error) {
	body := map[string]interface{}{
		"contents": []map[string]interface{}{
			{"parts": []map[string]string{{"text": prompt}}},
		},
		"generationConfig": map[string]interface{}{
			"temperature":     0.1,
			"maxOutputTokens": 2048,
		},
	}
	bodyBytes, _ := json.Marshal(body)
	url := "https://generativelanguage.googleapis.com/v1beta/models/gemini-2.0-flash:generateContent?key=" + apiKey

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
		return "", fmt.Errorf("resposta inválida: %s", string(raw))
	}
	if geminiResp.Error.Message != "" {
		return "", fmt.Errorf("Gemini: %s", geminiResp.Error.Message)
	}
	if len(geminiResp.Candidates) == 0 {
		return "", fmt.Errorf("Gemini não retornou resposta")
	}
	return geminiResp.Candidates[0].Content.Parts[0].Text, nil
}
