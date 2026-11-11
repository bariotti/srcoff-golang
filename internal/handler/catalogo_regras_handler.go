package handler

import (
	"context"
	"html/template"
	"net/http"
	"time"

	"srcoff/internal/model"
)

type catalogoSvc interface {
	RegrasAtivas(ctx context.Context) ([]model.RegraContabil, error)
}

// CatalogoRegrasHandler gera um documento HTML legível (imprimível em PDF pelo navegador) com
// todas as regras ATIVAS e suas condições — para o time contábil/auditoria (RF-134).
type CatalogoRegrasHandler struct {
	svc catalogoSvc
	tpl *template.Template
}

func NewCatalogoRegrasHandler(svc catalogoSvc) *CatalogoRegrasHandler {
	return &CatalogoRegrasHandler{svc: svc, tpl: template.Must(template.New("catalogo").Funcs(template.FuncMap{
		"tipoLanc":  rotuloTipoLancamento,
		"moedaBase": rotuloMoedaBase,
		"ouTodos":   ouTodos,
	}).Parse(catalogoTpl))}
}

// Catalogo trata GET /api/v1/catalogo-regras — devolve o documento HTML (text/html).
func (h *CatalogoRegrasHandler) Catalogo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	regras, err := h.svc.RegrasAtivas(r.Context())
	if err != nil {
		http.Error(w, "erro ao carregar regras: "+err.Error(), http.StatusInternalServerError)
		return
	}
	nCond := 0
	for _, rg := range regras {
		nCond += len(rg.Condicoes)
	}
	data := struct {
		GeradoEm  string
		Total     int
		TotalCond int
		Regras    []model.RegraContabil
	}{
		GeradoEm:  time.Now().Format("02/01/2006 15:04"),
		Total:     len(regras),
		TotalCond: nCond,
		Regras:    regras,
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.tpl.Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func rotuloTipoLancamento(t string) string {
	switch model.NormalizaTipoLancamento(t) {
	case model.TipoNaoReverte:
		return "Não reverte"
	case model.TipoIncremental:
		return "Incremental / Não reverte"
	case model.TipoIncrementalReverte:
		return "Incremental / Reverte"
	default:
		return "Reverte"
	}
}

func rotuloMoedaBase(c model.CondicaoRegra) string {
	if c.BaseNaPosicao {
		return "Da posição (" + ouTraco(c.CampoValorMoedaBase) + " / " + ouTraco(c.CampoMoedaBase) + ")"
	}
	return "Convertida (data: " + ouTraco(c.CampoDataConversao) + ")"
}

func ouTodos(s string) string {
	if s == "" {
		return "Todos"
	}
	return s
}

func ouTraco(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

const catalogoTpl = `<!DOCTYPE html>
<html lang="pt-BR"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Catálogo de Regras Contábeis</title>
<style>
  :root { color-scheme: light; }
  * { box-sizing: border-box; }
  body { font-family: -apple-system, Segoe UI, Roboto, Arial, sans-serif; color: #212529; margin: 0; padding: 24px 32px; background: #fff; }
  header { border-bottom: 2px solid #0d6efd; padding-bottom: 12px; margin-bottom: 20px; }
  h1 { font-size: 20px; margin: 0 0 4px; }
  .meta { color: #6c757d; font-size: 13px; }
  .toolbar { margin: 12px 0 20px; }
  .toolbar button { font-size: 13px; padding: 6px 14px; border: 1px solid #0d6efd; background: #0d6efd; color: #fff; border-radius: 6px; cursor: pointer; }
  .regra { border: 1px solid #dee2e6; border-radius: 8px; padding: 14px 16px; margin-bottom: 16px; page-break-inside: avoid; }
  .regra h2 { font-size: 15px; margin: 0 0 8px; }
  .regra .id { color: #6c757d; font-weight: normal; }
  .campos { font-size: 13px; color: #343a40; margin-bottom: 10px; display: flex; flex-wrap: wrap; gap: 4px 18px; }
  .campos b { color: #495057; font-weight: 600; }
  code { background: #f1f3f5; padding: 1px 5px; border-radius: 4px; font-size: 12px; }
  table { width: 100%; border-collapse: collapse; font-size: 12px; }
  th, td { border: 1px solid #e9ecef; padding: 5px 8px; text-align: left; vertical-align: top; }
  th { background: #f8f9fa; font-weight: 600; }
  .sem-cond { color: #adb5bd; font-size: 12px; font-style: italic; }
  .resumo { background: #f8f9fa; border: 1px solid #e9ecef; border-radius: 6px; padding: 8px 12px; font-size: 13px; margin-bottom: 18px; }
  @media print { .toolbar { display: none; } body { padding: 0; } .regra { border-color: #ccc; } }
</style></head>
<body>
<header>
  <h1>Catálogo de Regras Contábeis</h1>
  <div class="meta">SRCOff · gerado em {{.GeradoEm}}</div>
</header>
<div class="toolbar"><button onclick="window.print()">Imprimir / Salvar PDF</button></div>
<div class="resumo"><b>{{.Total}}</b> regra(s) ativa(s) · <b>{{.TotalCond}}</b> condição(ões) ativa(s)</div>
{{if not .Regras}}<p class="sem-cond">Nenhuma regra ativa cadastrada.</p>{{end}}
{{range .Regras}}
<div class="regra">
  <h2><span class="id">#{{.ID}}</span> {{.Descricao}}</h2>
  <div class="campos">
    <span><b>Produtos:</b> {{ouTodos .CodigoProdutoCorporativo}}</span>
    <span><b>Domínios:</b> {{ouTodos .Dominio}}</span>
    <span><b>Tipo:</b> {{tipoLanc .TipoLancamento}}</span>
    {{if .CampoProduto}}<span><b>Campo produto:</b> <code>{{.CampoProduto}}</code></span>{{end}}
    {{if .PreCondicao}}<span><b>Pré-condição:</b> <code>{{.PreCondicao}}</code></span>{{end}}
  </div>
  {{if .Condicoes}}
  <table>
    <thead><tr><th>Condição</th><th>Débito</th><th>Crédito</th><th>Campo valor</th><th>Moeda</th><th>Boleto</th><th>Moeda base do veículo</th></tr></thead>
    <tbody>
    {{range .Condicoes}}
      <tr>
        <td><code>{{.Condicao}}</code></td>
        <td>{{.ContaDebito}}</td>
        <td>{{.ContaCredito}}</td>
        <td><code>{{.CampoValor}}</code></td>
        <td>{{.CampoMoeda}}</td>
        <td>{{if .CampoBoleto}}<code>{{.CampoBoleto}}</code>{{else}}—{{end}}</td>
        <td>{{moedaBase .}}</td>
      </tr>
    {{end}}
    </tbody>
  </table>
  {{else}}<div class="sem-cond">Sem condições ativas.</div>{{end}}
</div>
{{end}}
</body></html>`
