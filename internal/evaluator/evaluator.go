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
	program, err := expr.Compile(expression, expr.Env(env), expr.AsBool())
	if err != nil {
		return false, fmt.Errorf("erro ao compilar expressão de condição %q: %w", expression, err)
	}

	result, err := expr.Run(program, env)
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
	program, err := expr.Compile(expression, expr.Env(env))
	if err != nil {
		return 0, fmt.Errorf("erro ao compilar expressão de valor %q: %w", expression, err)
	}

	result, err := expr.Run(program, env)
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

// LogEvalError registra um erro de avaliação de expressão com contexto completo:
// data do lote, código identificador do boleto, expressão que falhou e mensagem de erro.
// Deve ser chamado pela camada de serviço para que o processamento do lote não seja interrompido.
func LogEvalError(data time.Time, boleto string, expression string, err error) {
	log.Printf("[avaliador] data=%s boleto=%s expressao=%q erro=%v",
		data.Format("2006-01-02"), boleto, expression, err)
}

// PosicaoToEnv converte um PosicaoCarteira para map[string]interface{} com chaves snake_case.
func PosicaoToEnv(p model.PosicaoCarteira) map[string]interface{} {
	return map[string]interface{}{
		"id":                              p.ID,
		"data_posicao_carteira":           p.DataPosicaoCarteira,
		"codigo_versao_conteudo":          p.CodigoVersaoConteudo,
		"codigo_identificador_boleto":     p.CodigoIdentificadorBoleto,
		"descricao_veiculo":               p.DescricaoVeiculo,
		"indicador_contraparte_afiliada":  p.IndicadorContraparteAfiliada,
		"valor_mtm":                       p.ValorMTM,
		"principal_remanescente":          p.PrincipalRemanescente,
		"moeda_principal_remanescente":    p.MoedaPrincipalRemanescente,
	}
}
