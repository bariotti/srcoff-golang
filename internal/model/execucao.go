package model

import "time"

// ProdutoDominio identifica a combinação Produto + Domínio.
type ProdutoDominio struct {
	Produto string `json:"produto"`
	Dominio string `json:"dominio"`
}

// MovimentoExecucao registra que o contábil foi executado para uma combinação
// (data, produto, domínio) — base da visão de processados/pendentes.
type MovimentoExecucao struct {
	ID             int64     `json:"id"`
	DataLote       time.Time `json:"data_lote"`
	Produto        string    `json:"produto"`
	Dominio        string    `json:"dominio"`
	QtdLancamentos int       `json:"qtd_lancamentos"`
	QtdEstornos    int       `json:"qtd_estornos"`
	CriadoEm       time.Time `json:"criado_em"`
}
