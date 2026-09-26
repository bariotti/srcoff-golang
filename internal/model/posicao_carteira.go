package model

import "time"

// PosicaoCarteira representa um registro da tabela/arquivo de posição de carteira.
//
// O modelo é totalmente dinâmico: apenas os metadados do lote (id, data e versão)
// são campos fixos. Todos os campos de negócio (boleto, veículo, mtm, principal,
// moeda, produto e quaisquer colunas adicionais vindas de um upload) ficam em
// Campos e são referenciados por nome nas expressões das regras contábeis.
// Assim não há acoplamento entre a estrutura da posição e o código do sistema.
type PosicaoCarteira struct {
	ID                   int64                  `json:"id"`
	DataPosicaoCarteira  time.Time              `json:"data_posicao_carteira"`
	CodigoVersaoConteudo int                    `json:"codigo_versao_conteudo"`
	Campos               map[string]interface{} `json:"campos"`
}
