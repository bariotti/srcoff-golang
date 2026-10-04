package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"srcoff/internal/model"
)

// fakeRegraSvc implementa regraContabilSvc para testes de importação/exportação.
type fakeRegraSvc struct {
	regras         []model.RegraContabil
	regrasEditada  []int64
	condsEditada   []int64
	regrasCriadas  []model.RegraContabil
	condsCriadas   []model.CondicaoRegra
	proximoRegraID int64
}

func (f *fakeRegraSvc) ListarRegras(context.Context) ([]model.RegraContabil, error) {
	return f.regras, nil
}
func (f *fakeRegraSvc) CriarRegra(_ context.Context, r model.RegraContabil) (int64, error) {
	f.regrasCriadas = append(f.regrasCriadas, r)
	if f.proximoRegraID == 0 {
		f.proximoRegraID = 1000
	}
	f.proximoRegraID++
	return f.proximoRegraID, nil
}
func (f *fakeRegraSvc) EditarRegra(_ context.Context, r model.RegraContabil) error {
	f.regrasEditada = append(f.regrasEditada, r.ID)
	return nil
}
func (f *fakeRegraSvc) ExcluirRegra(context.Context, int64) error { return nil }
func (f *fakeRegraSvc) ListarCondicoes(context.Context, int64) ([]model.CondicaoRegra, error) {
	return nil, nil
}
func (f *fakeRegraSvc) CriarCondicao(_ context.Context, c model.CondicaoRegra) (int64, error) {
	f.condsCriadas = append(f.condsCriadas, c)
	return int64(len(f.condsCriadas)), nil
}
func (f *fakeRegraSvc) EditarCondicao(_ context.Context, c model.CondicaoRegra) error {
	f.condsEditada = append(f.condsEditada, c.ID)
	return nil
}
func (f *fakeRegraSvc) ExcluirCondicao(context.Context, int64) error { return nil }

// TestImportarRegrasCSV_UpdateSkipErro valida: regra alterada é atualizada, condição
// idêntica é pulada e ids numéricos inexistentes viram erro (não criam).
func TestImportarRegrasCSV_UpdateSkipErro(t *testing.T) {
	svc := &fakeRegraSvc{regras: []model.RegraContabil{
		{
			ID: 1, Descricao: "Original", Dominio: "Posição", Ativo: true, PostaReverte: true,
			Condicoes: []model.CondicaoRegra{
				{ID: 10, IDRegra: 1, Condicao: "v>0", ContaDebito: "1001", ContaCredito: "2001", CampoValor: "v", CampoMoeda: "m", CampoBoleto: "b", Ativo: true},
			},
		},
	}}
	h := NewRegraContabilHandler(svc)

	// regra 1: descrição alterada → deve atualizar a regra.
	// condição 10: idêntica → não deve atualizar.
	// regra 999 / condição 888: ids numéricos inexistentes → erro (não criam).
	csv := strings.Join([]string{
		"regra_id;descricao;codigo_produto_corporativo;dominio;campo_produto;pre_condicao;regra_ativa;posta_reverte;condicao_id;condicao;conta_debito;conta_credito;campo_valor;campo_moeda;campo_boleto;condicao_ativa",
		"1;ALTERADA;;Posição;;;Sim;Sim;10;v>0;1001;2001;v;m;b;Sim",
		"999;Fantasma;;;;;Sim;Não;888;x>0;1;2;v;m;b;Sim",
	}, "\n") + "\n"

	req := multipartReq(t, "/api/v1/regras/import", "arquivo", "regras.csv", csv)
	rec := httptest.NewRecorder()
	h.ImportarRegrasCSV(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, corpo = %s", rec.Code, rec.Body.String())
	}
	var res ResultadoImportacaoRegras
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("resposta inválida: %v", err)
	}
	if res.RegrasAtualizadas != 1 {
		t.Fatalf("esperado 1 regra atualizada, obteve %d", res.RegrasAtualizadas)
	}
	if res.CondicoesInalteradas != 1 {
		t.Fatalf("esperado 1 condição inalterada, obteve %d", res.CondicoesInalteradas)
	}
	// regra 999 e condição 888 inexistentes → 2 erros (nenhuma criação).
	if len(res.Erros) != 2 {
		t.Fatalf("esperado 2 erros (ids inexistentes), obteve %d: %v", len(res.Erros), res.Erros)
	}
	if res.RegrasCriadas != 0 || res.CondicoesCriadas != 0 {
		t.Fatalf("nada deveria ser criado, criadas regras=%d conds=%d", res.RegrasCriadas, res.CondicoesCriadas)
	}
	if len(svc.regrasEditada) != 1 || svc.regrasEditada[0] != 1 {
		t.Fatalf("apenas a regra 1 deveria ser editada, obteve %v", svc.regrasEditada)
	}
}

