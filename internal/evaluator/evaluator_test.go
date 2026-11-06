package evaluator

import (
	"bytes"
	"errors"
	"log"
	"strings"
	"testing"
	"time"
)

func TestNormalizarExpressao(t *testing.T) {
	casos := []struct{ in, want string }{
		// Identificadores com maiúsculas/acento são normalizados para casar com a coluna da posição.
		{"VALOR_MTM > 0", "valor_mtm > 0"},
		{"posição == \"X\"", "posicao == \"X\""},
		// Literais de texto NÃO são tocados, mesmo com acento/maiúscula dentro.
		{"tipo == \"Valor ALTO\"", "tipo == \"Valor ALTO\""},
		// Nome de função (callee) preservado; só o argumento-coluna é normalizado.
		{"len(Lista) > 0", "len(lista) > 0"},
		// Já normalizada → devolvida intacta (sem reformatar).
		{"valor_mtm > 0 and tipo == \"x\"", "valor_mtm > 0 and tipo == \"x\""},
		// Nome de coluna com ESPAÇO (caso do usuário): palavras ligadas viram um identificador.
		{"valor MTM > 0", "valor_mtm > 0"},
		{"valor mtm", "valor_mtm"},
		{"valor MTM > 0 and MOEDA == \"USD\"", "valor_mtm > 0 and moeda == \"USD\""},
		// Números não são tocados.
		{"valor_mtm * 1.5", "valor_mtm * 1.5"},
		// Vazio/espaços → vazio.
		{"   ", ""},
	}
	for _, c := range casos {
		if got := NormalizarExpressao(c.in); got != c.want {
			t.Errorf("NormalizarExpressao(%q) = %q, esperado %q", c.in, got, c.want)
		}
	}
}

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
