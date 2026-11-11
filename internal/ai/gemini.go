package ai

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
)

// GeminiClient fala com a API Gemini (Google). A chave tem FREE TIER (aistudio.google.com),
// então é uma opção gratuita de IA para o SRCOff. Chave em GEMINI_API_KEY; modelo em
// GEMINI_MODEL (padrão gemini-3.5-flash).
type GeminiClient struct {
	apiKey string
	modelo string
	http   *http.Client
}

func NewGeminiClient() *GeminiClient {
	modelo := strings.TrimSpace(os.Getenv("GEMINI_MODEL"))
	if modelo == "" {
		// gemini-3.8-flash e gemini-flash-latest travam no endpoint generateContent
		// (só respondem pela nova Interactions API); gemini-3.5-flash responde normalmente.
		// Sobreponível por GEMINI_MODEL.
		modelo = "gemini-3.5-flash"
	}
	return &GeminiClient{
		apiKey: strings.TrimSpace(os.Getenv("GEMINI_API_KEY")),
		modelo: modelo,
		http:   &http.Client{Timeout: timeoutRequisao},
	}
}

func (c *GeminiClient) Disponivel() bool { return c != nil && c.apiKey != "" }
func (c *GeminiClient) Nome() string     { return "gemini" }
func (c *GeminiClient) Modelo() string   { return c.modelo }

func (c *GeminiClient) Gerar(ctx context.Context, system string, mensagens []Mensagem, maxTokens int) (string, error) {
	if !c.Disponivel() {
		return "", fmt.Errorf("GEMINI_API_KEY não configurada")
	}
	if maxTokens <= 0 {
		maxTokens = 4096
	}
	type part struct {
		Text string `json:"text"`
	}
	type content struct {
		Role  string `json:"role,omitempty"`
		Parts []part `json:"parts"`
	}
	var contents []content
	for _, m := range mensagens {
		role := "user"
		if m.Role == "assistant" {
			role = "model"
		}
		contents = append(contents, content{Role: role, Parts: []part{{Text: m.Texto}}})
	}
	body := map[string]interface{}{
		"contents":         contents,
		"generationConfig": map[string]interface{}{"temperature": 0, "maxOutputTokens": maxTokens},
	}
	if strings.TrimSpace(system) != "" {
		body["systemInstruction"] = content{Parts: []part{{Text: system}}}
	}
	bodyBytes, _ := json.Marshal(body)
	url := "https://generativelanguage.googleapis.com/v1beta/models/" + c.modelo + ":generateContent?key=" + c.apiKey

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", err
	}
	req.Header.Set("content-type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("falha ao chamar o Gemini: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)

	var parsed struct {
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
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", fmt.Errorf("resposta inválida do Gemini (HTTP %d): %s", resp.StatusCode, trunc(string(raw), 300))
	}
	if parsed.Error.Message != "" {
		return "", fmt.Errorf("Gemini: %s", parsed.Error.Message)
	}
	if len(parsed.Candidates) == 0 || len(parsed.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("Gemini não retornou texto")
	}
	var sb strings.Builder
	for _, p := range parsed.Candidates[0].Content.Parts {
		sb.WriteString(p.Text)
	}
	return strings.TrimSpace(sb.String()), nil
}

// pingable é um atalho para GET com timeout curto (usado pelo Ollama).
func pingURL(ctx context.Context, url string, d time.Duration) bool {
	c := &http.Client{Timeout: d}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false
	}
	resp, err := c.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode < 500
}
