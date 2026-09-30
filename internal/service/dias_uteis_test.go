package service

import (
	"testing"
	"time"
)

func d(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

func TestEhDiaUtil(t *testing.T) {
	casos := []struct {
		data string
		util bool
		nota string
	}{
		{"2026-01-05", true, "segunda-feira"},
		{"2026-01-02", true, "sexta-feira"},
		{"2026-01-03", false, "sábado"},
		{"2026-01-04", false, "domingo"},
		{"2026-12-25", false, "Natal (sexta)"},
		{"2026-01-01", false, "Ano Novo (quinta)"},
		{"2025-12-25", false, "Natal (quinta)"},
	}
	for _, c := range casos {
		if got := EhDiaUtil(d(c.data)); got != c.util {
			t.Errorf("EhDiaUtil(%s) [%s] = %v; esperado %v", c.data, c.nota, got, c.util)
		}
	}
}

func TestDiaUtilAnterior(t *testing.T) {
	casos := []struct {
		data      string
		esperado  string
		nota      string
	}{
		{"2026-01-05", "2026-01-02", "segunda → sexta (pula fim de semana)"},
		{"2026-01-02", "2025-12-31", "sexta → quarta (pula Ano Novo e feriado? só Ano Novo 01/01 e quinta 01/01 é feriado)"},
		{"2026-12-28", "2026-12-24", "segunda → quinta (pula fim de semana e Natal 25/12)"},
	}
	for _, c := range casos {
		if got := DiaUtilAnterior(d(c.data)).Format("2006-01-02"); got != c.esperado {
			t.Errorf("DiaUtilAnterior(%s) [%s] = %s; esperado %s", c.data, c.nota, got, c.esperado)
		}
	}
}
