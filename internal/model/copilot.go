package model

// Copiloto conversacional (RF-138): um chat que responde sobre o sistema e, com confirmação,
// executa ações (criar regra, reprocessar, liberar competência) via a API da Claude. Habilitado
// somente quando ANTHROPIC_API_KEY está parametrizada.

// CopilotMensagem é uma mensagem da conversa trocada com o assistente.
type CopilotMensagem struct {
	Role  string `json:"role"` // "user" | "assistant"
	Texto string `json:"texto"`
}

// CopilotResposta é o retorno de um turno do assistente.
//   - tipo "resposta":    resposta em texto.
//   - tipo "confirmacao": o assistente propõe uma AÇÃO (operacao+args) que muda estado e aguarda
//     o OK do usuário; a UI confirma e chama /copilot/acao.
//   - tipo "erro":        falha (ex.: chave ausente, erro da API) — exibida ao usuário.
type CopilotResposta struct {
	Tipo     string                 `json:"tipo"`
	Texto    string                 `json:"texto,omitempty"`
	Operacao string                 `json:"operacao,omitempty"`
	Args     map[string]interface{} `json:"args,omitempty"`
	Resumo   string                 `json:"resumo,omitempty"`
}
