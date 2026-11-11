package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"srcoff/internal/ai"
	"srcoff/internal/model"
)

// copilotAI é o cliente de IA (abstraído para teste). Satisfeito por ai.Cliente (Claude/Gemini/Ollama).
type copilotAI interface {
	Disponivel() bool
	Nome() string
	Modelo() string
	Gerar(ctx context.Context, system string, mensagens []ai.Mensagem, maxTokens int) (string, error)
}

// Dependências do copiloto sobre os serviços existentes (nenhum SQL cru — funciona no sqlite).
type copilotRegraDep interface {
	ListarRegras(ctx context.Context) ([]model.RegraContabil, error)
	CriarRegra(ctx context.Context, regra model.RegraContabil) (int64, error)
	CriarCondicao(ctx context.Context, c model.CondicaoRegra) (int64, error)
}
type copilotMovDep interface {
	ConsultarLancamentosFiltradoEscopo(ctx context.Context, di, df time.Time, boleto, veiculo, produto, dominio string, versao int, versaoModo string, pagina, tamanho int) (*model.PaginaLancamentos, error)
	ReprocessarSeletivo(ctx context.Context, data time.Time, produto, dominio, boleto string) (*model.ReprocessamentoResultado, error)
}
type copilotPosDep interface {
	ListarPorData(ctx context.Context, data time.Time) ([]model.PosicaoCarteira, error)
}

// CopilotService orquestra o chat: planeja com a IA, executa ferramentas de LEITURA direto e
// propõe ações de ESCRITA para confirmação do usuário (RF-138).
type CopilotService struct {
	ai    copilotAI
	regra copilotRegraDep
	mov   copilotMovDep
	pos   copilotPosDep
	fech  *FechamentoService // tipo concreto: nil quando o backend não tem fechamento (nil-check seguro)
}

func NewCopilotService(aiClient copilotAI, regra copilotRegraDep, mov copilotMovDep, pos copilotPosDep, fech *FechamentoService) *CopilotService {
	return &CopilotService{ai: aiClient, regra: regra, mov: mov, pos: pos, fech: fech}
}

func (s *CopilotService) Disponivel() bool { return s.ai != nil && s.ai.Disponivel() }
func (s *CopilotService) Modelo() string {
	if s.ai == nil {
		return ""
	}
	return s.ai.Modelo()
}
func (s *CopilotService) Provedor() string {
	if s.ai == nil {
		return ""
	}
	return s.ai.Nome()
}

const copilotSystem = `Você é o assistente do SRCOff, um sistema de roteirização contábil offshore. Ajuda o usuário a consultar dados e, com confirmação, executar ações.

Contexto do sistema:
- Posições de carteira são importadas por data; regras contábeis (com condições em expressão) geram o "movimento contábil" (lançamentos), com estorno de D-1 e versionamento por produto/domínio.
- Produto e Domínio são dimensões de escopo. Boleto é o identificador da operação.

Você responde SEMPRE com UM objeto JSON (sem markdown, sem texto fora do JSON), em um destes formatos:
1) Resposta direta ao usuário:
   {"acao_tipo":"responder","resposta":"<texto em português>"}
2) Executar uma FERRAMENTA de leitura (os resultados voltam para você decidir o próximo passo):
   {"acao_tipo":"ferramenta","ferramenta":"<nome>","args":{...}}
3) Propor uma AÇÃO que muda o estado (precisa de confirmação do usuário):
   {"acao_tipo":"confirmar","operacao":"<nome>","args":{...},"resumo":"<frase curta do que será feito>"}

Ferramentas de leitura disponíveis:
- consultar_movimento {data:"AAAA-MM-DD", produto?, dominio?, boleto?} — lançamentos contábeis vigentes da data/escopo.
- listar_regras {} — regras cadastradas (id, descrição, produto, domínio, status).
- consultar_posicao {data:"AAAA-MM-DD", boleto?} — linhas de posição da data (campos).

Ações que exigem confirmação:
- criar_regra {descricao, produto?, dominio?, pre_condicao?, tipo_lancamento?("reverte"|"nao_reverte"|"incremental"|"incremental_reverte"), condicoes:[{condicao, campo_valor, campo_moeda, conta_debito, conta_credito, campo_boleto?}]}
- reprocessar {data:"AAAA-MM-DD", produto?, dominio?, boleto?}
- liberar_competencia {ano:AAAA, mes:1-12, produto?, dominio?}

Regras de conduta:
- Use ferramentas de leitura quando precisar de dados reais antes de responder. Não invente números.
- Para qualquer ação que crie/altere/reprocessar/liberar, use "confirmar" — nunca "ferramenta".
- Datas no formato AAAA-MM-DD. Responda em português, de forma objetiva.
- Se não tiver dados suficientes e nenhuma ferramenta ajudar, responda explicando o que falta.`

