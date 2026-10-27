package model

import "time"

// Status possíveis de um job na fila (RF-121).
const (
	JobPendente   = "pendente"
	JobExecutando = "executando"
	JobSucesso    = "sucesso"
	JobErro       = "erro"
)

// Job é uma tarefa assíncrona enfileirada e processada por um worker em segundo plano,
// com status, contagem de tentativas (retry) e log acumulado.
type Job struct {
	ID            int64     `json:"id"`
	Tipo          string    `json:"tipo"`      // ex.: geracao_movimento
	Descricao     string    `json:"descricao"` // texto amigável para exibição
	Payload       string    `json:"payload"`   // JSON com os parâmetros do job
	Status        string    `json:"status"`
	Tentativas    int       `json:"tentativas"`
	MaxTentativas int       `json:"max_tentativas"`
	Log           string    `json:"log"`
	CriadoEm      time.Time `json:"criado_em"`
	AtualizadoEm  time.Time `json:"atualizado_em"`
}

// JobPagina é uma página da listagem da fila de jobs.
type JobPagina struct {
	Total   int   `json:"total"`
	Pagina  int   `json:"pagina"`
	Tamanho int   `json:"tamanho"`
	Jobs    []Job `json:"jobs"`
}
