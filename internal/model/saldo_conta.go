package model

// SaldoConta é a projeção materializada do saldo líquido postado (vigente) do movimento
// contábil por chave (produto, domínio, boleto, id_regra, conta_débito, conta_crédito).
// É um CACHE derivado de movimento_contabil (fonte da verdade) — ver RN-170 (docs/especificacao.md §4.10).
type SaldoConta struct {
	Produto                   string  `json:"produto"`
	Dominio                   string  `json:"dominio"`
	CodigoIdentificadorBoleto string  `json:"codigo_identificador_boleto"`
	IDRegraContabil           int64   `json:"id_regra_contabil"`
	ContaDebito               string  `json:"conta_debito"`
	ContaCredito              string  `json:"conta_credito"`
	Saldo                     float64 `json:"saldo"`
	// Metadados da última linha que compôs o saldo (para a baixa reproduzir moeda/descrições
	// idênticas às da varredura, e para exibição). Não fazem parte da chave.
	MoedaLancamento   string `json:"moeda_lancamento"`
	DescricaoRegra    string `json:"descricao_regra"`
	DescricaoCondicao string `json:"descricao_condicao"`
}

// SaldoFiltro restringe a listagem de saldos (campos vazios/zero = sem filtro).
type SaldoFiltro struct {
	Produto      string
	Dominio      string
	Boleto       string
	IDRegra      int64
	ContaDebito  string  // conta_debito exata (usado pelo filtro de condição = par de contas)
	ContaCredito string  // conta_credito exata (usado pelo filtro de condição)
	Conta        string  // nº de conta que casa em conta_debito OU conta_credito (sem informar D/C)
	RegrasIn     []int64 // se não vazio, só chaves dessas regras (ex.: incremental_reverte)
	IncluiZero   bool    // false (default) oculta saldo == 0
}