// Conversar roda um turno do assistente: planeja com a IA e, em até alguns passos, executa
// ferramentas de leitura e então responde — ou propõe uma ação para confirmação.
func (s *CopilotService) Conversar(ctx context.Context, mensagens []model.CopilotMensagem) (model.CopilotResposta, error) {
	if !s.Disponivel() {
		return model.CopilotResposta{Tipo: "erro", Texto: "Assistente indisponível: configure a variável de ambiente ANTHROPIC_API_KEY."}, nil
	}
	ctxFerramentas := ""
	for passo := 0; passo < 4; passo++ {
		prompt := construirPromptCopilot(mensagens, ctxFerramentas)
		raw, err := s.ai.Gerar(ctx, copilotSystem, []ai.Mensagem{{Role: "user", Texto: prompt}}, 2048)
		if err != nil {
			return model.CopilotResposta{Tipo: "erro", Texto: "Falha ao consultar o assistente: " + err.Error()}, nil
		}
		plano, perr := parsePlanoCopilot(raw)
		if perr != nil {
			// A IA não devolveu JSON válido — usa o texto como resposta direta.
			return model.CopilotResposta{Tipo: "resposta", Texto: strings.TrimSpace(raw)}, nil
		}
		switch plano.AcaoTipo {
		case "responder":
			return model.CopilotResposta{Tipo: "resposta", Texto: plano.Resposta}, nil
		case "confirmar":
			return model.CopilotResposta{Tipo: "confirmacao", Operacao: plano.Operacao, Args: plano.Args, Resumo: plano.Resumo}, nil
		case "ferramenta":
			res := s.execFerramentaLeitura(ctx, plano.Ferramenta, plano.Args)
			ctxFerramentas += fmt.Sprintf("\n- %s(%s) => %s", plano.Ferramenta, jsonCompacto(plano.Args), res)
		default:
			return model.CopilotResposta{Tipo: "resposta", Texto: strings.TrimSpace(raw)}, nil
		}
	}
	return model.CopilotResposta{Tipo: "resposta", Texto: "Não consegui concluir o pedido com as ferramentas disponíveis. Pode reformular?"}, nil
}

type planoCopilot struct {
	AcaoTipo   string                 `json:"acao_tipo"`
	Resposta   string                 `json:"resposta"`
	Ferramenta string                 `json:"ferramenta"`
	Operacao   string                 `json:"operacao"`
	Resumo     string                 `json:"resumo"`
	Args       map[string]interface{} `json:"args"`
}

// parsePlanoCopilot extrai o objeto JSON da resposta da IA (tolerante a fences/ruído).
func parsePlanoCopilot(raw string) (planoCopilot, error) {
	s := strings.TrimSpace(raw)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	ini := strings.Index(s, "{")
	fim := strings.LastIndex(s, "}")
	if ini < 0 || fim <= ini {
		return planoCopilot{}, fmt.Errorf("sem JSON")
	}
	var p planoCopilot
	if err := json.Unmarshal([]byte(s[ini:fim+1]), &p); err != nil {
		return planoCopilot{}, err
	}
	if p.AcaoTipo == "" {
		return planoCopilot{}, fmt.Errorf("acao_tipo ausente")
	}
	return p, nil
}

func construirPromptCopilot(mensagens []model.CopilotMensagem, ctxFerramentas string) string {
	var sb strings.Builder
	sb.WriteString("Conversa até agora:\n")
	for _, m := range mensagens {
		quem := "Usuário"
		if m.Role == "assistant" {
			quem = "Assistente"
		}
		sb.WriteString(quem + ": " + m.Texto + "\n")
	}
	if strings.TrimSpace(ctxFerramentas) != "" {
		sb.WriteString("\nResultados de ferramentas executadas nesta rodada:" + ctxFerramentas + "\n")
	}
	sb.WriteString("\nDecida o próximo passo e responda SOMENTE com o objeto JSON do contrato.")
	return sb.String()
}

