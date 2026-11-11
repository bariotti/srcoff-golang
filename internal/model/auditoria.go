package model

// Trilha de auditoria global (RF-136): log unificado e ESTRUTURADO (JSON) de todas as ações
// relevantes do sistema — cadastro de regras/condições, geração do contábil, fechamento,
// liberação de competência e conciliação. Cada evento carrega o quê/quando/origem e um bloco
// `dados` com campos estruturados. Persistência sqlite-first; a linha de log JSON é sempre
// emitida (mesmo sem backend de consulta).

// Categorias de evento de auditoria.
const (
	AudCadastro    = "cadastro"
	AudGeracao     = "geracao"
	AudFechamento  = "fechamento"
	AudLiberacao   = "liberacao"
	AudConciliacao = "conciliacao"
)

// EventoAuditoria é um registro da trilha. `Dados` guarda os campos estruturados do evento
// (serializados em JSON na persistência e na linha de log).
type EventoAuditoria struct {
	ID        int64                  `json:"id"`
	CriadoEm  string                 `json:"criado_em"` // timestamp ISO
	Categoria string                 `json:"categoria"` // cadastro | geracao | fechamento | liberacao | conciliacao
	Acao      string                 `json:"acao"`      // ex.: regra.criada, movimento.gerado, competencia.liberada
	Entidade  string                 `json:"entidade"`  // alvo (ex.: "regra #12", "NDF/Posição 2024-01-08")
	Origem    string                 `json:"origem"`    // sistema | importacao | usuario | ...
	Detalhe   string                 `json:"detalhe"`   // descrição legível
	Dados     map[string]interface{} `json:"dados,omitempty"`
}

// FiltroAuditoria parametriza a consulta à trilha.
type FiltroAuditoria struct {
	Categoria string // vazio = todas
	Origem    string // vazio = todas
	De        string // YYYY-MM-DD (inclusive); vazio = sem limite inferior
	Ate       string // YYYY-MM-DD (inclusive); vazio = sem limite superior
	Texto     string // busca (case-insensitive) em acao/entidade/detalhe
	Pagina    int
	Tamanho   int
}

// PaginaAuditoria é uma página de eventos (mais recente primeiro).
type PaginaAuditoria struct {
	Total   int               `json:"total"`
	Pagina  int               `json:"pagina"`
	Tamanho int               `json:"tamanho"`
	Eventos []EventoAuditoria `json:"eventos"`
}
