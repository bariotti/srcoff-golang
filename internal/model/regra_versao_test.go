package model

import "testing"

func condRef(id int64, cond, cv string) CondicaoRegra {
	return CondicaoRegra{ID: id, Condicao: cond, ContaDebito: "1", ContaCredito: "2", CampoValor: cv, CampoMoeda: "moeda", Status: StatusAtiva, Ativo: true}
}

func TestDiffRegras(t *testing.T) {
	a := RegraContabil{
		Descricao: "R", PreCondicao: "x > 0", Status: StatusAtiva, Ativo: true,
		Condicoes: []CondicaoRegra{condRef(1, "a > 0", "valor"), condRef(2, "b > 0", "valor")},
	}
	b := RegraContabil{
		Descricao: "R", PreCondicao: "y > 0", Status: StatusAtiva, Ativo: true,
		Condicoes: []CondicaoRegra{condRef(1, "a > 0", "valor_liquido"), condRef(3, "c > 0", "valor")},
	}
	d := DiffRegras(a, b)

	if len(d.CamposRegra) != 1 || d.CamposRegra[0].Campo != "pre_condicao" {
		t.Fatalf("esperava 1 campo de regra alterado (pre_condicao), veio %+v", d.CamposRegra)
	}
	var alt, add, rem int
	for _, c := range d.Condicoes {
		switch c.Tipo {
		case DiffAlterada:
			alt++
			if c.IDCondicao != 1 {
				t.Fatalf("alterada deveria ser a condição 1, veio %d", c.IDCondicao)
			}
		case DiffAdicionada:
			add++
		case DiffRemovida:
			rem++
		}
	}
	if alt != 1 || add != 1 || rem != 1 {
		t.Fatalf("esperava 1 alterada, 1 adicionada, 1 removida; veio alt=%d add=%d rem=%d", alt, add, rem)
	}
	if d.SemAlteracoes {
		t.Fatal("não deveria ser 'sem alterações'")
	}
}

func TestDiffRegrasSemAlteracoes(t *testing.T) {
	a := RegraContabil{Descricao: "R", Status: StatusAtiva, Ativo: true, Condicoes: []CondicaoRegra{condRef(1, "a > 0", "valor")}}
	d := DiffRegras(a, a)
	if !d.SemAlteracoes {
		t.Fatalf("regras idênticas deveriam dar sem alterações, veio %+v", d)
	}
	if d.CondicoesIguais != 1 {
		t.Fatalf("esperava 1 condição igual, veio %d", d.CondicoesIguais)
	}
}

func TestResumoAlteracao(t *testing.T) {
	base := RegraContabil{Descricao: "R", Status: StatusAtiva, Ativo: true, Condicoes: []CondicaoRegra{condRef(1, "a > 0", "valor")}}

	if r := ResumoAlteracao(nil, base); r != "Regra criada" {
		t.Fatalf("nil anterior deveria ser 'Regra criada', veio %q", r)
	}

	excluida := base
	excluida.Status = StatusExcluida
	excluida.Ativo = false
	if r := ResumoAlteracao(&base, excluida); r != "Regra excluída" {
		t.Fatalf("mudança de status para excluída deveria dar 'Regra excluída', veio %q", r)
	}

	comNova := base
	comNova.Condicoes = append([]CondicaoRegra{}, base.Condicoes...)
	comNova.Condicoes = append(comNova.Condicoes, condRef(2, "b > 0", "valor"))
	if r := ResumoAlteracao(&base, comNova); r != "Condição adicionada" {
		t.Fatalf("esperava 'Condição adicionada', veio %q", r)
	}
}
