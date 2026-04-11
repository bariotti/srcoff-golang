package evaluator

import (
	"fmt"
	"log"
	"time"

	"github.com/expr-lang/expr"
	"srcoff/internal/model"
)

// Evaluator avalia expressões dinâmicas sobre campos de posição de carteira.
type Evaluator interface {
	EvaluateCondition(expression string, env map[string]interface{}) (bool, error)
	EvaluateValue(expression string, env map[string]interface{}) (float64, error)
}

// ExprEvaluator implementa Evaluator usando github.com/expr-lang/expr.
type ExprEvaluator struct{}

// New retorna uma nova instância de ExprEvaluator.
func New() *ExprEvaluator {
	return &ExprEvaluator{}
}

// EvaluateCondition compila e executa uma expressão booleana sobre o env fornecido.
func (e *ExprEvaluator) EvaluateCondition(expression string, env map[string]interface{}) (bool, error) {
	safeEnv := sanitizeEnv(env)
	// Compila sem expr.Env para não fazer type-checking estático,
	// permitindo que colunas dinâmicas da posição sejam usadas sem recompilação.
	program, err := expr.Compile(expression, expr.AsBool())
	if err != nil {
		return false, fmt.Errorf("erro ao compilar expressão de condição %q: %w", expression, err)
	}

	result, err := expr.Run(program, safeEnv)
	if err != nil {
		return false, fmt.Errorf("erro ao avaliar expressão de condição %q: %w", expression, err)
	}

	v, ok := result.(bool)
	if !ok {
		return false, fmt.Errorf("expressão de condição %q não retornou bool (retornou %T)", expression, result)
	}

	return v, nil
}

// EvaluateValue compila e executa uma expressão aritmética sobre o env fornecido,
// retornando o resultado como float64. Suporta resultados int e float64.
func (e *ExprEvaluator) EvaluateValue(expression string, env map[string]interface{}) (float64, error) {
	safeEnv := sanitizeEnv(env)
	program, err := expr.Compile(expression)
	if err != nil {
		return 0, fmt.Errorf("erro ao compilar expressão de valor %q: %w", expression, err)
	}

	result, err := expr.Run(program, safeEnv)
	if err != nil {
		return 0, fmt.Errorf("erro ao avaliar expressão de valor %q: %w", expression, err)
	}

	switch v := result.(type) {
	case float64:
		return v, nil
	case float32:
		return float64(v), nil
	case int:
		return float64(v), nil
	case int64:
		return float64(v), nil
	case int32:
		return float64(v), nil
	default:
		return 0, fmt.Errorf("expressão de valor %q retornou tipo inesperado %T", expression, result)
	}
}

// sanitizeEnv substitui valores nil por zero-values tipados (float64=0, string="", bool=false)
// para evitar erros de tipo no avaliador de expressões quando colunas têm valor NULL no banco.
func sanitizeEnv(env map[string]interface{}) map[string]interface{} {
	safe := make(map[string]interface{}, len(env))
	for k, v := range env {
		if v == nil {
			safe[k] = float64(0) // default numérico para colunas NULL
		} else {
			safe[k] = v
		}
	}
	return safe
}

// LogEvalError registra um erro de avaliação de expressão com contexto completo:
// data do lote, código identificador do boleto, expressão que falhou e mensagem de erro.
// Deve ser chamado pela camada de serviço para que o processamento do lote não seja interrompido.
func LogEvalError(data time.Time, boleto string, expression string, err error) {
	log.Printf("[avaliador] data=%s boleto=%s expressao=%q erro=%v",
		data.Format("2006-01-02"), boleto, expression, err)
}

// PosicaoToEnv retorna o mapa de campos da posição diretamente para o avaliador.
// O mapa é construído dinamicamente pelo repositório a partir de SELECT *,
// portanto qualquer coluna presente na tabela posicao_carteira fica disponível
// nas expressões das regras sem necessidade de alteração de código.
func PosicaoToEnv(p model.PosicaoCarteira) map[string]interface{} {
	return p.Campos
}
