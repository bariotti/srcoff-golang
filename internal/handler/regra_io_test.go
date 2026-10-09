package handler

import (
	"strings"
	"testing"

	"srcoff/internal/model"
)

// TestParseRegrasCSV valida o parse do CSV de regras/condições: desduplicação de regras
// por id, leitura das condições e linha de regra sem condição.
func TestParseRegrasCSV(t *testing.T) {
	csv := strings.Join([]string{
		"regra_id;descricao;codigo_produto_corporativo;dominio;campo_produto;pre_condicao;regra_ativa;tipo_lancamento;condicao_id;condicao;conta_debito;conta_credito;campo_valor;campo_moeda;campo_boleto;condicao_ativa",
		"1;Regra A;NDF;Posição;produto;;Sim;reverte;10;valor_mtm > 0;1001;2001;valor_mtm;moeda;codigo_identificador_boleto;Sim",
		"1;Regra A;NDF;Posição;produto;;Sim;reverte;11;valor_mtm < 0;3001;4001;valor_mtm;moeda;codigo_identificador_boleto;Sim",
		"2;Regra B;SWAP;Liquidação;produto;;Não;nao_reverte;;;;;;;;",
	}, "\n") + "\n"

	imp, err := parseRegrasCSV(strings.NewReader(csv))
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(imp.updRegras) != 2 {
		t.Fatalf("esperado 2 regras existentes (desduplicadas), obteve %d", len(imp.updRegras))
	}
	if len(imp.updConds) != 2 {
		t.Fatalf("esperado 2 condições existentes, obteve %d", len(imp.updConds))
	}
	if len(imp.novas) != 0 || len(imp.novasCondExistente) != 0 {
		t.Fatalf("não deveria haver regras/condições novas: %#v / %#v", imp.novas, imp.novasCondExistente)
	}
	if imp.updRegras[0].ID != 1 || imp.updRegras[0].Descricao != "Regra A" || !imp.updRegras[0].EhReverte() || !imp.updRegras[0].Ativo {
		t.Fatalf("regra 1 mal interpretada: %#v", imp.updRegras[0])
	}
	if imp.updRegras[1].ID != 2 || imp.updRegras[1].Ativo || imp.updRegras[1].EhReverte() {
		t.Fatalf("regra 2 deveria estar inativa e sem posta/reverte: %#v", imp.updRegras[1])
	}
	if imp.updConds[0].cond.ID != 10 || imp.updConds[0].cond.IDRegra != 1 || imp.updConds[0].cond.ContaDebito != "1001" {
		t.Fatalf("condição 10 mal interpretada: %#v", imp.updConds[0].cond)
	}
}

// TestParseRegrasCSV_Criacao valida a classificação de linhas de criação: rótulo de texto
// agrupando condições de uma regra nova, regra_id vazio (regra nova isolada) e condição
// nova em regra existente (condicao_id vazio).
func TestParseRegrasCSV_Criacao(t *testing.T) {
	csv := strings.Join([]string{
		"regra_id;descricao;codigo_produto_corporativo;dominio;campo_produto;pre_condicao;regra_ativa;tipo_lancamento;condicao_id;condicao;conta_debito;conta_credito;campo_valor;campo_moeda;campo_boleto;condicao_ativa",
		// regra nova por rótulo, 2 condições
		"NOVA-1;Regra Nova;NDF;Posição;;;Sim;reverte;;v>0;1001;2001;v;m;b;",
		"NOVA-1;Regra Nova;NDF;Posição;;;Sim;reverte;;v<0;3001;4001;v;m;b;",
		// regra nova isolada (regra_id vazio)
		";Outra Nova;SWAP;Liquidação;;;Sim;nao_reverte;;x>0;5001;6001;x;m;b;",
		// condição nova em regra existente (id 7), sem condicao_id
		"7;Existente;NDF;Posição;;;Sim;reverte;;y>0;7001;8001;y;m;b;",
	}, "\n") + "\n"

	imp, err := parseRegrasCSV(strings.NewReader(csv))
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(imp.novas) != 2 {
		t.Fatalf("esperado 2 regras novas (rótulo + vazio), obteve %d", len(imp.novas))
	}
	if len(imp.novas[0].condicoes) != 2 {
		t.Fatalf("regra NOVA-1 deveria agrupar 2 condições, obteve %d", len(imp.novas[0].condicoes))
	}
	if imp.novas[0].regra.Descricao != "Regra Nova" {
		t.Fatalf("descrição da regra nova inesperada: %q", imp.novas[0].regra.Descricao)
	}
	if len(imp.novas[1].condicoes) != 1 {
		t.Fatalf("regra nova isolada deveria ter 1 condição, obteve %d", len(imp.novas[1].condicoes))
	}
	if len(imp.updRegras) != 1 || imp.updRegras[0].ID != 7 {
		t.Fatalf("esperado 1 regra existente (id 7), obteve %#v", imp.updRegras)
	}
	if len(imp.novasCondExistente) != 1 || imp.novasCondExistente[0].cond.IDRegra != 7 {
		t.Fatalf("esperado 1 condição nova na regra 7, obteve %#v", imp.novasCondExistente)
	}
	if len(imp.updConds) != 0 {
		t.Fatalf("não deveria haver condições existentes para atualizar, obteve %#v", imp.updConds)
	}
}

// TestRegraIgualCondicaoIgual valida a detecção de "sem mudança".
func TestRegraIgualCondicaoIgual(t *testing.T) {
	a := model.RegraContabil{ID: 1, Descricao: "X", Dominio: "Posição", Ativo: true, TipoLancamento: model.TipoReverte}
	b := a
	if !regraIgual(a, b) {
		t.Fatal("regras idênticas deveriam ser iguais")
	}
	b.Descricao = "Y"
	if regraIgual(a, b) {
		t.Fatal("regras com descrição diferente não deveriam ser iguais")
	}

	c1 := model.CondicaoRegra{ID: 1, Condicao: "a>0", ContaDebito: "1", ContaCredito: "2", CampoValor: "v", CampoMoeda: "m", CampoBoleto: "b", Ativo: true}
	c2 := c1
	c2.Ativo = false // Ativo não entra na comparação
	if !condicaoIgual(c1, c2) {
		t.Fatal("condições com só Ativo diferente deveriam ser consideradas iguais")
	}
	c2.ContaDebito = "99"
	if condicaoIgual(c1, c2) {
		t.Fatal("condições com conta diferente não deveriam ser iguais")
	}
}

// TestParseBoolCSV cobre os valores aceitos e o default.
func TestParseBoolCSV(t *testing.T) {
	casos := map[string]bool{"Sim": true, "sim": true, "true": true, "1": true, "Não": false, "nao": false, "0": false}
	for in, want := range casos {
		if got := parseBoolCSV(in, !want); got != want {
			t.Fatalf("parseBoolCSV(%q) = %v, esperado %v", in, got, want)
		}
	}
	// Vazio/desconhecido → default.
	if parseBoolCSV("", true) != true || parseBoolCSV("xyz", false) != false {
		t.Fatal("valor vazio/desconhecido deveria retornar o default")
	}
}
