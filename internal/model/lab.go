package model

// [LAB] Features experimentais (Diagnóstico de dupla contabilização + Linhagem).
// Para remover tudo: apague model/lab.go, service/lab_service.go, handler/lab_handler.go,
// as linhas marcadas "[LAB]" em cmd/api/main.go e a seção "[LAB]" em internal/webui/app.html.

// DiagnosticoRegras é o resultado do diagnóstico DATA-DRIVEN das regras ativas contra a
// posição de uma data. Diferente do Lint estático (RF-129), que compara expressões
// textualmente, aqui as regras são efetivamente AVALIADAS sobre as posições reais — por
// isso detecta sobreposição entre regras de expressões DIFERENTES que, ainda assim,
// disparam no MESMO boleto (risco de lançamento em dobro).
type DiagnosticoRegras struct {
	Data               string             `json:"data"`
	TotalPosicoes      int                `json:"total_posicoes"`
	TotalBoletos       int                `json:"total_boletos"`
	Conflitos          []ConflitoBoleto   `json:"conflitos"`
	SemCorrespondencia []RegraCondRef     `json:"sem_correspondencia"`
	CamposAusentes     []CampoAusenteItem `json:"campos_ausentes"`
	Truncado           bool               `json:"truncado"`
}

// ConflitoBoleto é um boleto atingido por mais de uma REGRA distinta na mesma data.
type ConflitoBoleto struct {
	Boleto  string         `json:"boleto"`
	Produto string         `json:"produto"`
	Dominio string         `json:"dominio"`
	Regras  []RegraCondRef `json:"regras"`
}

// RegraCondRef referencia a (regra, condição) que disparou, com as contas do lançamento.
type RegraCondRef struct {
	IDRegra      int64  `json:"id_regra"`
	Regra        string `json:"regra"`
	Condicao     string `json:"condicao"`
	ContaDebito  string `json:"conta_debito"`
	ContaCredito string `json:"conta_credito"`
}

// CampoAusenteItem agrupa os campos que uma condição referencia mas não existem na posição.
type CampoAusenteItem struct {
	IDRegra  int64    `json:"id_regra"`
	Regra    string   `json:"regra"`
	Condicao string   `json:"condicao"`
	Campos   []string `json:"campos"`
}

// LinhagemInput identifica o lançamento cuja linhagem ("por que esse lançamento?") se quer
// reconstruir. Vem da própria linha da consulta de movimentos (já tem todos os campos).
type LinhagemInput struct {
	Data              string `json:"data"`
	Boleto            string `json:"boleto"`
	IDRegra           int64  `json:"id_regra"`
	DescricaoCondicao string `json:"descricao_condicao"` // expressão da condição armazenada no movimento
	Produto           string `json:"produto"`
	Dominio           string `json:"dominio"`
}

// Linhagem é a explicação estruturada de um lançamento: a regra/condição que o originou e
// os campos da posição que casaram, reavaliados sobre a posição vigente da data.
type Linhagem struct {
	Encontrada     bool            `json:"encontrada"`
	Observacao     string          `json:"observacao"`
	Boleto         string          `json:"boleto"`
	Data           string          `json:"data"`
	Versao         int             `json:"versao"`
	Produto        string          `json:"produto"`
	Dominio        string          `json:"dominio"`
	IDRegra        int64           `json:"id_regra"`
	Regra          string          `json:"regra"`
	TipoLancamento string          `json:"tipo_lancamento"`
	PreCondicao    string          `json:"pre_condicao"`
	PreCondicaoOK  bool            `json:"pre_condicao_ok"`
	Condicao       string          `json:"condicao"`
	CondicaoOK     bool            `json:"condicao_ok"`
	ContaDebito    string          `json:"conta_debito"`
	ContaCredito   string          `json:"conta_credito"`
	CampoValor     string          `json:"campo_valor"`
	Valor          float64         `json:"valor"`
	CampoMoeda     string          `json:"campo_moeda"`
	Moeda          string          `json:"moeda"`
	Campos         []LinhagemCampo `json:"campos"`
}

// LinhagemCampo é um campo da posição referenciado pela regra, com o valor que tinha.
type LinhagemCampo struct {
	Nome       string `json:"nome"`
	Valor      string `json:"valor"`
	Ausente    bool   `json:"ausente"`
	NaCondicao bool   `json:"na_condicao"`
}
