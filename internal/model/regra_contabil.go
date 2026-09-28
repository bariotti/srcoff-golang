package model

type RegraContabil struct {
	ID                       int64           `json:"id"`
	Descricao                string          `json:"descricao"`
	CodigoProdutoCorporativo string          `json:"codigo_produto_corporativo"` // separado por vírgula: "NDF,SWAP"
	Dominio                  string          `json:"dominio"`                    // separado por vírgula: "Posição,Liquidação"
	CampoProduto             string          `json:"campo_produto"`              // nome do campo da posição comparado com o código do produto
	CampoData                string          `json:"campo_data"`                 // nome do campo da posição que contém a data (usado na importação)
	PreCondicao              string          `json:"pre_condicao"`               // expressão opcional combinada com E a cada condição da regra
	Ativo                    bool            `json:"ativo"`
	PostaReverte             bool            `json:"posta_reverte"`
	Condicoes                []CondicaoRegra `json:"condicoes"`
}

type CondicaoRegra struct {
	ID           int64  `json:"id"`
	IDRegra      int64  `json:"id_regra"`
	Condicao     string `json:"condicao"`
	ContaDebito  string `json:"conta_debito"`
	ContaCredito string `json:"conta_credito"`
	CampoValor   string `json:"campo_valor"`
	CampoMoeda   string `json:"campo_moeda"`
	CampoBoleto  string `json:"campo_boleto"` // nome do campo da posição usado como identificador do boleto no lançamento
	Ativo        bool   `json:"ativo"`
}
