package handler

import (
	"bytes"
	"testing"

	"golang.org/x/text/encoding/charmap"
)

// TestParseCSV_Windows1252 garante que um CSV em Windows-1252 (Latin-1), como os exportados
// pelo Excel no Windows, tem seus cabeçalhos acentuados lidos e normalizados corretamente —
// reproduz e valida a correção do erro "coluna de data base ... não existe no arquivo".
func TestParseCSV_Windows1252(t *testing.T) {
	utf8csv := "Data Posição Carteira;Número Boleto;Valor MTM\n12/10/2026;BOL-1;-1000\n"
	lat1, err := charmap.Windows1252.NewEncoder().Bytes([]byte(utf8csv))
	if err != nil {
		t.Fatalf("encode latin-1: %v", err)
	}
	if bytes.Equal(lat1, []byte(utf8csv)) {
		t.Fatal("o conteúdo deveria diferir do UTF-8 (há acentos)")
	}

	regs, cols, err := parseCSV(bytes.NewReader(lat1), parseConfig{delimitador: ';'})
	if err != nil {
		t.Fatalf("parseCSV: %v", err)
	}
	if !colunaExiste(cols, "data_posicao_carteira") {
		t.Fatalf("esperava coluna normalizada data_posicao_carteira; obteve %v", cols)
	}
	if !colunaExiste(cols, "numero_boleto") {
		t.Fatalf("esperava coluna normalizada numero_boleto; obteve %v", cols)
	}
	if len(regs) != 1 || regs[0]["codigo_identificador_boleto"] != nil && regs[0]["numero_boleto"] == nil {
		t.Fatalf("registro inesperado: %+v", regs)
	}
}
