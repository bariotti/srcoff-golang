package model

import "testing"

func TestFechamentoControla(t *testing.T) {
	off := FechamentoConfig{Modo: FechamentoOff}
	if off.Controla("NDF", "Posicao") {
		t.Error("modo off não deveria controlar")
	}

	todos := FechamentoConfig{Modo: FechamentoTodos}
	if !todos.Controla("QUALQUER", "Coisa") {
		t.Error("modo todos deveria controlar tudo (inclusive novos)")
	}

	sel := FechamentoConfig{Modo: FechamentoSelecionados, Escopos: []FechamentoEscopo{
		{Produto: "NDF", Dominio: "Posicao"},
		{Produto: "SWAP", Dominio: ""}, // curinga de domínio
	}}
	if !sel.Controla("NDF", "Posicao") {
		t.Error("escopo exato deveria controlar")
	}
	if !sel.Controla("SWAP", "QualquerDominio") {
		t.Error("escopo com domínio curinga deveria controlar")
	}
	if sel.Controla("OUTRO", "Posicao") {
		t.Error("escopo fora da lista não deveria controlar")
	}
}

func TestFechamentoLiberado(t *testing.T) {
	cfg := FechamentoConfig{
		Modo: FechamentoTodos,
		Liberacoes: []FechamentoLiberacao{
			{Ano: 2026, Mes: 11, Produto: "", Dominio: ""},        // libera nov/2026 para todos
			{Ano: 2026, Mes: 12, Produto: "NDF", Dominio: "Posicao"}, // dez/2026 só NDF/Posicao
		},
	}
	if !cfg.Liberado(2026, 11, "QUALQUER", "Coisa") {
		t.Error("nov/2026 deveria estar liberado para todos")
	}
	if !cfg.Liberado(2026, 12, "NDF", "Posicao") {
		t.Error("dez/2026 deveria estar liberado para NDF/Posicao")
	}
	if cfg.Liberado(2026, 12, "SWAP", "Posicao") {
		t.Error("dez/2026 não deveria estar liberado para SWAP")
	}
	if cfg.Liberado(2026, 10, "NDF", "Posicao") {
		t.Error("out/2026 não foi liberado")
	}
}
