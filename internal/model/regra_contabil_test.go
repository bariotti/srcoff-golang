package model

import "testing"

// TestNormalizaTipoLancamento garante que valores fora do padrão (maiúsculas, espaços,
// vazio ou desconhecidos) são normalizados para os tipos canônicos — evitando que um
// "Incremental" vindo da API vire "reverte" silenciosamente.
func TestNormalizaTipoLancamento(t *testing.T) {
	casos := map[string]string{
		"reverte":        TipoReverte,
		"nao_reverte":    TipoNaoReverte,
		"incremental":    TipoIncremental,
		"Incremental":    TipoIncremental, // maiúscula
		"  incremental ": TipoIncremental, // espaços
		"NAO_REVERTE":    TipoNaoReverte,
		"":               TipoReverte, // vazio = reverte
		"qualquer":       TipoReverte, // desconhecido = reverte
	}
	for entrada, esperado := range casos {
		if got := NormalizaTipoLancamento(entrada); got != esperado {
			t.Errorf("NormalizaTipoLancamento(%q) = %q, esperado %q", entrada, got, esperado)
		}
	}
}
