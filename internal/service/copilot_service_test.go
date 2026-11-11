package service

import (
	"context"
	"testing"
	"time"

	"srcoff/internal/ai"
	"srcoff/internal/model"
)

// ── Fakes ───────────────────────────────────────────────────────────────────

type fakeCopilotAI struct {
	disp      bool
	respostas []string
	i         int
	prompts   []string
}

func (f *fakeCopilotAI) Disponivel() bool { return f.disp }
func (f *fakeCopilotAI) Nome() string     { return "fake" }
func (f *fakeCopilotAI) Modelo() string   { return "fake-model" }
func (f *fakeCopilotAI) Gerar(_ context.Context, _ string, msgs []ai.Mensagem, _ int) (string, error) {
	if len(msgs) > 0 {
		f.prompts = append(f.prompts, msgs[len(msgs)-1].Texto)
	}
	if f.i >= len(f.respostas) {
		return `{"acao_tipo":"responder","resposta":"ok"}`, nil
	}
	r := f.respostas[f.i]
	f.i++
	return r, nil
}

type fakeCpRegra struct {
	regras    []model.RegraContabil
	criouID   int64
	criouCond int
}

func (f *fakeCpRegra) ListarRegras(context.Context) ([]model.RegraContabil, error) {
	return f.regras, nil
}
func (f *fakeCpRegra) CriarRegra(context.Context, model.RegraContabil) (int64, error) {
	f.criouID = 7
	return 7, nil
}
func (f *fakeCpRegra) CriarCondicao(context.Context, model.CondicaoRegra) (int64, error) {
	f.criouCond++
	return int64(f.criouCond), nil
}

type fakeCpMov struct{ reprocessou bool }

func (f *fakeCpMov) ConsultarLancamentosFiltradoEscopo(_ context.Context, _, _ time.Time, _, _, _, _ string, _ int, _ string, _, _ int) (*model.PaginaLancamentos, error) {
	return &model.PaginaLancamentos{Total: 2, Lancamentos: []model.LancamentoContabil{
		{CodigoIdentificadorBoleto: "B1", ValorLancamentoContabil: 100, ContaDebito: "1", ContaCredito: "2", MoedaLancamentoContabil: "USD"},
	}}, nil
}
func (f *fakeCpMov) ReprocessarSeletivo(_ context.Context, _ time.Time, _, _, _ string) (*model.ReprocessamentoResultado, error) {
	f.reprocessou = true
	return &model.ReprocessamentoResultado{Mensagem: "reprocessado ok"}, nil
}

type fakeCpPos struct{}

func (f *fakeCpPos) ListarPorData(context.Context, time.Time) ([]model.PosicaoCarteira, error) {
	return nil, nil
}

func novoCopilot(aic *fakeCopilotAI) (*CopilotService, *fakeCpRegra, *fakeCpMov) {
	regra := &fakeCpRegra{regras: []model.RegraContabil{{ID: 1, Descricao: "R1"}}}
	mov := &fakeCpMov{}
	return NewCopilotService(aic, regra, mov, &fakeCpPos{}, nil), regra, mov
}

// ── Testes ──────────────────────────────────────────────────────────────────

func TestCopilotIndisponivel(t *testing.T) {
	svc, _, _ := novoCopilot(&fakeCopilotAI{disp: false})
	if svc.Disponivel() {
		t.Fatal("deveria estar indisponível sem IA")
	}
	resp, _ := svc.Conversar(context.Background(), []model.CopilotMensagem{{Role: "user", Texto: "oi"}})
	if resp.Tipo != "erro" {
		t.Fatalf("esperava tipo erro, veio %q", resp.Tipo)
	}
}

func TestCopilotRespostaDireta(t *testing.T) {
	svc, _, _ := novoCopilot(&fakeCopilotAI{disp: true, respostas: []string{
		`{"acao_tipo":"responder","resposta":"Olá! Como posso ajudar?"}`,
	}})
	resp, err := svc.Conversar(context.Background(), []model.CopilotMensagem{{Role: "user", Texto: "oi"}})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Tipo != "resposta" || resp.Texto != "Olá! Como posso ajudar?" {
		t.Fatalf("resposta inesperada: %+v", resp)
	}
}

func TestCopilotFerramentaDepoisResponde(t *testing.T) {
	aic := &fakeCopilotAI{disp: true, respostas: []string{
		`{"acao_tipo":"ferramenta","ferramenta":"consultar_movimento","args":{"data":"2024-01-15","produto":"NDF"}}`,
		`{"acao_tipo":"responder","resposta":"O NDF gerou 2 lançamentos."}`,
	}}
	svc, _, _ := novoCopilot(aic)
	resp, err := svc.Conversar(context.Background(), []model.CopilotMensagem{{Role: "user", Texto: "quantos lançamentos o NDF gerou em 2024-01-15?"}})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Tipo != "resposta" || resp.Texto != "O NDF gerou 2 lançamentos." {
		t.Fatalf("resposta inesperada: %+v", resp)
	}
	// A 2ª chamada à IA deve ter recebido o resultado da ferramenta no prompt.
	if len(aic.prompts) < 2 || !contains(aic.prompts[1], "consultar_movimento") || !contains(aic.prompts[1], `"total":2`) {
		t.Fatalf("o resultado da ferramenta não foi repassado à IA: %v", aic.prompts)
	}
}

func TestCopilotConfirmacao(t *testing.T) {
	svc, _, _ := novoCopilot(&fakeCopilotAI{disp: true, respostas: []string{
		`{"acao_tipo":"confirmar","operacao":"reprocessar","args":{"data":"2024-01-15","boleto":"B1"},"resumo":"Reprocessar o boleto B1 de 2024-01-15"}`,
	}})
	resp, _ := svc.Conversar(context.Background(), []model.CopilotMensagem{{Role: "user", Texto: "reprocessa o B1 de 2024-01-15"}})
	if resp.Tipo != "confirmacao" || resp.Operacao != "reprocessar" {
		t.Fatalf("esperava confirmação de reprocessar, veio %+v", resp)
	}
	if resp.Args["boleto"] != "B1" {
		t.Fatalf("args não repassados: %+v", resp.Args)
	}
}

func TestCopilotExecutarReprocessar(t *testing.T) {
	svc, _, mov := novoCopilot(&fakeCopilotAI{disp: true})
	txt, err := svc.ExecutarAcao(context.Background(), "reprocessar", map[string]interface{}{"data": "2024-01-15", "boleto": "B1"})
	if err != nil {
		t.Fatal(err)
	}
	if !mov.reprocessou {
		t.Fatal("ReprocessarSeletivo não foi chamado")
	}
	if txt != "reprocessado ok" {
		t.Fatalf("mensagem inesperada: %q", txt)
	}
}

func TestCopilotExecutarCriarRegra(t *testing.T) {
	svc, regra, _ := novoCopilot(&fakeCopilotAI{disp: true})
	txt, err := svc.ExecutarAcao(context.Background(), "criar_regra", map[string]interface{}{
		"descricao": "Nova regra",
		"produto":   "NDF",
		"condicoes": []interface{}{
			map[string]interface{}{"condicao": "valor_mtm > 0", "campo_valor": "valor_mtm", "campo_moeda": "moeda", "conta_debito": "1.1.2", "conta_credito": "4.1.9"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if regra.criouID != 7 || regra.criouCond != 1 {
		t.Fatalf("criação inesperada: id=%d conds=%d", regra.criouID, regra.criouCond)
	}
	if !contains(txt, "RASCUNHO") {
		t.Fatalf("mensagem deveria indicar rascunho: %q", txt)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (indexOf(s, sub) >= 0)
}
func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
