package model

// VeiculoDePara mapeia o NOME do veículo (como vem na posição/movimento) para um
// CÓDIGO de veículo. Usado APENAS na extração CSV do movimento contábil: quando há
// um De/Para para o nome, o CSV exporta o código; caso contrário, exporta o nome.
// A comparação do nome é case-insensitive (NASSAU = nassau = Nassau) e ignora espaços
// nas pontas. Não afeta nenhuma outra funcionalidade (nem o TXT). Spec: RF-035/RN-181.
type VeiculoDePara struct {
	ID        int64  `json:"id"`
	Veiculo   string `json:"veiculo"`    // nome do veículo (único no cadastro)
	Codigo    string `json:"codigo"`     // código correspondente
	MoedaBase string `json:"moeda_base"` // moeda base do veículo (obrigatória) — RF-036/RN-182
}
