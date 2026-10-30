package service

import (
	"context"
	"testing"
	"time"
)

type fakeConfigRepo struct{ m map[string]string }

func (f *fakeConfigRepo) Obter(ctx context.Context, chave string) (string, error) {
	return f.m[chave], nil
}

type fakeExpurgavel struct {
	corte   time.Time
	chamado bool
	apagar  int64
}

func (f *fakeExpurgavel) ExpurgarAteData(ctx context.Context, corte time.Time) (int64, error) {
	f.corte = corte
	f.chamado = true
	return f.apagar, nil
}

// Em 29/10 com retenção de 10 dias, o corte é 19/10 (retém os últimos 10 dias,
// 20/10..29/10) e os três repositórios são expurgados com essa data.
func TestExpurgo_CorteERetencao(t *testing.T) {
	cfg := &fakeConfigRepo{m: map[string]string{ConfigRetencaoDiasMovimento: "10"}}
	pos := &fakeExpurgavel{apagar: 9}
	mov := &fakeExpurgavel{apagar: 5}
	inc := &fakeExpurgavel{apagar: 2}
	exe := &fakeExpurgavel{apagar: 1}
	svc := NewExpurgoService(cfg, pos, mov, inc, exe)
	svc.agora = func() time.Time { return time.Date(2026, 10, 29, 15, 0, 0, 0, time.UTC) }

	if err := svc.Executar(context.Background(), nil); err != nil {
		t.Fatalf("Executar: %v", err)
	}
	quero := "2026-10-19"
	for nome, f := range map[string]*fakeExpurgavel{"posicoes": pos, "movimentos": mov, "inconsistencias": inc, "execucoes": exe} {
		if !f.chamado {
			t.Errorf("%s: expurgo não foi chamado", nome)
		}
		if got := f.corte.Format("2006-01-02"); got != quero {
			t.Errorf("%s: corte esperado %s, obtido %s", nome, quero, got)
		}
	}
}

// Retenção 0/ausente desativa o expurgo: nada é apagado.
func TestExpurgo_DesativadoQuandoZero(t *testing.T) {
	for _, v := range []string{"", "0", "-3", "abc"} {
		cfg := &fakeConfigRepo{m: map[string]string{ConfigRetencaoDiasMovimento: v}}
		mov := &fakeExpurgavel{}
		svc := NewExpurgoService(cfg, &fakeExpurgavel{}, mov, &fakeExpurgavel{}, &fakeExpurgavel{})
		if _, ok := svc.DataCorte(context.Background()); ok {
			t.Errorf("valor %q: deveria estar desativado", v)
		}
		if err := svc.Executar(context.Background(), nil); err != nil {
			t.Fatalf("Executar (%q): %v", v, err)
		}
		if mov.chamado {
			t.Errorf("valor %q: não deveria ter expurgado", v)
		}
	}
}

// IntervaloHoras usa default 24 quando ausente/invalido e respeita valor valido.
func TestExpurgo_IntervaloHoras(t *testing.T) {
	casos := map[string]int{"": 24, "0": 24, "xyz": 24, "6": 6, "48": 48}
	for v, querido := range casos {
		cfg := &fakeConfigRepo{m: map[string]string{ConfigRetencaoIntervaloHoras: v}}
		svc := NewExpurgoService(cfg, &fakeExpurgavel{}, &fakeExpurgavel{}, &fakeExpurgavel{}, &fakeExpurgavel{})
		if got := svc.IntervaloHoras(context.Background()); got != querido {
			t.Errorf("intervalo %q: esperado %d, obtido %d", v, querido, got)
		}
	}
}
