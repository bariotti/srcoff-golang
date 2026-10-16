package service

import (
	"context"
	"fmt"
	"sort"
	"testing"
	"time"

	"srcoff/internal/evaluator"
	"srcoff/internal/model"
	filerepo "srcoff/internal/repository/file"
)

// cenárioSaldo monta a série B1 (some em d11 → baixa 1700) + B2 filler, e roda a geração.
// Retorna todos os lançamentos/estornos persistidos (ordenados de forma canônica) e o serviço.
func cenarioSaldo(t *testing.T, comSaldo bool) ([]string, *MovimentoContabilService, *filerepo.SaldoContaRepo) {
	t.Helper()
	ctx := context.Background()
	eval := evaluator.New()
	dir := t.TempDir()
	d08, d09, d10, d11 := d("2024-01-08"), d("2024-01-09"), d("2024-01-10"), d("2024-01-11")

	posRepo := &fakePosicaoRepo{registros: []model.PosicaoCarteira{
		posD1(1, d08, "NDF", "Posição", "B1", 1000),
		posD1(2, d09, "NDF", "Posição", "B1", 1300),
		posD1(3, d10, "NDF", "Posição", "B1", 900),
		// B1 ausente em d11
		posD1(4, d08, "NDF", "Posição", "B2", 500),
		posD1(5, d09, "NDF", "Posição", "B2", 600),
		posD1(6, d10, "NDF", "Posição", "B2", 700),
		posD1(7, d11, "NDF", "Posição", "B2", 800),
	}}
	regraRepo := &fakeRegraRepo{regras: []model.RegraContabil{regraIncrementalReverte()}}
	movRepo := filerepo.NewMovimentoContabilRepo(dir)
	execRepo := filerepo.NewExecucaoRepo(dir)
	padrao := &fakePadraoLookup{padroes: []model.PadraoArquivo{
		{Produto: "NDF", Dominio: "Posição", ObrigatorioMovD1: boolPtr(false)},
	}}
	svc := NewMovimentoContabilService(posRepo, regraRepo, movRepo, eval).
		ComExecucaoRepo(execRepo).ComPadraoRepo(padrao)
	var saldoRepo *filerepo.SaldoContaRepo
	if comSaldo {
		saldoRepo = filerepo.NewSaldoContaRepo(dir)
		svc = svc.ComSaldoRepo(saldoRepo)
	}

	for _, dt := range []time.Time{d08, d09, d10, d11} {
		if _, err := svc.GerarMovimentoEscopo(ctx, dt, "NDF", "Posição"); err != nil {
			t.Fatalf("gerar %s: %v", dt.Format("2006-01-02"), err)
		}
	}

	var linhas []string
	for _, dt := range []time.Time{d08, d09, d10, d11} {
		for _, ind := range []bool{false, true} {
			ls, _ := movRepo.BuscarPorDataEIndicador(ctx, dt, ind)
			for _, l := range ls {
				linhas = append(linhas, fmt.Sprintf("%s|%s|%s|%.2f|%s/%s|rev=%t|v%d",
					dt.Format("2006-01-02"), l.Produto+"/"+l.Dominio, l.CodigoIdentificadorBoleto,
					l.ValorLancamentoContabil, l.ContaDebito, l.ContaCredito, l.IndicadorReversao, l.CodigoVersaoConteudo))
			}
		}
	}
	sort.Strings(linhas)
	return linhas, svc, saldoRepo
}

// TestSaldo_EquivalenciaComESemProjecao garante que o movimento gerado é IDÊNTICO com e sem
// a projeção de saldo — ou seja, usar a tabela de saldo na baixa não muda nenhum resultado.
func TestSaldo_EquivalenciaComESemProjecao(t *testing.T) {
	comSaldo, _, _ := cenarioSaldo(t, true)
	semSaldo, _, _ := cenarioSaldo(t, false)
	if len(comSaldo) != len(semSaldo) {
		t.Fatalf("contagem difere: com=%d sem=%d\ncom=%v\nsem=%v", len(comSaldo), len(semSaldo), comSaldo, semSaldo)
	}
	for i := range comSaldo {
		if comSaldo[i] != semSaldo[i] {
			t.Fatalf("linha %d difere:\ncom=%s\nsem=%s", i, comSaldo[i], semSaldo[i])
		}
	}
	// Confirma que a baixa de 1700 ocorreu (prova que o cenário exercita o caminho).
	achou := false
	for _, l := range comSaldo {
		if l == "2024-01-11|NDF/Posição|B1|1700.00|2001/1001|rev=true|v1" {
			achou = true
		}
	}
	if !achou {
		t.Fatalf("baixa de B1 (1700) não encontrada no resultado: %v", comSaldo)
	}
}

// TestSaldo_ProjecaoRefleteNet valida que a projeção de saldo reflete o net vigente:
// B1 zerado após a baixa; B2 (nunca some) com o net acumulado.
func TestSaldo_ProjecaoRefleteNet(t *testing.T) {
	ctx := context.Background()
	_, _, saldoRepo := cenarioSaldo(t, true)
	if saldoRepo == nil {
		t.Fatal("saldoRepo nil")
	}
	saldos, err := saldoRepo.Listar(ctx, model.SaldoFiltro{IncluiZero: true})
	if err != nil {
		t.Fatalf("listar saldos: %v", err)
	}
	porBoleto := map[string]float64{}
	for _, s := range saldos {
		porBoleto[s.CodigoIdentificadorBoleto] += s.Saldo
	}
	if v := porBoleto["B1"]; v != 0 {
		t.Fatalf("B1 deveria estar zerado após baixa, obteve %v", v)
	}
	if v := porBoleto["B2"]; v != 800 { // 500 + 100 + 100 + 100
		t.Fatalf("B2 esperado 800 (net postado), obteve %v", v)
	}
}
