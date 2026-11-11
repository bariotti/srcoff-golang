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
	"sync"
	"time"
)

// OllamaClient fala com um Ollama LOCAL (https://ollama.com) — IA 100% gratuita e offline, sem
// chave. Configuração: OLLAMA_URL (padrão http://localhost:11434) e OLLAMA_MODEL (padrão llama3.1).
// Disponível quando o servidor local responde.
type OllamaClient struct {
	baseURL string
	modelo  string
	http    *http.Client

	mu       sync.Mutex
	okAte    time.Time
	okCache  bool
}

func NewOllamaClient() *OllamaClient {
	url := strings.TrimRight(strings.TrimSpace(os.Getenv("OLLAMA_URL")), "/")
	if url == "" {
		url = "http://localhost:11434"
	}
	modelo := strings.TrimSpace(os.Getenv("OLLAMA_MODEL"))
	if modelo == "" {
		modelo = "llama3.1"
	}
	return &OllamaClient{baseURL: url, modelo: modelo, http: &http.Client{Timeout: timeoutRequisao}}
}

func (c *OllamaClient) Nome() string   { return "ollama" }
func (c *OllamaClient) Modelo() string { return c.modelo }

// Disponivel verifica (com cache curto) se o Ollama local está no ar.
func (c *OllamaClient) Disponivel() bool {
	if c == nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if time.Now().Before(c.okAte) {
		return c.okCache
	}
	ctx, cancel := context.WithTimeout(context.Background(), 900*time.Millisecond)
	defer cancel()
	c.okCache = pingURL(ctx, c.baseURL+"/api/tags", 900*time.Millisecond)
	c.okAte = time.Now().Add(10 * time.Second)
	return c.okCache
}

func (c *OllamaClient) Gerar(ctx context.Context, system string, mensagens []Mensagem, maxTokens int) (string, error) {
	type msg struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	ms := make([]msg, 0, len(mensagens)+1)
	if strings.TrimSpace(system) != "" {
		ms = append(ms, msg{Role: "system", Content: system})
	}
	for _, m := range mensagens {
		role := "user"
		if m.Role == "assistant" {
			role = "assistant"
		}
		ms = append(ms, msg{Role: role, Content: m.Texto})
	}
	body, _ := json.Marshal(map[string]interface{}{
		"model":    c.modelo,
		"messages": ms,
		"stream":   false,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("content-type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("falha ao chamar o Ollama em %s (o servidor local está rodando?): %w", c.baseURL, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var parsed struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", fmt.Errorf("resposta inválida do Ollama (HTTP %d): %s", resp.StatusCode, trunc(string(raw), 300))
	}
	if parsed.Error != "" {
		return "", fmt.Errorf("Ollama: %s", parsed.Error)
	}
	texto := strings.TrimSpace(parsed.Message.Content)
	if texto == "" {
		return "", fmt.Errorf("o Ollama não retornou texto")
	}
	return texto, nil
}
