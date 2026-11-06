package model

// Diff de versões do movimento contábil (RF-124). Compara DUAS versões de um mesmo
// (data, produto, domínio) — lembrando que a versão é independente por combinação — e
// devolve os lançamentos adicionados (só na versão B), removidos (só na A) e alterados
// (mesma chave, valor/moeda diferente). A chave de um lançamento é
// (boleto, conta débito, conta crédito, regra, indicador de reversão); o valor é a soma
// de valor_lancamento_contabil por chave dentro da versão.

// DiffVersoesInput parametriza a comparação. VersaoA/VersaoB <= 0 fazem o serviço
// assumir, respectivamente, a versão anterior e a vigente (padrão "vigente × anterior").
type DiffVersoesInput struct {
	Data    string
	Produto string
	Dominio string
	VersaoA int
	VersaoB int
}

// DiffLinha é uma diferença de lançamento entre as duas versões.
type DiffLinha struct {
	Boleto        string  `json:"boleto"`
	ContaDebito   string  `json:"conta_debito"`
	ContaCredito  string  `json:"conta_credito"`
	IDRegra       int64   `json:"id_regra_contabil"`
	Reversao      bool    `json:"indicador_reversao"`
	DescricaoRegra string `json:"descricao_regra"`
	Moeda         string  `json:"moeda"`
	ValorA        float64 `json:"valor_a"` // valor na versão A (0 para adicionados)
	ValorB        float64 `json:"valor_b"` // valor na versão B (0 para removidos)
}

// DiffVersoes é o resultado da comparação.
type DiffVersoes struct {
	Data               string      `json:"data"`
	Produto            string      `json:"produto"`
	Dominio            string      `json:"dominio"`
	VersaoA            int         `json:"versao_a"`
	VersaoB            int         `json:"versao_b"`
	VersoesDisponiveis []int       `json:"versoes_disponiveis"`
	Adicionados        []DiffLinha `json:"adicionados"`
	Removidos          []DiffLinha `json:"removidos"`
	Alterados          []DiffLinha `json:"alterados"`
	TotAdicionados     int         `json:"tot_adicionados"`
	TotRemovidos       int         `json:"tot_removidos"`
	TotAlterados       int         `json:"tot_alterados"`
	Aviso              string      `json:"aviso,omitempty"`
}
