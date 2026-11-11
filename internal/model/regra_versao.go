package model

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Histórico de versões do CADASTRO de uma regra contábil (RF-133). A unidade de versão é a
// REGRA INTEIRA (campos + todas as suas condições visíveis): qualquer alteração na regra OU em
// alguma condição gera uma nova versão DA REGRA. Assim o histórico é único por regra — uma
// mudança feita só numa condição aparece como "Condição alterada" na linha do tempo da regra.
//
// Cada versão guarda um snapshot completo (a própria RegraContabil com as condições embutidas),
// o que torna o diff e a restauração triviais e robustos a mudanças futuras de schema. É um
// recurso sqlite-first (como fechamento/jobs/expurgo): indisponível nos backends file/sqlserver.

// RegraVersao é uma versão (snapshot) do cadastro de uma regra num instante.
type RegraVersao struct {
	ID       int64         `json:"id"`
	IDRegra  int64         `json:"id_regra"`
	Versao   int           `json:"versao"`
	Resumo   string        `json:"resumo"`            // o que mudou em relação à versão anterior
	Origem   string        `json:"origem,omitempty"`  // sistema | importacao | ... (preparado para autoria futura)
	CriadoEm string        `json:"criado_em"`         // timestamp ISO
	Ativa    bool          `json:"ativa"`             // true quando este snapshot é o estado vigente da regra
	Regra    RegraContabil `json:"regra"`             // snapshot completo (campos + condições)
}

// CampoDiff descreve a alteração de um campo (de → para).
type CampoDiff struct {
	Campo  string `json:"campo"`  // chave técnica (ex.: pre_condicao)
	Rotulo string `json:"rotulo"` // rótulo amigável (ex.: Pré-condição)
	De     string `json:"de"`
	Para   string `json:"para"`
}

// Tipos de diferença de uma condição entre duas versões.
const (
	DiffAdicionada = "adicionada"
	DiffRemovida   = "removida"
	DiffAlterada   = "alterada"
)

// CondicaoDiffItem é a diferença de uma condição entre as duas versões comparadas.
type CondicaoDiffItem struct {
	IDCondicao int64       `json:"id_condicao"`
	Tipo       string      `json:"tipo"`   // adicionada | removida | alterada
	Rotulo     string      `json:"rotulo"` // identificação curta da condição (expressão/contas)
	Campos     []CampoDiff `json:"campos"` // campos alterados (preenchido quando tipo=alterada)
}

// DiffRegraVersoes é o resultado da comparação entre duas versões de uma regra.
type DiffRegraVersoes struct {
	IDRegra         int64              `json:"id_regra"`
	VersaoA         int                `json:"versao_a"`
	VersaoB         int                `json:"versao_b"`
	CamposRegra     []CampoDiff        `json:"campos_regra"`
	Condicoes       []CondicaoDiffItem `json:"condicoes"`
	CondicoesIguais int                `json:"condicoes_iguais"`
	SemAlteracoes   bool               `json:"sem_alteracoes"`
}

var rotulosRegra = []struct{ campo, rotulo string }{
	{"descricao", "Descrição"},
	{"codigo_produto_corporativo", "Produto"},
	{"dominio", "Domínio"},
	{"campo_produto", "Campo produto"},
	{"pre_condicao", "Pré-condição"},
	{"tipo_lancamento", "Tipo de lançamento"},
	{"status", "Status"},
}

func valorCampoRegra(r RegraContabil, campo string) string {
	switch campo {
	case "descricao":
		return r.Descricao
	case "codigo_produto_corporativo":
		return r.CodigoProdutoCorporativo
	case "dominio":
		return r.Dominio
	case "campo_produto":
		return r.CampoProduto
	case "pre_condicao":
		return r.PreCondicao
	case "tipo_lancamento":
		return NormalizaTipoLancamento(r.TipoLancamento)
	case "status":
		return NormalizaStatus(r.Status, r.Ativo)
	}
	return ""
}

var rotulosCondicao = []struct{ campo, rotulo string }{
	{"condicao", "Condição"},
	{"conta_debito", "Conta débito"},
	{"conta_credito", "Conta crédito"},
	{"campo_valor", "Campo valor"},
	{"campo_moeda", "Campo moeda"},
	{"campo_boleto", "Campo boleto"},
	{"status", "Status"},
	{"base_na_posicao", "Moeda base vem da posição"},
	{"campo_valor_moeda_base", "Campo valor (moeda base)"},
	{"campo_moeda_base", "Campo moeda base"},
	{"campo_data_conversao", "Campo data de conversão"},
}

func valorCampoCondicao(c CondicaoRegra, campo string) string {
	switch campo {
	case "condicao":
		return c.Condicao
	case "conta_debito":
		return c.ContaDebito
	case "conta_credito":
		return c.ContaCredito
	case "campo_valor":
		return c.CampoValor
	case "campo_moeda":
		return c.CampoMoeda
	case "campo_boleto":
		return c.CampoBoleto
	case "status":
		return NormalizaStatus(c.Status, c.Ativo)
	case "base_na_posicao":
		if c.BaseNaPosicao {
			return "sim"
		}
		return "não"
	case "campo_valor_moeda_base":
		return c.CampoValorMoedaBase
	case "campo_moeda_base":
		return c.CampoMoedaBase
	case "campo_data_conversao":
		return c.CampoDataConversao
	}
	return ""
}

