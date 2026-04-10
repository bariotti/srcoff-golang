package evaluator

import (
	"bytes"
	"errors"
	"log"
	"strings"
	"testing"
	"time"
)

func TestLogEvalError_FormatAndContext(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	log.SetFlags(0) // remove timestamp prefix for deterministic output
	defer log.SetOutput(nil)

	data := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)
	boleto := "BOLETO-001"
	expression := `valor_mtm > 0`
	err := errors.New("tipo inesperado")

	LogEvalError(data, boleto, expression, err)

	got := buf.String()
	expected := `[avaliador] data=2024-01-15 boleto=BOLETO-001 expressao="valor_mtm > 0" erro=tipo inesperado`

	if !strings.Contains(got, expected) {
		t.Errorf("log inesperado\ngot:  %q\nwant: %q", strings.TrimSpace(got), expected)
	}
}

func TestLogEvalError_DoesNotPanic(t *testing.T) {
	// Garante que LogEvalError não entra em pânico com valores zero/nil
	var buf bytes.Buffer
	log.SetOutput(&buf)
	log.SetFlags(0)
	defer log.SetOutput(nil)

	LogEvalError(time.Time{}, "", "", errors.New("erro"))
	// se chegou aqui, não entrou em pânico
}
