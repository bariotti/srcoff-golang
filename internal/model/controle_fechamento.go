package model

// Controle de fechamento de competência (RF-132). O usuário escolhe um MODO (não há data para
// entrar na regra): nenhum escopo, todos (inclusive produtos/domínios novos) ou apenas os
// selecionados. Estando sob controle, uma competência (mês/ano) POSTERIOR a outra já
// processada só roda após liberação manual — e liberar é escolher o mês/ano.
const (
	FechamentoOff          = "off"          // sem controle
	FechamentoTodos        = "todos"        // todos os produtos/domínios (inclusive novos)
	FechamentoSelecionados = "selecionados" // apenas os escopos listados
)

// FechamentoLiberacao é o OK manual de uma competência (mês/ano) para um escopo. Produto e/ou
// Domínio vazios = "todos".
type FechamentoLiberacao struct {
	ID       int64  `json:"id"`
	Ano      int    `json:"ano"`
	Mes      int    `json:"mes"`
	Produto  string `json:"produto"`
	Dominio  string `json:"dominio"`
	CriadoEm string `json:"criado_em"`
}

// FechamentoEscopo é um produto/domínio controlado (modo = selecionados). Campo vazio = curinga.
type FechamentoEscopo struct {
	ID      int64  `json:"id"`
	Produto string `json:"produto"`
	Dominio string `json:"dominio"`
}

// FechamentoConfig é o estado completo do controle: o modo, os escopos controlados (quando
// modo = selecionados) e as competências liberadas.
type FechamentoConfig struct {
	Modo       string                `json:"modo"`
	Escopos    []FechamentoEscopo    `json:"escopos"`
	Liberacoes []FechamentoLiberacao `json:"liberacoes"`
}

// escopoAplica: um escopo (p,d) cobre a combinação (produto,dominio); campo vazio = curinga.
func escopoAplica(p, d, produto, dominio string) bool {
	if p != "" && p != produto {
		return false
	}
	if d != "" && d != dominio {
		return false
	}
	return true
}

// Controla informa se a combinação (produto, domínio) está sujeita ao controle de fechamento.
// modo=todos cobre tudo (inclusive combinações novas); modo=selecionados cobre só os escopos
// cadastrados; modo=off não controla nada.
func (fc FechamentoConfig) Controla(produto, dominio string) bool {
	switch fc.Modo {
	case FechamentoTodos:
		return true
	case FechamentoSelecionados:
		for _, e := range fc.Escopos {
			if escopoAplica(e.Produto, e.Dominio, produto, dominio) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

// Liberado informa se a competência (ano, mês) está liberada (OK) para a combinação.
func (fc FechamentoConfig) Liberado(ano, mes int, produto, dominio string) bool {
	for _, l := range fc.Liberacoes {
		if l.Ano == ano && l.Mes == mes && escopoAplica(l.Produto, l.Dominio, produto, dominio) {
			return true
		}
	}
	return false
}
