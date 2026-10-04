package model

// PadraoArquivo mapeia um padrão de nome de arquivo (glob, ex: "posicao_ndf*.csv")
// ao produto correspondente. Usado na importação em lote e no monitoramento de
// pasta para identificar o produto da posição pelo nome do arquivo.
type PadraoArquivo struct {
	ID      int64  `json:"id"`
	Padrao  string `json:"padrao"`
	Produto string `json:"produto"`
	Dominio string `json:"dominio"`
	// Parâmetros opcionais de parsing do CSV. Vazios = comportamento automático.
	Delimitador      string `json:"delimitador"`       // ";" ou "," (vazio = auto-detecta)
	SeparadorDecimal string `json:"separador_decimal"` // "," ou "." (vazio = auto)
	SeparadorMilhar  string `json:"separador_milhar"`  // "." , "," ou "" (nenhum)
	// Formato de data de TODAS as colunas de data do arquivo (obrigatório no cadastro).
	// Guarda o rótulo escolhido (ex.: "DD/MM/AAAA", "MM/DD/AAAA"). Vazio = automático
	// (mantido apenas para padrões legados cadastrados antes deste campo).
	FormatoData string `json:"formato_data"`
	// Nome da coluna do arquivo que contém a data base da posição — é ela que define a
	// data do lote (obrigatório no cadastro). Comparada após normalização (snake_case).
	// Vazio = comportamento legado (resolve pela regra/auto-detecção).
	ColunaData string `json:"coluna_data"`
	// Nome da coluna do arquivo que contém o número do boleto (obrigatório no cadastro).
	// Na importação essa coluna é SEMPRE persistida como texto, preservando zeros à
	// esquerda e a precisão de identificadores longos (sem conversão para número).
	// Comparada após normalização (snake_case). Vazio = comportamento legado.
	ColunaBoleto string `json:"coluna_boleto"`
	// ObrigatorioMovD1 indica se o contábil exige que o movimento de D-1 útil já exista
	// antes de processar (quando já houver movimento em alguma data para o produto/domínio).
	// nil = padrão (true). Quando false, não valida e o estorno usa o movimento da maior
	// data anterior disponível.
	ObrigatorioMovD1 *bool `json:"obrigatorio_mov_d1,omitempty"`
}

// ExigeMovimentoD1 retorna se a validação de obrigatoriedade do movimento de D-1 útil
// se aplica a este padrão. Ausente (nil) = true (padrão).
func (p PadraoArquivo) ExigeMovimentoD1() bool {
	return p.ObrigatorioMovD1 == nil || *p.ObrigatorioMovD1
}
