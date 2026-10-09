package handler

import (
	"encoding/json"
	"net/http"

	"github.com/expr-lang/expr"
)

// ValidarExpressaoHandler valida a sintaxe de uma expressão antes de persistir.
type ValidarExpressaoHandler struct{}

func NewValidarExpressaoHandler() *ValidarExpressaoHandler {
	return &ValidarExpressaoHandler{}
}

// Validar trata POST /api/v1/validar-expressao
// Spec: RF-025 (docs/especificacao.md §8.2, §11.6).
func (h *ValidarExpressaoHandler) Validar(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Expressao string `json:"expressao"`
		Tipo      string `json:"tipo"` // "condicao" ou "valor"
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Expressao == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"erro": "informe o campo 'expressao'"})
		return
	}

	// Validação apenas de SINTAXE (compilação). Não executamos contra um env de
	// exemplo fixo, pois a posição é dinâmica: qualquer campo importado pode ser
	// referenciado e não deve ser rejeitado como "campo desconhecido".
	var compileErr error
	if req.Tipo == "valor" {
		_, compileErr = expr.Compile(req.Expressao)
	} else {
		_, compileErr = expr.Compile(req.Expressao, expr.AsBool())
	}

	if compileErr != nil {
		sugestao := gerarSugestao(req.Expressao, compileErr.Error())
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"valido":   false,
			"erro":     compileErr.Error(),
			"sugestao": sugestao,
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"valido": true,
	})
}

// gerarSugestao analisa o erro e sugere a correção.
func gerarSugestao(expr, errMsg string) string {
	sugestoes := map[string]string{
		"<>":        "Use '!=' em vez de '<>' (ex: valor_mtm != 0)",
		"AND":       "Use '&&' em vez de 'AND' (ex: valor_mtm > 0 && descricao_veiculo == \"NASSAU\")",
		"OR":        "Use '||' em vez de 'OR'",
		"NOT":       "Use '!' em vez de 'NOT'",
		"=":         "Use '==' para comparação (ex: descricao_veiculo == \"NASSAU\")",
		"AsBool":    "A expressão deve retornar verdadeiro ou falso (ex: valor_mtm > 0)",
		"undefined": "Verifique o nome do campo — use snake_case (ex: valor_mtm, principal_remanescente)",
		"string":    "Strings devem estar entre aspas duplas (ex: descricao_veiculo == \"NASSAU\")",
	}

	for chave, sugestao := range sugestoes {
		if containsIgnoreCase(errMsg, chave) || containsIgnoreCase(expr, chave) {
			return sugestao
		}
	}

	return "Verifique a sintaxe. Exemplos válidos:\n• Condição: descricao_veiculo == \"NASSAU\" && valor_mtm > 0\n• Valor: principal_remanescente + valor_mtm"
}

func containsIgnoreCase(s, sub string) bool {
	return len(s) >= len(sub) && func() bool {
		sl := []byte(s)
		subl := []byte(sub)
		for i := 0; i <= len(sl)-len(subl); i++ {
			match := true
			for j := range subl {
				a, b := sl[i+j], subl[j]
				if a >= 'A' && a <= 'Z' {
					a += 32
				}
				if b >= 'A' && b <= 'Z' {
					b += 32
				}
				if a != b {
					match = false
					break
				}
			}
			if match {
				return true
			}
		}
		return false
	}()
}
