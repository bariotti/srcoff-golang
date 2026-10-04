package service

import (
	"context"
	"testing"
	"time"

	"srcoff/internal/model"
	filerepo "srcoff/internal/repository/file"
)

// statusDoDia localiza o status de um dia específico no calendário.
func statusDoDia(dias []DiaCalendario, dia int) string {
	for _, d := range dias {
		if d.Dia == dia {
			return d.Status
		}
	}
	return ""
}

func fechamentoNoDia(dias []DiaCalendario, dia int) bool {
	for _, d := range dias {
		if d.Dia == dia {
			return d.FechamentoMensal
		}
	}
	return false
}

func contaFechamentos(dias []DiaCalendario) int {
	n := 0
	for _, d := range dias {
		if d.FechamentoMensal {
			n++
		}
	}
	return n
}

// registrar grava uma execução para (data, produto, domínio).
func registrar(t *testing.T, repo *filerepo.ExecucaoRepo, ds, produto, dominio string) {
	t.Helper()
	d, _ := time.Parse("2006-01-02", ds)
	if err := repo.RegistrarExecucao(context.Background(), model.MovimentoExecucao{
		DataLote: d, Produto: produto, Dominio: dominio, QtdLancamentos: 1,
	}); err != nil {
		t.Fatalf("registrar %s %s/%s: %v", ds, produto, dominio, err)
	}
}

// padroesDois retorna dois combos esperados: NDF/Posição e SWAP/Posição.
func padroesDois() *fakePadraoLookup {
	return &fakePadraoLookup{padroes: []model.PadraoArquivo{
		{Produto: "NDF", Dominio: "Posição"},
		{Produto: "SWAP", Dominio: "Posição"},
	}}
}

// TestCalendarioMes_Agregado valida a cor agregada: completo (todos os combos),
// parcial (falta algum) e nenhum (dia útil passado sem execução), além de nao_util.
func TestCalendarioMes_Agregado(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	execRepo := filerepo.NewExecucaoRepo(dir)

	// 08/01/2024: ambos os combos executados → completo.
	registrar(t, execRepo, "2024-01-08", "NDF", "Posição")
	registrar(t, execRepo, "2024-01-08", "SWAP", "Posição")
	// 09/01/2024: só um combo → parcial.
	registrar(t, execRepo, "2024-01-09", "NDF", "Posição")
	// 10/01/2024: nenhum → nenhum.

	svc := NewExecucaoService(execRepo, padroesDois())
	dias, err := svc.CalendarioMes(ctx, 2024, time.January)
	if err != nil {
		t.Fatalf("erro: %v", err)
	}
	if len(dias) != 31 {
		t.Fatalf("janeiro deveria ter 31 dias, obteve %d", len(dias))
	}
	if got := statusDoDia(dias, 1); got != "nao_util" { // Ano Novo
		t.Fatalf("01/01 deveria ser nao_util, obteve %q", got)
	}
	if got := statusDoDia(dias, 6); got != "nao_util" { // sábado
		t.Fatalf("06/01 (sáb) deveria ser nao_util, obteve %q", got)
	}
	if got := statusDoDia(dias, 8); got != "completo" {
		t.Fatalf("08/01 deveria ser completo, obteve %q", got)
	}
	if got := statusDoDia(dias, 9); got != "parcial" {
		t.Fatalf("09/01 deveria ser parcial, obteve %q", got)
	}
	if got := statusDoDia(dias, 10); got != "nenhum" {
		t.Fatalf("10/01 deveria ser nenhum, obteve %q", got)
	}
	// Fechamento mensal = último dia útil. Jan/2024 termina em 31 (quarta).
	if !fechamentoNoDia(dias, 31) {
		t.Fatalf("31/01/2024 deveria ser o fechamento mensal")
	}
	if contaFechamentos(dias) != 1 {
		t.Fatalf("deveria haver exatamente 1 fechamento mensal, obteve %d", contaFechamentos(dias))
	}
}

// TestCalendarioMes_FechamentoFimDeSemana valida que o fechamento recua para o último
// dia útil quando o mês termina em fim de semana. Ago/2025 termina no domingo (31);
// último dia útil = sexta (29).
func TestCalendarioMes_FechamentoFimDeSemana(t *testing.T) {
	ctx := context.Background()
	svc := NewExecucaoService(filerepo.NewExecucaoRepo(t.TempDir()), padroesDois())
	dias, err := svc.CalendarioMes(ctx, 2025, time.August)
	if err != nil {
		t.Fatalf("erro: %v", err)
	}
	if !fechamentoNoDia(dias, 29) {
		t.Fatalf("29/08/2025 (sexta) deveria ser o fechamento mensal")
	}
	if fechamentoNoDia(dias, 30) || fechamentoNoDia(dias, 31) {
		t.Fatalf("fim de semana (30/31) não deveria ser fechamento mensal")
	}
}

// TestCalendarioMes_Futuro valida que dias úteis futuros são "futuro" (não "nenhum").
func TestCalendarioMes_Futuro(t *testing.T) {
	ctx := context.Background()
	svc := NewExecucaoService(filerepo.NewExecucaoRepo(t.TempDir()), padroesDois())
	dias, err := svc.CalendarioMes(ctx, 2999, time.January)
	if err != nil {
		t.Fatalf("erro: %v", err)
	}
	if got := statusDoDia(dias, 1); got != "nao_util" { // Ano Novo
		t.Fatalf("01/01/2999 deveria ser nao_util, obteve %q", got)
	}
	if got := statusDoDia(dias, 2); got != "futuro" {
		t.Fatalf("02/01/2999 (dia útil futuro) deveria ser futuro, obteve %q", got)
	}
}

// TestCalendarioMes_Hoje valida que o dia corrente é marcado com Hoje=true no mês atual.
func TestCalendarioMes_Hoje(t *testing.T) {
	ctx := context.Background()
	svc := NewExecucaoService(filerepo.NewExecucaoRepo(t.TempDir()), padroesDois())
	hoje := time.Now()
	dias, err := svc.CalendarioMes(ctx, hoje.Year(), hoje.Month())
	if err != nil {
		t.Fatalf("erro: %v", err)
	}
	marcados := 0
	for _, d := range dias {
		if d.Hoje {
			marcados++
			if d.Dia != hoje.Day() {
				t.Fatalf("Hoje marcado no dia %d, esperado %d", d.Dia, hoje.Day())
			}
		}
	}
	if marcados != 1 {
		t.Fatalf("deveria haver exatamente 1 dia marcado como Hoje, obteve %d", marcados)
	}
}
