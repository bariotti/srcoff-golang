package handler

import (
	"strings"
	"testing"

	"srcoff/internal/model"
)

// TestInferirCelulaBoletoTexto garante que a coluna do boleto é mantida como texto,
// preservando zeros à esquerda e a precisão de identificadores longos, enquanto as
// demais colunas seguem a inferência automática (número).
func TestInferirCelulaBoletoTexto(t *testing.T) {
	cfg := parseConfig{colunaBoleto: "codigo_identificador_boleto"}

	// Boleto com zeros à esquerda → texto literal (sem virar número).
	if got := inferirCelula("codigo_identificador_boleto", "0001234", cfg); got != "0001234" {
		t.Fatalf("boleto com zeros à esquerda deveria ser texto '0001234', obteve %#v", got)
	}
	// Boleto longo (acima da precisão exata do float64) → texto literal.
	long := "12345678901234567"
	if got := inferirCelula("codigo_identificador_boleto", long, cfg); got != long {
		t.Fatalf("boleto longo deveria ser texto %q, obteve %#v", long, got)
	}
	// Célula vazia na coluna do boleto → nil.
	if got := inferirCelula("codigo_identificador_boleto", "", cfg); got != nil {
		t.Fatalf("boleto vazio deveria ser nil, obteve %#v", got)
	}
	// Outra coluna numérica continua virando número.
	if got := inferirCelula("valor_mtm", "1234", cfg); got != float64(1234) {
		t.Fatalf("coluna comum deveria virar número 1234, obteve %#v", got)
	}
}

// TestConfigDoPadraoColunaBoleto verifica que o nome da coluna do boleto é normalizado
// (snake_case) para casar com o cabeçalho já normalizado do arquivo.
func TestConfigDoPadraoColunaBoleto(t *testing.T) {
	cfg := configDoPadrao(model.PadraoArquivo{ColunaBoleto: "Código Identificador Boleto"})
	if cfg.colunaBoleto != "codigo_identificador_boleto" {
		t.Fatalf("coluna do boleto deveria ser normalizada, obteve %q", cfg.colunaBoleto)
	}
}

// TestParseCSVBoletoTexto exercita o parse completo de um CSV garantindo que a coluna
// do boleto é persistida como texto.
func TestParseCSVBoletoTexto(t *testing.T) {
	csv := "codigo_identificador_boleto;valor_mtm\n0009876;1500,50\n"
	cfg := parseConfig{delimitador: ';', sepDecimal: ",", colunaBoleto: "codigo_identificador_boleto"}
	registros, _, err := parseCSV(strings.NewReader(csv), cfg)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(registros) != 1 {
		t.Fatalf("esperado 1 registro, obteve %d", len(registros))
	}
	if b := registros[0]["codigo_identificador_boleto"]; b != "0009876" {
		t.Fatalf("boleto deveria ser texto '0009876', obteve %#v", b)
	}
	if v := registros[0]["valor_mtm"]; v != float64(1500.50) {
		t.Fatalf("valor deveria ser número 1500.5, obteve %#v", v)
	}
}
