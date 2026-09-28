package model

import "time"

// Tipos de notificação.
const (
	NotificacaoPosicaoImportada = "POSICAO_IMPORTADA"
	NotificacaoContabilExecutado = "CONTABIL_EXECUTADO"
)

// Notificacao representa um evento automático notificado ao usuário (importação
// automática de posição ou execução automática do contábil).
type Notificacao struct {
	ID        int64     `json:"id"`
	Tipo      string    `json:"tipo"`
	DataLote  string    `json:"data_lote"`
	Produto   string    `json:"produto"`
	Dominio   string    `json:"dominio"`
	Mensagem  string    `json:"mensagem"`
	Lida      bool      `json:"lida"`
	CriadoEm  time.Time `json:"criado_em"`
}
