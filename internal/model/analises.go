package model

// Modelos das análises contábeis: Balancete (RF-127), Cobertura de posições (RF-128),
// Lint de regras (RF-129) e Série temporal de saldo por conta (RF-130).

// ── Balancete de verificação (RF-127) ──────────────────────────────────────────
// Σ débitos × Σ créditos por conta num período (lançamentos vigentes). Cada lançamento
// credita sua conta de crédito e debita sua conta de débito (partida dobrada); estornos já
// vêm com as contas invertidas, então entram naturalmente. Por construção Σdébitos==Σcréditos.

type BalanceteConta struct {
	Conta   string  `json:"conta"`
	Debito  float64 `json:"debito"`
	Credito float64 `json:"credito"`
	Saldo   float64 `json:"saldo"` // débito − crédito
}

type Balancete struct {
	DataInicio   string           `json:"data_inicio"`
	DataFim      string           `json:"data_fim"`
	Contas       []BalanceteConta `json:"contas"`
	TotalDebito  float64          `json:"total_debito"`
	TotalCredito float64          `json:"total_credito"`
	Diferenca    float64          `json:"diferenca"`  // total_debito − total_credito (deve ser ~0)
	Balanceado   bool             `json:"balanceado"` // |diferença| < 0,005
}

// ── Cobertura de posições (RF-128) ─────────────────────────────────────────────
// Numa data/escopo, classifica cada posição (boleto) em: coberta (≥1 regra gerou),
// órfã (nenhuma regra gerou) ou inconsistente (campo ausente/erro impediu).

type CoberturaBoleto struct {
	Boleto   string `json:"boleto"`
	Produto  string `json:"produto"`
	Dominio  string `json:"dominio"`
	Detalhe  string `json:"detalhe,omitempty"`
}

type Cobertura struct {
	Data                  string            `json:"data"`
	TotalPosicoes         int               `json:"total_posicoes"`
	Cobertos              int               `json:"cobertos"`
	Orfaos                int               `json:"orfaos"`
	Inconsistentes        int               `json:"inconsistentes"`
	BoletosOrfaos         []CoberturaBoleto `json:"boletos_orfaos"`
	BoletosInconsistentes []CoberturaBoleto `json:"boletos_inconsistentes"`
	Truncado              bool              `json:"truncado"`
}

// ── Lint de regras (RF-129) ────────────────────────────────────────────────────
// Análise estática das regras/condições ATIVAS: achados de erro/aviso.

type LintAchado struct {
	Severidade string `json:"severidade"` // "erro" | "aviso"
	Tipo       string `json:"tipo"`
	IDRegra    int64  `json:"id_regra"`
	Regra      string `json:"regra"`
	IDCondicao int64  `json:"id_condicao,omitempty"`
	Detalhe    string `json:"detalhe"`
}

type LintResultado struct {
	Achados []LintAchado `json:"achados"`
	Total   int          `json:"total"`
}

// ── Série temporal de saldo por conta (RF-130) ─────────────────────────────────
// Evolução do movimento líquido (débito−crédito) de uma conta ao longo das datas, com
// o saldo acumulado no período.

type PontoSerieSaldo struct {
	Data      string  `json:"data"`
	Debito    float64 `json:"debito"`
	Credito   float64 `json:"credito"`
	Net       float64 `json:"net"`       // débito − crédito do dia
	Acumulado float64 `json:"acumulado"` // soma dos net até a data
}

type SerieSaldo struct {
	Conta  string            `json:"conta"`
	Pontos []PontoSerieSaldo `json:"pontos"`
}

// ── Conciliação posição × movimento (RF-131) ───────────────────────────────────
// UMA execução analisa dois pontos para a data/escopo:
//  (1) Saldo × Posição: para cada linha de saldo_conta, compara o saldo projetado com o valor
//      da posição lido na COLUNA que alimenta a regra (o campo_valor da condição daquele
//      saldo, identificada pelas contas). Divergente quando saldo ≠ valor da posição.
//  (2) Na posição, sem contábil: boletos presentes na posição que uma regra ativa deveria
//      gerar (valor esperado > 0), mas sem lançamento no movimento vigente (órfãos).

// ConciliacaoSaldoLinha é uma linha da parte "Saldo × Posição".
type ConciliacaoSaldoLinha struct {
	Boleto       string  `json:"boleto"`
	Produto      string  `json:"produto"`
	Dominio      string  `json:"dominio"`
	IDRegra      int64   `json:"id_regra"`
	Regra        string  `json:"regra"`
	ContaDebito  string  `json:"conta_debito"`
	ContaCredito string  `json:"conta_credito"`
	CampoValor   string  `json:"campo_valor"`
	Saldo        float64 `json:"saldo"`
	ValorPosicao float64 `json:"valor_posicao"`
	Status       string  `json:"status"` // divergente | sem_posicao
}

// ConciliacaoOrfao é um boleto presente na posição que deveria gerar, mas sem contábil.
type ConciliacaoOrfao struct {
	Boleto        string  `json:"boleto"`
	Produto       string  `json:"produto"`
	Dominio       string  `json:"dominio"`
	ValorEsperado float64 `json:"valor_esperado"`
}

// Conciliacao é o resultado ÚNICO (as duas análises juntas).
type Conciliacao struct {
	Data            string                  `json:"data"`
	SaldoDesativado bool                    `json:"saldo_desativado"` // projeção de saldo off (parte 1 indisponível)
	// (1) Saldo × Posição
	TotValorOk     int                     `json:"tot_valor_ok"`
	TotDivergentes int                     `json:"tot_divergentes"`
	TotSemPosicao  int                     `json:"tot_sem_posicao"`
	Divergencias   []ConciliacaoSaldoLinha `json:"divergencias"` // só os problemas, até o limite
	// (2) Na posição, sem contábil
	TotOrfaos int                `json:"tot_orfaos"`
	Orfaos    []ConciliacaoOrfao `json:"orfaos"` // até o limite
	Truncado  bool               `json:"truncado"`
}