func rotuloCondicao(c CondicaoRegra) string {
	cond := strings.TrimSpace(c.Condicao)
	if cond == "" {
		cond = "(sem expressão)"
	}
	return fmt.Sprintf("%s · %s / %s", cond, c.ContaDebito, c.ContaCredito)
}

// DiffRegras compara dois snapshots da MESMA regra (a = anterior, b = posterior) e devolve as
// diferenças de campos da regra e das condições (casadas por id). Condições presentes só em B
// são "adicionadas"; só em A, "removidas"; em ambas com algum campo diferente, "alteradas".
func DiffRegras(a, b RegraContabil) DiffRegraVersoes {
	d := DiffRegraVersoes{CamposRegra: []CampoDiff{}, Condicoes: []CondicaoDiffItem{}}

	for _, rc := range rotulosRegra {
		de, para := valorCampoRegra(a, rc.campo), valorCampoRegra(b, rc.campo)
		if de != para {
			d.CamposRegra = append(d.CamposRegra, CampoDiff{Campo: rc.campo, Rotulo: rc.rotulo, De: de, Para: para})
		}
	}

	ca := map[int64]CondicaoRegra{}
	for _, c := range a.Condicoes {
		ca[c.ID] = c
	}
	cb := map[int64]CondicaoRegra{}
	for _, c := range b.Condicoes {
		cb[c.ID] = c
	}

	for _, c := range b.Condicoes {
		old, existia := ca[c.ID]
		if !existia {
			d.Condicoes = append(d.Condicoes, CondicaoDiffItem{IDCondicao: c.ID, Tipo: DiffAdicionada, Rotulo: rotuloCondicao(c)})
			continue
		}
		var campos []CampoDiff
		for _, rc := range rotulosCondicao {
			de, para := valorCampoCondicao(old, rc.campo), valorCampoCondicao(c, rc.campo)
			if de != para {
				campos = append(campos, CampoDiff{Campo: rc.campo, Rotulo: rc.rotulo, De: de, Para: para})
			}
		}
		if len(campos) > 0 {
			d.Condicoes = append(d.Condicoes, CondicaoDiffItem{IDCondicao: c.ID, Tipo: DiffAlterada, Rotulo: rotuloCondicao(c), Campos: campos})
		} else {
			d.CondicoesIguais++
		}
	}
	// Removidas: estavam em A e sumiram em B.
	var removidas []CondicaoRegra
	for _, c := range a.Condicoes {
		if _, existe := cb[c.ID]; !existe {
			removidas = append(removidas, c)
		}
	}
	sort.Slice(removidas, func(i, j int) bool { return removidas[i].ID < removidas[j].ID })
	for _, c := range removidas {
		d.Condicoes = append(d.Condicoes, CondicaoDiffItem{IDCondicao: c.ID, Tipo: DiffRemovida, Rotulo: rotuloCondicao(c)})
	}

	d.SemAlteracoes = len(d.CamposRegra) == 0 && len(d.Condicoes) == 0
	return d
}

// ResumoAlteracao gera um resumo curto do que mudou de `anterior` para `atual` (anterior nil =
// primeira versão → "Regra criada"). Usado como rótulo da versão na linha do tempo.
func ResumoAlteracao(anterior *RegraContabil, atual RegraContabil) string {
	if anterior == nil {
		return "Regra criada"
	}
	d := DiffRegras(*anterior, atual)
	if d.SemAlteracoes {
		return "Sem alterações"
	}
	var partes []string
	// Campos da regra alterados (inclui status → "Regra ativada/excluída/rascunho").
	for _, c := range d.CamposRegra {
		if c.Campo == "status" {
			switch c.Para {
			case StatusAtiva:
				partes = append(partes, "Regra ativada")
			case StatusExcluida:
				partes = append(partes, "Regra excluída")
			case StatusRascunho:
				partes = append(partes, "Regra marcada como rascunho")
			default:
				partes = append(partes, "Status da regra alterado")
			}
			continue
		}
		partes = append(partes, c.Rotulo+" alterada")
	}
	// Condições.
	var add, rem, alt int
	for _, c := range d.Condicoes {
		switch c.Tipo {
		case DiffAdicionada:
			add++
		case DiffRemovida:
			rem++
		case DiffAlterada:
			alt++
		}
	}
	if add > 0 {
		partes = append(partes, plural(add, "Condição adicionada", "condições adicionadas"))
	}
	if alt > 0 {
		partes = append(partes, plural(alt, "Condição alterada", "condições alteradas"))
	}
	if rem > 0 {
		partes = append(partes, plural(rem, "Condição excluída", "condições excluídas"))
	}
	if len(partes) == 0 {
		return "Alteração"
	}
	return strings.Join(partes, "; ")
}

func plural(n int, um, varios string) string {
	if n == 1 {
		return um
	}
	return strconv.Itoa(n) + " " + varios
}