// TestImportarRegrasCSV_Criacao valida a criação de regras e condições novas: regra por
// rótulo com condição, e condição nova numa regra existente.
func TestImportarRegrasCSV_Criacao(t *testing.T) {
	svc := &fakeRegraSvc{regras: []model.RegraContabil{
		{ID: 1, Descricao: "Existente", Dominio: "Posição", Ativo: true, PostaReverte: true},
	}}
	h := NewRegraContabilHandler(svc)

	csv := strings.Join([]string{
		"regra_id;descricao;codigo_produto_corporativo;dominio;campo_produto;pre_condicao;regra_ativa;posta_reverte;condicao_id;condicao;conta_debito;conta_credito;campo_valor;campo_moeda;campo_boleto;condicao_ativa",
		// regra nova (rótulo) + 1 condição nova
		"NOVA-1;Regra Nova;NDF;Posição;;;Sim;Sim;;v>0;1001;2001;v;m;b;",
		// condição nova em regra existente (id 1), condicao_id vazio
		"1;Existente;;Posição;;;Sim;Sim;;y>0;7001;8001;y;m;b;",
	}, "\n") + "\n"

	req := multipartReq(t, "/api/v1/regras/import", "arquivo", "regras.csv", csv)
	rec := httptest.NewRecorder()
	h.ImportarRegrasCSV(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, corpo = %s", rec.Code, rec.Body.String())
	}
	var res ResultadoImportacaoRegras
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("resposta inválida: %v", err)
	}
	if res.RegrasCriadas != 1 {
		t.Fatalf("esperado 1 regra criada, obteve %d (erros: %v)", res.RegrasCriadas, res.Erros)
	}
	if res.CondicoesCriadas != 2 {
		t.Fatalf("esperado 2 condições criadas (1 na regra nova + 1 na existente), obteve %d (erros: %v)", res.CondicoesCriadas, res.Erros)
	}
	if res.RegrasInalteradas != 1 {
		t.Fatalf("a regra existente 1 deveria ficar inalterada, obteve %d", res.RegrasInalteradas)
	}
	if len(svc.regrasCriadas) != 1 || svc.regrasCriadas[0].Descricao != "Regra Nova" {
		t.Fatalf("regra nova não criada corretamente: %#v", svc.regrasCriadas)
	}
	// A condição da regra nova deve ter recebido o IDRegra gerado na criação.
	var achouNaRegraNova bool
	for _, c := range svc.condsCriadas {
		if c.IDRegra == svc.proximoRegraID {
			achouNaRegraNova = true
		}
	}
	if !achouNaRegraNova {
		t.Fatalf("a condição da regra nova deveria apontar para o id gerado %d: %#v", svc.proximoRegraID, svc.condsCriadas)
	}
}

func multipartReq(t *testing.T, url, field, filename, content string) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile(field, filename)
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	fw.Write([]byte(content))
	mw.Close()
	req := httptest.NewRequest(http.MethodPost, url, &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	return req
}
