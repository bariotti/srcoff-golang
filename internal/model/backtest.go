package model

// Backtesting de regra candidata (RF-135): roda a avaliação da regra (como o What-if) sobre as
// últimas N datas COM posição, retrocedendo da última data importada, e devolve o impacto
// agregado (por dia e por par de contas) — para o usuário dimensionar a regra ANTES de ativá-la.
// Reaproveita a mesma avaliação da simulação (não persiste nada). Mesma ressalva do What-if para
// regras incrementais (mostra o valor bruto de campo_valor).

// BacktestInput parametriza o backtest. Regra.Data é ignorada — as datas são calculadas.
type BacktestInput struct {
	Dias  int                 `json:"dias"`  // nº de datas COM posição a avaliar (default 10, máx 60)
	Regra SimulacaoRegraInput `json:"regra"` // regra candidata (mesma do What-if)
}

// BacktestDia é o impacto agregado da regra numa data.
type BacktestDia struct {
	Data                 string  `json:"data"`
	PosicoesAvaliadas    int     `json:"posicoes_avaliadas"`
	TotalLancamentos     int     `json:"total_lancamentos"`
	TotalInconsistencias int     `json:"total_inconsistencias"`
	SomaValor            float64 `json:"soma_valor"`             // Σ valor (moeda original)
	SomaValorBase        float64 `json:"soma_valor_base_veiculo"` // Σ valor na moeda base do veículo
}

// BacktestConta agrega o impacto por par de contas (débito/crédito).
type BacktestConta struct {
	ContaDebito  string  `json:"conta_debito"`
	ContaCredito string  `json:"conta_credito"`
	Qtd          int     `json:"qtd"`
	SomaValor    float64 `json:"soma_valor"`
}

// BacktestResultado é o resultado agregado do backtest.
type BacktestResultado struct {
	DiasSolicitados      int             `json:"dias_solicitados"`
	DiasComPosicao       int             `json:"dias_com_posicao"`
	DataInicio           string          `json:"data_inicio"` // data mais antiga avaliada
	DataFim              string          `json:"data_fim"`    // data mais recente avaliada
	TotalLancamentos     int             `json:"total_lancamentos"`
	TotalInconsistencias int             `json:"total_inconsistencias"`
	SomaValor            float64         `json:"soma_valor"`
	SomaValorBase        float64         `json:"soma_valor_base_veiculo"`
	PorConta             []BacktestConta `json:"por_conta"`
	Dias                 []BacktestDia   `json:"dias"` // da mais recente para a mais antiga
	Aviso                string          `json:"aviso,omitempty"`
}
