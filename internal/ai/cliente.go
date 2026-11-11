package ai

import (
	"context"
	"os"
	"strings"
)

// Cliente é o contrato de um provedor de IA do SRCOff. Implementações: Claude (Anthropic, pago),
// Gemini (Google, free tier) e Ollama (local, grátis). Permite usar IA sem custo.
type Cliente interface {
	Disponivel() bool
	Nome() string   // claude | gemini | ollama
	Modelo() string // id do modelo
	Gerar(ctx context.Context, system string, mensagens []Mensagem, maxTokens int) (string, error)
}

// NewCliente escolhe o provedor de IA. Com AI_PROVIDER definido (claude|gemini|ollama) usa o
// escolhido; sem ele, AUTO: usa o primeiro disponível (Claude → Gemini → Ollama). Assim o chat
// funciona com uma chave gratuita do Gemini ou com um modelo local no Ollama, sem pagar nada.
func NewCliente() Cliente {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("AI_PROVIDER"))) {
	case "claude", "anthropic":
		return NewClaudeClient()
	case "gemini", "google":
		return NewGeminiClient()
	case "ollama", "local":
		return NewOllamaClient()
	}
	if c := NewClaudeClient(); c.Disponivel() {
		return c
	}
	if g := NewGeminiClient(); g.Disponivel() {
		return g
	}
	if o := NewOllamaClient(); o.Disponivel() {
		return o
	}
	// Nenhum provedor disponível → devolve o Claude (desabilitado): status "indisponível".
	return NewClaudeClient()
}
