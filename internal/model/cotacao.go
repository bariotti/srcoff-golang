package model

import "time"

// Direção da conversão de moeda (RF-037/RN-183).
const (
	DirecaoMultiplicar = "multiplicar"
	DirecaoDividir     = "dividir"
)

// Cotacao é a taxa de conversão entre duas moedas para uma data (RF-037/RN-183).
// Usada na geração do contábil quando a condição exige conversão (não vem da posição):
// Moeda1 = moeda base do veículo, Moeda2 = moeda do lançamento. A direção informa se o
// valor do lançamento deve ser multiplicado ou dividido pela taxa.
type Cotacao struct {
	ID      int64     `json:"id"`
	Data    time.Time `json:"data"`
	Moeda1  string    `json:"moeda1"` // moeda do veículo (base)
	Moeda2  string    `json:"moeda2"` // moeda do lançamento
	Direcao string    `json:"direcao"` // multiplicar | dividir
	Taxa    float64   `json:"taxa"`
}
