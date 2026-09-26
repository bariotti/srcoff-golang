package evaluator

import (
	"fmt"
	"log"
	"time"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/ast"
	"github.com/expr-lang/expr/parser"
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
	case nil:
		return 0, nil // campo NULL tratado como zero
	default:
		return 0, fmt.Errorf("expressão de valor %q retornou tipo inesperado %T", expression, result)
	}
}

// sanitizeEnv produz uma cópia do env pronta para o avaliador, sem mutar o mapa
// original. Converte time.Time para string YYYY-MM-DD e substitui valores nil por
// zero-values numéricos (0), evitando erros de tipo quando um campo é NULL/ausente.
func sanitizeEnv(env map[string]interface{}) map[string]interface{} {
	safe := make(map[string]interface{}, len(env))
	for k, v := range env {
		switch t := v.(type) {
		case time.Time:
			safe[k] = t.Format("2006-01-02")
		case *time.Time:
			if t != nil {
				safe[k] = t.Format("2006-01-02")
			} else {
				safe[k] = float64(0)
			}
		case nil:
			safe[k] = float64(0)
		default:
			safe[k] = v
		}
	}
	return safe
}

// visitorFunc adapta uma função ao ast.Visitor do expr-lang.
type visitorFunc func(*ast.Node)

func (f visitorFunc) Visit(n *ast.Node) { f(n) }

// CamposReferenciados retorna os nomes de campos (identificadores) usados em uma
// expressão. Nomes de funções (callees) e literais booleanos/nil não são campos e
// portanto não são retornados. Usado para detectar expressões que referenciam
// colunas ausentes na posição.
func CamposReferenciados(expression string) ([]string, error) {
	tree, err := parser.Parse(expression)
	if err != nil {
		return nil, err
	}
	identificadores := map[string]bool{}
	callees := map[string]bool{}
	v := visitorFunc(func(n *ast.Node) {
		switch node := (*n).(type) {
		case *ast.IdentifierNode:
			identificadores[node.Value] = true
		case *ast.CallNode:
			if id, ok := node.Callee.(*ast.IdentifierNode); ok {
				callees[id.Value] = true
			}
		}
	})
	ast.Walk(&tree.Node, v)

	var campos []string
	for nome := range identificadores {
		if callees[nome] {
			continue // é nome de função, não de campo
		}
		campos = append(campos, nome)
	}
	return campos, nil
}

// LogEvalError registra um erro de avaliação de expressão com contexto completo:
// data do lote, código identificador do boleto, expressão que falhou e mensagem de erro.
// Deve ser chamado pela camada de serviço para que o processamento do lote não seja interrompido.
func LogEvalError(data time.Time, boleto string, expression string, err error) {
	log.Printf("[avaliador] data=%s boleto=%s expressao=%q erro=%v",
		data.Format("2006-01-02"), boleto, expression, err)
}

// PosicaoToEnv retorna o mapa de campos da posição para o avaliador.
// O mapa é construído dinamicamente pelo repositório (upload ou SELECT *),
// portanto qualquer campo presente na posição fica disponível nas expressões
// das regras sem necessidade de alteração de código. Os metadados do lote
// (id, data e versão) também são expostos como campos utilizáveis.
func PosicaoToEnv(p model.PosicaoCarteira) map[string]interface{} {
	env := make(map[string]interface{}, len(p.Campos)+3)
	for k, v := range p.Campos {
		env[k] = v
	}
	if _, ok := env["id"]; !ok {
		env["id"] = float64(p.ID)
	}
	if _, ok := env["codigo_versao_conteudo"]; !ok {
		env["codigo_versao_conteudo"] = float64(p.CodigoVersaoConteudo)
	}
	if _, ok := env["data_posicao_carteira"]; !ok && !p.DataPosicaoCarteira.IsZero() {
		env["data_posicao_carteira"] = p.DataPosicaoCarteira
	}
	return env
}

// CampoString extrai o valor de um campo do env como string. Retorna "" se o
// campo não existir ou não for representável como texto simples.
func CampoString(env map[string]interface{}, campo string) string {
	if campo == "" {
		return ""
	}
	v, ok := env[campo]
	if !ok || v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return fmt.Sprintf("%v", t)
	case int64:
		return fmt.Sprintf("%d", t)
	case int:
		return fmt.Sprintf("%d", t)
	case bool:
		return fmt.Sprintf("%v", t)
	default:
		return fmt.Sprintf("%v", t)
	}
}
