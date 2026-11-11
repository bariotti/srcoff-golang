// Package ai integra o SRCOff com a API de mensagens da Claude (Anthropic). A chave vem SEMPRE
// da variável de ambiente ANTHROPIC_API_KEY (nunca embutida no código/binário). O modelo padrão
// é claude-opus-5-5, sobreponível por ANTHROPIC_MODEL. Chamada via HTTP direto (net/http), no
// mesmo padrão já usado no projeto, para preservar o build offline (sem dependência nova).
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

const (
	apiURL          = "https://api.anthropic.com/v1/messages"
	apiVersion      = "2023-06-01"
	modeloPadrao    = "claude-opus-5-5"
	timeoutRequisao = 120 * time.Second
)

// Mensagem é uma mensagem da conversa (role: "user" ou "assistant").
type Mensagem struct {
	Role  string
	Texto string
}

// ClaudeClient fala com a Anthropic Messages API.
type ClaudeClient struct {
	apiKey string
	modelo string
	http   *http.Client
}

// NewClaudeClient lê ANTHROPIC_API_KEY e ANTHROPIC_MODEL do ambiente. Nunca recebe a chave por
// parâmetro — ela só vem do ambiente, por decisão de segurança.
func NewClaudeClient() *ClaudeClient {
	modelo := strings.TrimSpace(os.Getenv("ANTHROPIC_MODEL"))
	if modelo == "" {
		modelo = modeloPadrao
	}
	return &ClaudeClient{
		apiKey: strings.TrimSpace(os.Getenv("ANTHROPIC_API_KEY")),
		modelo: modelo,
		http:   &http.Client{Timeout: timeoutRequisao},
	}
}

// Disponivel indica se a chave está parametrizada (habilita o chat).
func (c *ClaudeClient) Disponivel() bool { return c != nil && c.apiKey != "" }

// Nome do provedor (para status/UI).
func (c *ClaudeClient) Nome() string { return "claude" }

// Modelo devolve o modelo configurado (para exibição no status).
func (c *ClaudeClient) Modelo() string { return c.modelo }

type reqMensagem struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type reqBody struct {
	Model     string        `json:"model"`
	MaxTokens int           `json:"max_tokens"`
	System    string        `json:"system,omitempty"`
	Messages  []reqMensagem `json:"messages"`
}

type respBody struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	StopReason string `json:"stop_reason"`
	Error      struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

// Gerar envia a conversa (system + mensagens) e devolve o texto da resposta. Erros de rede/HTTP/API
// voltam como erro Go com mensagem clara (para a falha ficar visível, não silenciosa).
func (c *ClaudeClient) Gerar(ctx context.Context, system string, mensagens []Mensagem, maxTokens int) (string, error) {
	if !c.Disponivel() {
		return "", fmt.Errorf("ANTHROPIC_API_KEY não configurada")
	}
	if maxTokens <= 0 {
		maxTokens = 4096
	}
	msgs := make([]reqMensagem, 0, len(mensagens))
	for _, m := range mensagens {
		role := m.Role
		if role != "assistant" {
			role = "user"
		}
		msgs = append(msgs, reqMensagem{Role: role, Content: m.Texto})
	}
	body, _ := json.Marshal(reqBody{Model: c.modelo, MaxTokens: maxTokens, System: system, Messages: msgs})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", apiVersion)
	req.Header.Set("content-type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("falha ao chamar a API da Claude: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)

	var parsed respBody
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", fmt.Errorf("resposta inválida da API da Claude (HTTP %d): %s", resp.StatusCode, trunc(string(raw), 300))
	}
	if resp.StatusCode != http.StatusOK || parsed.Error.Message != "" {
		msg := parsed.Error.Message
		if msg == "" {
			msg = trunc(string(raw), 300)
		}
		return "", fmt.Errorf("API da Claude (HTTP %d): %s", resp.StatusCode, msg)
	}
	var sb strings.Builder
	for _, b := range parsed.Content {
		if b.Type == "text" {
			sb.WriteString(b.Text)
		}
	}
	texto := strings.TrimSpace(sb.String())
	if texto == "" {
		return "", fmt.Errorf("a API da Claude não retornou texto (stop_reason=%s)", parsed.StopReason)
	}
	return texto, nil
}

func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
