package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"srcoff/internal/model"
)

type fakeCatalogoSvc struct{ regras []model.RegraContabil }

func (f *fakeCatalogoSvc) RegrasAtivas(context.Context) ([]model.RegraContabil, error) {
	return f.regras, nil
}

func TestCatalogoRegras(t *testing.T) {
	svc := &fakeCatalogoSvc{regras: []model.RegraContabil{
		{ID: 1, Descricao: "MTM de NDF", CodigoProdutoCorporativo: "NDF", Dominio: "Posição", PreCondicao: "tipo == 'NDF'", TipoLancamento: "reverte",
			Condicoes: []model.CondicaoRegra{{ID: 1, Condicao: "valor > 0", ContaDebito: "1.1.2", ContaCredito: "4.1.9", CampoValor: "valor", CampoMoeda: "moeda", BaseNaPosicao: true, CampoValorMoedaBase: "vb", CampoMoedaBase: "mb"}}},
		{ID: 2, Descricao: "Regra sem condicao", TipoLancamento: "nao_reverte"},
	}}
	h := NewCatalogoRegrasHandler(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/catalogo-regras", nil)
	rec := httptest.NewRecorder()
	h.Catalogo(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, esperava 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Fatalf("content-type = %q, esperava text/html", ct)
	}
	body := rec.Body.String()
	for _, want := range []string{"Catálogo de Regras", "MTM de NDF", "regra(s) ativa(s)", ">2</b>", "1.1.2", "valor &gt; 0", "Todos", "Sem condições ativas", "Não reverte"} {
		if !strings.Contains(body, want) {
			t.Fatalf("documento não contém %q", want)
		}
	}
}

func TestCatalogoRegrasMetodoInvalido(t *testing.T) {
	h := NewCatalogoRegrasHandler(&fakeCatalogoSvc{})
	rec := httptest.NewRecorder()
	h.Catalogo(rec, httptest.NewRequest(http.MethodPost, "/api/v1/catalogo-regras", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, esperava 405", rec.Code)
	}
}
