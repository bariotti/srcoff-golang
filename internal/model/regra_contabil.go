package model

type RegraContabil struct {
	ID                       int64           `json:"id"`
	Descricao                string          `json:"descricao"`
	CodigoProdutoCorporativo string          `json:"codigo_produto_corporativo"`
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
	Ativo        bool   `json:"ativo"`
}
