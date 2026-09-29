package handler

import (
	"testing"

	"srcoff/internal/model"
)

// TestInferirValorCfgFormatoData garante que o formato de data configurado no padrão
// é autoritativo e resolve a ambiguidade DD/MM (Brasil) vs MM/DD (EUA).
func TestInferirValorCfgFormatoData(t *testing.T) {
	cfgUSA := parseConfig{formatoData: layoutsFormatoData["MM/DD/AAAA"]}
	cfgBR := parseConfig{formatoData: layoutsFormatoData["DD/MM/AAAA"]}

	// Mesma string, formatos diferentes → datas diferentes (normalizadas para ISO).
	if got := inferirValorCfg("04/03/2026", cfgUSA); got != "2026-04-03" {
		t.Fatalf("MM/DD/AAAA: esperado 2026-04-03, obteve %v", got)
	}
	if got := inferirValorCfg("04/03/2026", cfgBR); got != "2026-03-04" {
		t.Fatalf("DD/MM/AAAA: esperado 2026-03-04, obteve %v", got)
	}

	// Valor que não casa com o formato configurado → mantém como texto (não vira data).
	if got := inferirValorCfg("31/12/2026", cfgUSA); got != "31/12/2026" {
		t.Fatalf("valor fora do formato deveria virar texto, obteve %v", got)
	}

	// Sem formato configurado → comportamento automático (lista multi-formato) reconhece ISO.
	if got := inferirValorCfg("2026-03-04", parseConfig{}); got != "2026-03-04" {
		t.Fatalf("auto ISO: esperado 2026-03-04, obteve %v", got)
	}
}

// TestConfigDoPadraoFormatoData verifica o mapeamento rótulo → layout Go.
func TestConfigDoPadraoFormatoData(t *testing.T) {
	if cfg := configDoPadrao(model.PadraoArquivo{FormatoData: "MM/DD/AAAA"}); cfg.formatoData != "01/02/2006" {
		t.Fatalf("MM/DD/AAAA deveria mapear para 01/02/2006, obteve %q", cfg.formatoData)
	}
	if cfg := configDoPadrao(model.PadraoArquivo{FormatoData: "AAAA-DD-MM"}); cfg.formatoData != "2006-02-01" {
		t.Fatalf("AAAA-DD-MM deveria mapear para 2006-02-01, obteve %q", cfg.formatoData)
	}
	// Rótulo vazio/desconhecido → automático ("").
	if cfg := configDoPadrao(model.PadraoArquivo{FormatoData: ""}); cfg.formatoData != "" {
		t.Fatalf("formato vazio deveria mapear para automático, obteve %q", cfg.formatoData)
	}
}
