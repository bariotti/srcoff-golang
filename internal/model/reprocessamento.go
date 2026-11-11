package model

// Reprocessamento seletivo (RF-137): reprocessa o contábil de um escopo restrito (produto e/ou
// domínio e, opcionalmente, um boleto) para uma data, sem refazer o dia inteiro. Como o
// versionamento é por combinação (produto, domínio), o reprocessamento ocorre no nível do combo:
// quando um boleto é informado, resolvem-se os combos em que ele aparece na posição da data e
// apenas esses são reprocessados (reaproveitando a geração padrão, com estorno/incremental/saldo).

// ReprocessamentoCombo é o resultado do reprocessamento de uma combinação (produto, domínio).
type ReprocessamentoCombo struct {
	Produto     string `json:"produto"`
	Dominio     string `json:"dominio"`
	Bloqueado   bool   `json:"bloqueado"`
	Motivo      string `json:"motivo,omitempty"` // d1 | fechamento (quando bloqueado)
	Lancamentos int    `json:"lancamentos"`      // -1 quando não medido individualmente
}

// ReprocessamentoResultado resume um reprocessamento seletivo.
type ReprocessamentoResultado struct {
	Data            string                 `json:"data"`
	Produto         string                 `json:"produto,omitempty"`
	Dominio         string                 `json:"dominio,omitempty"`
	Boleto          string                 `json:"boleto,omitempty"`
	CombosProcessados []ReprocessamentoCombo `json:"combos_processados"`
	Bloqueios       []ProdutoDominio       `json:"bloqueios"`
	Mensagem        string                 `json:"mensagem"`
}
