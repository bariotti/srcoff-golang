package model

// Dashboard é a visão consolidada apresentada no menu Dashboard (RF-120):
// volumes por dia/produto, inconsistências, saldos e câmbio para um período.
type Dashboard struct {
	Inicio string `json:"inicio"` // AAAA-MM-DD
	Fim    string `json:"fim"`

	TotalMovimento       int `json:"total_movimento"`       // lançamentos de movimento (não-estorno) no período
	TotalEstornos        int `json:"total_estornos"`        // estornos no período
	TotalInconsistencias int `json:"total_inconsistencias"` // inconsistências no período
	TotalContasComSaldo  int `json:"total_contas_com_saldo"`// contas com saldo != 0 (projeção RN-170)

	VolumesPorDia          []DashboardVolumeDia     `json:"volumes_por_dia"`
	VolumesPorProduto      []DashboardVolumeProduto `json:"volumes_por_produto"`
	InconsistenciasPorDia  []DashboardContagem      `json:"inconsistencias_por_dia"`
	InconsistenciasPorTipo []DashboardContagem      `json:"inconsistencias_por_tipo"`
	Cambio                 []DashboardCambio        `json:"cambio"`
}

// DashboardVolumeDia agrega as contagens de movimento de um dia.
type DashboardVolumeDia struct {
	Data        string `json:"data"` // AAAA-MM-DD
	Lancamentos int    `json:"lancamentos"`
	Estornos    int    `json:"estornos"`
	Movimento   int    `json:"movimento"`
}

// DashboardVolumeProduto agrega as contagens de movimento por (produto, domínio) no período.
type DashboardVolumeProduto struct {
	Produto     string `json:"produto"`
	Dominio     string `json:"dominio"`
	Lancamentos int    `json:"lancamentos"`
	Movimento   int    `json:"movimento"`
}

// DashboardContagem é um par rótulo/total genérico (por dia, por tipo, etc.).
type DashboardContagem struct {
	Rotulo string `json:"rotulo"`
	Total  int    `json:"total"`
}

// DashboardCambio é uma cotação recente exibida no painel de câmbio.
type DashboardCambio struct {
	Data    string  `json:"data"` // AAAA-MM-DD
	Moeda1  string  `json:"moeda1"`
	Moeda2  string  `json:"moeda2"`
	Direcao string  `json:"direcao"`
	Taxa    float64 `json:"taxa"`
}
