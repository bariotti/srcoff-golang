package evaluator

import (
	"testing"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
)

// Feature: srcoff-roteirizacao-contabil-offshore, Property 2: Avaliador de expressão é determinístico
//
// Valida: Requisitos 2.1, 2.2
func TestEvaluatorIsDeterministic(t *testing.T) {
	// Fixed sets of valid expressions to avoid generating syntactically invalid ones.
	conditionExprs := []string{
		"valor_mtm > 0",
		"principal_remanescente > 0",
		"indicador_contraparte_afiliada == true",
		"valor_mtm > principal_remanescente",
	}
	valueExprs := []string{
		"valor_mtm",
		"principal_remanescente",
		"valor_mtm + principal_remanescente",
		"valor_mtm * 2",
	}

	params := gopter.DefaultTestParameters()
	params.MinSuccessfulTests = 100

	properties := gopter.NewProperties(params)

	evaluator := New()

	// Property: EvaluateCondition is deterministic
	properties.Property("EvaluateCondition é determinístico", prop.ForAll(
		func(exprIdx int, valorMTM float64, principalRemanescente float64, indicadorAfiliada bool) bool {
			expression := conditionExprs[exprIdx%len(conditionExprs)]
			env := map[string]interface{}{
				"valor_mtm":                      valorMTM,
				"principal_remanescente":         principalRemanescente,
				"indicador_contraparte_afiliada": indicadorAfiliada,
			}

			result1, err1 := evaluator.EvaluateCondition(expression, env)
			result2, err2 := evaluator.EvaluateCondition(expression, env)

			// Both calls must agree: either both error or both return the same bool
			if err1 != nil && err2 != nil {
				return true
			}
			if err1 != nil || err2 != nil {
				return false
			}
			return result1 == result2
		},
		gen.IntRange(0, len(conditionExprs)-1),
		gen.Float64(),
		gen.Float64(),
		gen.Bool(),
	))

	// Property: EvaluateValue is deterministic
	properties.Property("EvaluateValue é determinístico", prop.ForAll(
		func(exprIdx int, valorMTM float64, principalRemanescente float64) bool {
			expression := valueExprs[exprIdx%len(valueExprs)]
			env := map[string]interface{}{
				"valor_mtm":              valorMTM,
				"principal_remanescente": principalRemanescente,
			}

			result1, err1 := evaluator.EvaluateValue(expression, env)
			result2, err2 := evaluator.EvaluateValue(expression, env)

			// Both calls must agree: either both error or both return the same float64
			if err1 != nil && err2 != nil {
				return true
			}
			if err1 != nil || err2 != nil {
				return false
			}
			return result1 == result2
		},
		gen.IntRange(0, len(valueExprs)-1),
		gen.Float64(),
		gen.Float64(),
	))

	properties.TestingRun(t)
}
