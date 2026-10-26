package evaluator

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/ast"
	"github.com/expr-lang/expr/parser"
	"github.com/expr-lang/expr/vm"
	"srcoff/internal/model"
)

// Evaluator avalia expressões dinâmicas sobre campos de posição de carteira.
type Evaluator interface {
	EvaluateCondition(expression string, env map[string]interface{}) (bool, error)
	EvaluateValue(expression string, env map[string]interface{}) (float64, error)
}

// ExprEvaluator implementa Evaluator usando github.com/expr-lang/expr.
// Mantém um cache de programas compilados por expressão (a compilação depende só do
// texto, não do env), evitando recompilar a mesma expressão para cada posição — ganho
// grande de CPU na geração (dezenas de milhares de posições).
type ExprEvaluator struct {
	mu    sync.RWMutex
	cache map[string]*vm.Program
}

// New retorna uma nova instância de ExprEvaluator.
func New() *ExprEvaluator {
	return &ExprEvaluator{cache: map[string]*vm.Program{}}
}

// compilar devolve o programa compilado da expressão, usando/preenchendo o cache.
// A chave inclui o modo (bool x valor), pois expr.AsBool() gera programa diferente.
func (e *ExprEvaluator) compilar(expression string, asBool bool) (*vm.Program, error) {
	key := "v\x00" + expression
	if asBool {
		key = "b\x00" + expression
	}
	e.mu.RLock()
	p, ok := e.cache[key]
	e.mu.RUnlock()
	if ok {
		return p, nil
	}
	var (
		prog *vm.Program
		err  error
	)
	// Compila sem expr.Env para não fazer type-checking estático, permitindo colunas
	// dinâmicas da posição.
	if asBool {
		prog, err = expr.Compile(expression, expr.AsBool())
	} else {
		prog, err = expr.Compile(expression)
	}
	if err != nil {
		return nil, err
	}
	e.mu.Lock()
	e.cache[key] = prog
	e.mu.Unlock()
	return prog, nil
}

// EvaluateCondition compila (com cache) e executa uma expressão booleana sobre o env.
func (e *ExprEvaluator) EvaluateCondition(expression string, env map[string]interface{}) (bool, error) {
	program, err := e.compilar(expression, true)
	if err != nil {
		return false, fmt.Errorf("erro ao compilar expressão de condição %q: %w", expression, err)
	}

	result, err := expr.Run(program, sanitizeEnv(env))
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
	program, err := e.compilar(expression, false)
	if err != nil {
		return 0, fmt.Errorf("erro ao compilar expressão de valor %q: %w", expression, err)
	}

	result, err := expr.Run(program, sanitizeEnv(env))
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
// camposRefCache memoiza o resultado de CamposReferenciados por expressão (o parse não
// depende do env). Evita re-parsear a mesma expressão para cada posição.
var camposRefCache sync.Map // string -> []string

func CamposReferenciados(expression string) ([]string, error) {
	if v, ok := camposRefCache.Load(expression); ok {
		return v.([]string), nil
	}
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
	camposRefCache.Store(expression, campos)
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

// acentosNorm mapeia caracteres acentuados para o equivalente sem acento.
var acentosNorm = map[rune]rune{
	'á': 'a', 'à': 'a', 'ã': 'a', 'â': 'a', 'ä': 'a',
	'é': 'e', 'ê': 'e', 'è': 'e', 'ë': 'e',
	'í': 'i', 'î': 'i', 'ì': 'i', 'ï': 'i',
	'ó': 'o', 'õ': 'o', 'ô': 'o', 'ò': 'o', 'ö': 'o',
	'ú': 'u', 'û': 'u', 'ù': 'u', 'ü': 'u',
	'ç': 'c', 'ñ': 'n',
}

// LayoutsFormatoData mapeia o rótulo do padrão de arquivo (ex: "DD/MM/AAAA") para o layout
// de referência do Go. Fonte única usada na importação e na geração.
var LayoutsFormatoData = map[string]string{
	"AAAA/MM/DD": "2006/01/02",
	"AAAA-MM-DD": "2006-01-02",
	"DD/MM/AAAA": "02/01/2006",
	"DD-MM-AAAA": "02-01-2006",
	"AAAA/DD/MM": "2006/02/01",
	"AAAA-DD-MM": "2006-02-01",
	"MM/DD/AAAA": "01/02/2006",
	"MM-DD-AAAA": "01-02-2006",
}

// LayoutData devolve o layout Go para o rótulo de formato do padrão ("" se desconhecido).
func LayoutData(label string) string { return LayoutsFormatoData[strings.TrimSpace(label)] }

// NormalizarDataISO converte um valor de data (campo da posição) para "AAAA-MM-DD".
// Aceita time.Time direto e strings em vários formatos — ISO, o formato configurado no
// padrão do arquivo (layoutPadrao, quando informado) e o texto padrão de time.Time.
// Devolve ok=false quando o valor não é uma data reconhecível.
func NormalizarDataISO(v interface{}, layoutPadrao string) (string, bool) {
	switch t := v.(type) {
	case time.Time:
		return t.Format("2006-01-02"), true
	case nil:
		return "", false
	}
	s := strings.TrimSpace(fmt.Sprintf("%v", v))
	if s == "" {
		return "", false
	}
	// Algumas datas chegam com hora anexada (ex.: "2026-10-12 00:00:00 +0000 UTC"): usa só o 1º token.
	primeiro := s
	if i := strings.IndexByte(s, ' '); i > 0 {
		primeiro = s[:i]
	}
	layouts := []string{"2006-01-02"}
	if layoutPadrao != "" {
		layouts = append(layouts, layoutPadrao)
	}
	layouts = append(layouts, "02/01/2006", "01/02/2006", "2006/01/02", time.RFC3339, "2006-01-02T15:04:05Z07:00")
	for _, cand := range []string{primeiro, s} {
		for _, lay := range layouts {
			if ti, err := time.Parse(lay, cand); err == nil {
				return ti.Format("2006-01-02"), true
			}
		}
	}
	return "", false
}

// NormalizarNome converte um nome de coluna para snake_case sem acento. É a fonte
// única de normalização usada tanto na importação (cabeçalho da posição) quanto na
// geração (resolução da coluna do veículo), para que os nomes casem exatamente.
func NormalizarNome(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	var sb strings.Builder
	for _, r := range s {
		if sub, ok := acentosNorm[r]; ok {
			sb.WriteRune(sub)
		} else {
			sb.WriteRune(r)
		}
	}
	s = sb.String()
	var b strings.Builder
	prevUnderscore := false
	for _, r := range s {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			prevUnderscore = false
		default:
			if !prevUnderscore && b.Len() > 0 {
				b.WriteRune('_')
				prevUnderscore = true
			}
		}
	}
	return strings.Trim(b.String(), "_")
}
