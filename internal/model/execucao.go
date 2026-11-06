package model

import "time"

// ProdutoDominio identifica a combinação Produto + Domínio.
type ProdutoDominio struct {
	Produto string `json:"produto"`
	Dominio string `json:"dominio"`
	// Motivo (opcional) explica por que a combinação foi bloqueada na geração, p.ex.
	// "d1" (falta o movimento do dia útil anterior) ou "fechamento" (competência fechada,
	// aguardando liberação). Vazio quando não se aplica.
	Motivo string `json:"motivo,omitempty"`
}

// MovimentoExecucao registra que o contábil foi executado para uma combinação
// (data, produto, domínio) — base da visão de processados/pendentes.
type MovimentoExecucao struct {
	ID             int64     `json:"id"`
	DataLote       time.Time `json:"data_lote"`
	Produto        string    `json:"produto"`
	Dominio        string    `json:"dominio"`
	QtdLancamentos int       `json:"qtd_lancamentos"` // contagem VISÍVEL (sem cancelados), para exibição
	QtdEstornos    int       `json:"qtd_estornos"`
	// QtdMovimento é a contagem BRUTA de lançamentos de movimento (não-estorno) gerados —
	// usada para saber se a data realmente tem contábil, independentemente de o par
	// lançamento+estorno ficar com saldo zero na consulta (que zeraria QtdLancamentos).
	QtdMovimento int       `json:"qtd_movimento"`
	CriadoEm     time.Time `json:"criado_em"`
}