// ── Ferramentas de LEITURA ──────────────────────────────────────────────────

func (s *CopilotService) execFerramentaLeitura(ctx context.Context, nome string, args map[string]interface{}) string {
	switch nome {
	case "consultar_movimento":
		return s.toolConsultarMovimento(ctx, args)
	case "listar_regras":
		return s.toolListarRegras(ctx)
	case "consultar_posicao":
		return s.toolConsultarPosicao(ctx, args)
	default:
		return "erro: ferramenta desconhecida " + nome
	}
}

func (s *CopilotService) toolConsultarMovimento(ctx context.Context, args map[string]interface{}) string {
	data, err := parseDataArg(args, "data")
	if err != nil {
		return "erro: " + err.Error()
	}
	pag, err := s.mov.ConsultarLancamentosFiltradoEscopo(ctx, data, data,
		argStr(args, "boleto"), "", argStr(args, "produto"), argStr(args, "dominio"), 0, "vigente", 1, 100)
	if err != nil {
		return "erro ao consultar movimento: " + err.Error()
	}
	type l struct {
		Boleto, Debito, Credito, Moeda, Regra string
		Valor                                 float64
		Reversao                              bool
	}
	out := struct {
		Total       int     `json:"total"`
		SomaValor   float64 `json:"soma_valor"`
		Lancamentos []l     `json:"lancamentos"`
	}{Total: pag.Total}
	for i, mv := range pag.Lancamentos {
		out.SomaValor += mv.ValorLancamentoContabil
		if i < 25 {
			out.Lancamentos = append(out.Lancamentos, l{mv.CodigoIdentificadorBoleto, mv.ContaDebito, mv.ContaCredito, mv.MoedaLancamentoContabil, mv.DescricaoRegraContabil, mv.ValorLancamentoContabil, mv.IndicadorReversao})
		}
	}
	return jsonCompacto(out)
}

func (s *CopilotService) toolListarRegras(ctx context.Context) string {
	regras, err := s.regra.ListarRegras(ctx)
	if err != nil {
		return "erro ao listar regras: " + err.Error()
	}
	type r struct {
		ID                       int64  `json:"id"`
		Descricao, Produto, Dominio, Status, Tipo string
		Condicoes                int `json:"condicoes"`
	}
	var out []r
	for _, rg := range regras {
		out = append(out, r{rg.ID, rg.Descricao, rg.CodigoProdutoCorporativo, rg.Dominio, model.NormalizaStatus(rg.Status, rg.Ativo), model.NormalizaTipoLancamento(rg.TipoLancamento), len(rg.Condicoes)})
	}
	return jsonCompacto(out)
}

func (s *CopilotService) toolConsultarPosicao(ctx context.Context, args map[string]interface{}) string {
	data, err := parseDataArg(args, "data")
	if err != nil {
		return "erro: " + err.Error()
	}
	posicoes, err := s.pos.ListarPorData(ctx, data)
	if err != nil {
		return "erro ao consultar posição: " + err.Error()
	}
	boleto := argStr(args, "boleto")
	var out []map[string]interface{}
	for _, p := range posicoes {
		if boleto != "" && campoStrModel(p, "codigo_identificador_boleto") != boleto {
			continue
		}
		out = append(out, p.Campos)
		if len(out) >= 10 {
			break
		}
	}
	if len(out) == 0 {
		return fmt.Sprintf(`{"total":%d,"amostra":[],"aviso":"nenhuma posição no filtro"}`, len(posicoes))
	}
	return jsonCompacto(struct {
		Total   int                      `json:"total"`
		Amostra []map[string]interface{} `json:"amostra"`
	}{len(posicoes), out})
}

// ── Ações de ESCRITA (após confirmação do usuário) ──────────────────────────

// ExecutarAcao executa uma ação previamente proposta e confirmada pelo usuário.
func (s *CopilotService) ExecutarAcao(ctx context.Context, operacao string, args map[string]interface{}) (string, error) {
	if !s.Disponivel() {
		return "", fmt.Errorf("assistente indisponível")
	}
	switch operacao {
	case "criar_regra":
		return s.acaoCriarRegra(ctx, args)
	case "reprocessar":
		return s.acaoReprocessar(ctx, args)
	case "liberar_competencia":
		return s.acaoLiberar(ctx, args)
	default:
		return "", fmt.Errorf("operação desconhecida: %s", operacao)
	}
}

