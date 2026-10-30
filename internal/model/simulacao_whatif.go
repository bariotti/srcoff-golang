package model

// Simulação What-if de regra candidata (RF-122). Avalia uma regra HIPOTÉTICA sobre a
// posição de carteira de uma data/escopo e devolve os lançamentos que ela GERARIA,
// sem persistir nada. Reaproveita a mesma lógica de avaliação da geração real.

// SimulacaoRegraInput é a regra candidata submetida à simulação. O escopo (produto/domínio/
// boleto) filtra as posições avaliadas e, quando a regra é materializada (criar regra real),
// o produto/domínio do escopo viram os da regra. Trata só lançamentos normais — ver
// SimulacaoWhatIf.Aviso.
type SimulacaoRegraInput struct {
	Data           string              `json:"data"`            // AAAA-MM-DD (posição a avaliar)
	Produto        string              `json:"produto"`         // escopo: filtra as posições e vira o produto da regra
	Dominio        string              `json:"dominio"`         // escopo: filtra as posições e vira o domínio da regra
	Boleto         string              `json:"boleto"`          // escopo opcional: avalia só um boleto específico
	PreCondicao    string              `json:"pre_condicao"`    // expressão opcional combinada com E a cada condição
	TipoLancamento string              `json:"tipo_lancamento"` // reverte | nao_reverte | incremental | incremental_reverte
	LimiteLinhas   int                 `json:"limite_linhas"`   // trava de linhas de detalhe retornadas (default 500, máx 5000)
	Condicoes      []SimulacaoCondicao `json:"condicoes"`
}

// SimulacaoCondicao é uma condição da regra candidata. Espelha o cadastro de condição,
// inclusive a configuração de moeda base do veículo (RF-036/RN-182).
type SimulacaoCondicao struct {
	Condicao     string `json:"condicao"`
	ContaDebito  string `json:"conta_debito"`
	ContaCredito string `json:"conta_credito"`
	CampoValor   string `json:"campo_valor"`
	CampoMoeda   string `json:"campo_moeda"`
	CampoBoleto  string `json:"campo_boleto"`
	// Moeda base do veículo (igual ao cadastro da condição): base_na_posicao=true lê
	// valor/moeda base da posição; caso contrário converte pela data de campo_data_conversao.
	BaseNaPosicao       bool   `json:"base_na_posicao"`
	CampoValorMoedaBase string `json:"campo_valor_moeda_base"`
	CampoMoedaBase      string `json:"campo_moeda_base"`
	CampoDataConversao  string `json:"campo_data_conversao"`
}

// SimulacaoWhatIf é o resultado da simulação (nada é persistido).
type SimulacaoWhatIf struct {
	PosicoesAvaliadas    int                       `json:"posicoes_avaliadas"`    // linhas de posição lidas no escopo
	TotalLancamentos     int                       `json:"total_lancamentos"`     // lançamentos que a regra geraria (contagem completa)
	TotalInconsistencias int                       `json:"total_inconsistencias"` // inconsistências detectadas (contagem completa)
	Truncado             bool                      `json:"truncado"`              // alguma lista de detalhe foi truncada por LimiteLinhas
	Lancamentos          []SimulacaoLancamento     `json:"lancamentos"`           // detalhe (até LimiteLinhas)
	Inconsistencias      []SimulacaoInconsistencia `json:"inconsistencias"`       // detalhe (até LimiteLinhas)
	Aviso                string                    `json:"aviso,omitempty"`       // nota sobre o escopo da simulação (ex.: incremental)
}

// SimulacaoLancamento é um lançamento que a regra candidata geraria.
type SimulacaoLancamento struct {
	Boleto                string  `json:"boleto"`
	Valor                 float64 `json:"valor"`
	Moeda                 string  `json:"moeda"`
	ValorMoedaBaseVeiculo float64 `json:"valor_moeda_base_veiculo"` // valor na moeda base do veículo (RF-036); 0 quando não configurado
	MoedaBaseVeiculo      string  `json:"moeda_base_veiculo"`       // moeda base do veículo; "" quando não configurado
	ContaDebito           string  `json:"conta_debito"`
	ContaCredito          string  `json:"conta_credito"`
	Produto               string  `json:"produto"`
	Dominio               string  `json:"dominio"`
	Condicao              string  `json:"condicao"`
}

// SimulacaoInconsistencia descreve por que uma linha não gerou lançamento.
type SimulacaoInconsistencia struct {
	Boleto    string `json:"boleto"`
	Tipo      string `json:"tipo"`
	Expressao string `json:"expressao"`
	Detalhe   string `json:"detalhe"`
}
