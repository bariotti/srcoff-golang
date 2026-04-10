package model

type RegraContabil struct {
	ID                       int64
	Descricao                string
	CodigoProdutoCorporativo string
	Ativo                    bool
	Condicoes                []CondicaoRegra
}

type CondicaoRegra struct {
	ID           int64
	IDRegra      int64
	Condicao     string
	ContaDebito  string
	ContaCredito string
	CampoValor   string
	CampoMoeda   string
	Ativo        bool
}
