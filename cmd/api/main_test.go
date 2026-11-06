package main

import (
	"testing"
	"time"
)

// TestDuracaoAteProximoHorario valida o cálculo do próximo HH:MM (RF-126, geração agendada).
func TestDuracaoAteProximoHorario(t *testing.T) {
	loc := time.UTC
	agora := time.Date(2026, 1, 1, 10, 0, 0, 0, loc) // 10:00

	// Inválidos/ vazio → desativado.
	for _, h := range []string{"", "   ", "abc", "25:00", "10:99", "10"} {
		if _, ok := duracaoAteProximoHorario(h, agora); ok {
			t.Errorf("horário %q deveria ser inválido (ok=false)", h)
		}
	}

	// Mais tarde no mesmo dia → hoje.
	if d, ok := duracaoAteProximoHorario("15:30", agora); !ok || d != 5*time.Hour+30*time.Minute {
		t.Errorf("15:30: esperava 5h30 hoje, obteve %v ok=%v", d, ok)
	}
	// Horário já passado → amanhã.
	if d, ok := duracaoAteProximoHorario("03:00", agora); !ok || d != 17*time.Hour {
		t.Errorf("03:00: esperava 17h (amanhã), obteve %v ok=%v", d, ok)
	}
	// Exatamente agora → próximo é amanhã (evita disparo imediato em loop).
	if d, ok := duracaoAteProximoHorario("10:00", agora); !ok || d != 24*time.Hour {
		t.Errorf("10:00: esperava 24h (amanhã), obteve %v ok=%v", d, ok)
	}
}
