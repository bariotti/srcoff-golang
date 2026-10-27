package model

// RazoneteLinha é uma linha de um razonete: um dos lados (débito ou crédito) de um
// lançamento, visto por uma conta. Spec: RF-070.
type RazoneteLinha struct {
	Data           string  `json:"data"`
	Boleto         string  `json:"boleto"`
	Historico      string  `json:"historico"` // descrição da regra
	Debito         float64 `json:"debito"`
	Credito        float64 `json:"credito"`
	Reversao       bool    `json:"reversao"`
	Moeda          string  `json:"moeda"`
	SaldoAcumulado float64 `json:"saldo_acumulado"`
}

// RazoneteConta agrupa as linhas de uma conta (por moeda) em formato de razonete (T),
// com totais e saldo. Natureza "D" (devedor) quando saldo >= 0, senão "C" (credor).
type RazoneteConta struct {
	Conta        string          `json:"conta"`
	Moeda        string          `json:"moeda"`
	Linhas       []RazoneteLinha `json:"linhas"`
	TotalDebito  float64         `json:"total_debito"`
	TotalCredito float64         `json:"total_credito"`
	Saldo        float64         `json:"saldo"` // total_debito - total_credito (em módulo, a natureza indica o lado)
	Natureza     string          `json:"natureza"`
}
