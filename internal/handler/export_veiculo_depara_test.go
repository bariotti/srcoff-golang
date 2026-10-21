package handler

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"srcoff/internal/model"
)

type fakeExportSvc struct{ lancs []model.LancamentoContabil }

func (f *fakeExportSvc) ConsultarLancamentosFiltrado(_ context.Context, _, _ time.Time, _, _ string, _ int, _ string, _, _ int) (*model.PaginaLancamentos, error) {
	return &model.PaginaLancamentos{Total: len(f.lancs), Lancamentos: f.lancs}, nil
}
func (f *fakeExportSvc) ConsultarLancamentosFiltradoEscopo(_ context.Context, _, _ time.Time, _, _, _, _ string, _ int, _ string, _, _ int) (*model.PaginaLancamentos, error) {
	return &model.PaginaLancamentos{Total: len(f.lancs), Lancamentos: f.lancs}, nil
}

type fakeDePara struct{ m map[string]string }

func (f *fakeDePara) MapaVeiculoCodigo(_ context.Context) map[string]string { return f.m }

// TestExportCSV_AplicaDeParaVeiculo valida que o CSV extrai o CÓDIGO quando há De/Para
// (case-insensitive) e o NOME quando não há. Spec: RF-035/RN-181.
func TestExportCSV_AplicaDeParaVeiculo(t *testing.T) {
	svc := &fakeExportSvc{lancs: []model.LancamentoContabil{
		{DataLoteContabil: time.Date(2024, 1, 8, 0, 0, 0, 0, time.UTC), CodigoIdentificadorBoleto: "B1", Veiculo: "NASSAU", ContaDebito: "1001", ContaCredito: "2001", ValorLancamentoContabil: 100, MoedaLancamentoContabil: "USD"},
		{DataLoteContabil: time.Date(2024, 1, 8, 0, 0, 0, 0, time.UTC), CodigoIdentificadorBoleto: "B2", Veiculo: "BAHAMAS", ContaDebito: "1001", ContaCredito: "2001", ValorLancamentoContabil: 200, MoedaLancamentoContabil: "USD"},
	}}
	// De/Para só para "nassau" (minúsculo) — deve casar com "NASSAU" do lançamento (case-insensitive).
	dp := &fakeDePara{m: map[string]string{"nassau": "001"}}
	h := NewExportHandler(svc).ComVeiculoDePara(dp)

	req := httptest.NewRequest("GET", "/api/v1/movimento-contabil/export", nil)
	rec := httptest.NewRecorder()
	h.ExportMovimentoCSV(rec, req)

	body := rec.Body.String()
	// NASSAU mapeado → código 001 na coluna do veículo; o nome NASSAU não deve aparecer.
	if !strings.Contains(body, ";001;") {
		t.Fatalf("esperava código 001 no CSV, corpo:\n%s", body)
	}
	if strings.Contains(body, "NASSAU") {
		t.Fatalf("NASSAU (nome) não deveria aparecer; esperado o código. Corpo:\n%s", body)
	}
	// BAHAMAS não mapeado → mantém o nome.
	if !strings.Contains(body, "BAHAMAS") {
		t.Fatalf("esperava BAHAMAS (nome) no CSV, corpo:\n%s", body)
	}
}