func (s *CopilotService) acaoCriarRegra(ctx context.Context, args map[string]interface{}) (string, error) {
	descricao := strings.TrimSpace(argStr(args, "descricao"))
	if descricao == "" {
		return "", fmt.Errorf("descrição da regra é obrigatória")
	}
	regra := model.RegraContabil{
		Descricao:                descricao,
		CodigoProdutoCorporativo: argStr(args, "produto"),
		Dominio:                  argStr(args, "dominio"),
		PreCondicao:              argStr(args, "pre_condicao"),
		TipoLancamento:           argStr(args, "tipo_lancamento"),
		Status:                   model.StatusRascunho, // nasce como rascunho (não vale na geração até ativar)
	}
	id, err := s.regra.CriarRegra(ctx, regra)
	if err != nil {
		return "", err
	}
	conds, _ := args["condicoes"].([]interface{})
	criadas := 0
	for _, ci := range conds {
		cm, _ := ci.(map[string]interface{})
		if cm == nil {
			continue
		}
		c := model.CondicaoRegra{
			IDRegra:      id,
			Condicao:     argStr(cm, "condicao"),
			CampoValor:   argStr(cm, "campo_valor"),
			CampoMoeda:   argStr(cm, "campo_moeda"),
			ContaDebito:  argStr(cm, "conta_debito"),
			ContaCredito: argStr(cm, "conta_credito"),
			CampoBoleto:  argStr(cm, "campo_boleto"),
			Status:       model.StatusRascunho,
			// Moeda base: por padrão converte na geração pela data da posição (mesmo default do What-if).
			BaseNaPosicao:      false,
			CampoDataConversao: "data_posicao_carteira",
		}
		if _, err := s.regra.CriarCondicao(ctx, c); err != nil {
			return "", fmt.Errorf("regra #%d criada, mas falhou ao criar condição: %w", id, err)
		}
		criadas++
	}
	return fmt.Sprintf("Regra #%d criada como RASCUNHO com %d condição(ões). Revise em Regras e ative quando quiser.", id, criadas), nil
}

func (s *CopilotService) acaoReprocessar(ctx context.Context, args map[string]interface{}) (string, error) {
	data, err := parseDataArg(args, "data")
	if err != nil {
		return "", err
	}
	res, err := s.mov.ReprocessarSeletivo(ctx, data, argStr(args, "produto"), argStr(args, "dominio"), argStr(args, "boleto"))
	if err != nil {
		return "", err
	}
	return res.Mensagem, nil
}

func (s *CopilotService) acaoLiberar(ctx context.Context, args map[string]interface{}) (string, error) {
	if s.fech == nil {
		return "", fmt.Errorf("liberação de competência indisponível neste backend")
	}
	ano := argInt(args, "ano")
	mes := argInt(args, "mes")
	if err := s.fech.Liberar(ctx, ano, mes, argStr(args, "produto"), argStr(args, "dominio")); err != nil {
		return "", err
	}
	return fmt.Sprintf("Competência %04d-%02d liberada.", ano, mes), nil
}

// ── Helpers de args ─────────────────────────────────────────────────────────

func argStr(args map[string]interface{}, k string) string {
	if args == nil {
		return ""
	}
	if v, ok := args[k]; ok {
		switch t := v.(type) {
		case string:
			return strings.TrimSpace(t)
		case float64:
			return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.0f", t), "0"), ".")
		}
	}
	return ""
}

func argInt(args map[string]interface{}, k string) int {
	if args == nil {
		return 0
	}
	switch t := args[k].(type) {
	case float64:
		return int(t)
	case string:
		var n int
		fmt.Sscanf(strings.TrimSpace(t), "%d", &n)
		return n
	}
	return 0
}

func parseDataArg(args map[string]interface{}, k string) (time.Time, error) {
	ds := argStr(args, k)
	if ds == "" {
		return time.Time{}, fmt.Errorf("informe a data (%s) no formato AAAA-MM-DD", k)
	}
	t, err := time.Parse("2006-01-02", ds)
	if err != nil {
		return time.Time{}, fmt.Errorf("data inválida %q (use AAAA-MM-DD)", ds)
	}
	return t, nil
}

func jsonCompacto(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}
