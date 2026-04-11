package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// Template data structs
// ---------------------------------------------------------------------------

type operacaoData struct {
	MensagemMovimento string
	ErroMovimento     string
	MensagemEstorno   string
	ErroEstorno       string
}

type consultaData struct {
	DataInicio       string
	DataFim          string
	Boleto           string
	VersaoModo       string
	Versao           string
	Pagina           int
	Tamanho          int
	Resultado        *paginaLancamentosView
	Erro             string
	ErroExclusao     string
	MensagemExclusao string
	MensagemTxt      string
	TemAnterior      bool
	PaginaAnterior   int
	TemProxima       bool
	ProximaPagina    int
}

type paginaLancamentosView struct {
	Total       int              `json:"total"`
	Pagina      int              `json:"pagina"`
	Tamanho     int              `json:"tamanho"`
	Lancamentos []lancamentoView `json:"lancamentos"`
}

type lancamentoView struct {
	DataLoteContabil          time.Time `json:"data_lote_contabil"`
	CodigoVersaoConteudo      int       `json:"codigo_versao_conteudo"`
	CodigoIdentificadorBoleto string    `json:"codigo_identificador_boleto"`
	ContaDebito               string    `json:"conta_debito"`
	ContaCredito              string    `json:"conta_credito"`
	ValorLancamentoContabil   float64   `json:"valor_lancamento_contabil"`
	MoedaLancamentoContabil   string    `json:"moeda_lancamento_contabil"`
	IndicadorReversao         bool      `json:"indicador_reversao"`
	DescricaoRegraContabil    string    `json:"descricao_regra_contabil"`
}

type posicaoData struct {
	Data      string
	Posicoes  []map[string]interface{}
	Mensagem  string
	Erro      string
}

type conciliacaoData struct {
	Data            string
	TotalPosicoes   int
	TotalMovimentos int
	Inconsistencias []inconsistenciaView
	Erro            string
}

type inconsistenciaView struct {
	Tipo                      string
	CodigoIdentificadorBoleto string
	DescricaoRegra            string
	IndicadorReversao         bool
	Detalhe                   string
}

type regrasData struct {
	Regras           []regraView
	RegraSelecionada *regraView
	Mensagem         string
	Erro             string
}

type regraView struct {
	ID                       int64
	Descricao                string
	CodigoProdutoCorporativo string
	Condicoes                []condicaoView
}

type condicaoView struct {
	ID           int64
	IDRegra      int64
	Condicao     string
	ContaDebito  string
	ContaCredito string
	CampoValor   string
	CampoMoeda   string
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func proxyPost(client *http.Client, url string, body interface{}) (map[string]interface{}, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	resp, err := client.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var result map[string]interface{}
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func proxyPut(client *http.Client, url string, body interface{}) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPut, url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

func renderTemplate(tmpl *template.Template, w http.ResponseWriter, name string, data interface{}) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, name, data); err != nil {
		log.Printf("erro ao renderizar template %s: %v", name, err)
		http.Error(w, "Erro interno ao renderizar página", http.StatusInternalServerError)
	}
}

// ---------------------------------------------------------------------------
// main
// ---------------------------------------------------------------------------

func main() {
	tmpl := template.Must(template.ParseGlob("cmd/frontend/templates/*.html"))

	apiURL := os.Getenv("API_URL")
	if apiURL == "" {
		apiURL = "http://localhost:8080"
	}
	frontendPort := os.Getenv("FRONTEND_PORT")
	if frontendPort == "" {
		frontendPort = "9090"
	}

	client := &http.Client{Timeout: 30 * time.Second}

	// GET /operacao
	http.HandleFunc("/operacao", func(w http.ResponseWriter, r *http.Request) {
		renderTemplate(tmpl, w, "operacao.html", operacaoData{})
	})

	// POST /operacao/movimento
	http.HandleFunc("/operacao/movimento", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Redirect(w, r, "/operacao", http.StatusSeeOther)
			return
		}
		data := r.FormValue("data")
		result, err := proxyPost(client, apiURL+"/api/v1/movimento-contabil", map[string]string{"data": data})
		d := operacaoData{}
		if err != nil {
			d.ErroMovimento = fmt.Sprintf("Erro ao comunicar com a API: %v", err)
		} else if errMsg, ok := result["erro"].(string); ok && errMsg != "" {
			d.ErroMovimento = errMsg
		} else if msg, ok := result["mensagem"].(string); ok {
			d.MensagemMovimento = msg
		} else {
			d.MensagemMovimento = "Movimento gerado com sucesso."
		}
		renderTemplate(tmpl, w, "operacao.html", d)
	})

	// POST /operacao/estorno
	http.HandleFunc("/operacao/estorno", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Redirect(w, r, "/operacao", http.StatusSeeOther)
			return
		}
		data := r.FormValue("data")
		result, err := proxyPost(client, apiURL+"/api/v1/estorno", map[string]string{"data": data})
		d := operacaoData{}
		if err != nil {
			d.ErroEstorno = fmt.Sprintf("Erro ao comunicar com a API: %v", err)
		} else if errMsg, ok := result["erro"].(string); ok && errMsg != "" {
			d.ErroEstorno = errMsg
		} else if msg, ok := result["mensagem"].(string); ok {
			d.MensagemEstorno = msg
		} else {
			d.MensagemEstorno = "Estorno gerado com sucesso."
		}
		renderTemplate(tmpl, w, "operacao.html", d)
	})

	// GET /consulta/export — proxy para download CSV
	http.HandleFunc("/consulta/export", func(w http.ResponseWriter, r *http.Request) {
		apiURL2 := apiURL + "/api/v1/movimento-contabil/export?" + r.URL.RawQuery
		resp, err := client.Get(apiURL2)
		if err != nil {
			http.Error(w, "Erro ao gerar export: "+err.Error(), http.StatusInternalServerError)
			return
		}
		defer resp.Body.Close()
		w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
		w.Header().Set("Content-Disposition", resp.Header.Get("Content-Disposition"))
		io.Copy(w, resp.Body)
	})

	// GET /consulta/export-txt — proxy para download TXT
	http.HandleFunc("/consulta/export-txt", func(w http.ResponseWriter, r *http.Request) {
		dataParam := r.URL.Query().Get("data")
		apiURL2 := fmt.Sprintf("%s/api/v1/movimento-contabil/export-txt?data=%s", apiURL, dataParam)
		resp, err := client.Get(apiURL2)
		if err != nil {
			http.Error(w, "Erro ao gerar export TXT: "+err.Error(), http.StatusInternalServerError)
			return
		}
		defer resp.Body.Close()

		// Verificar se é sem_dados
		if resp.Header.Get("Content-Type") == "application/json" {
			var result map[string]string
			json.NewDecoder(resp.Body).Decode(&result)
			if msg, ok := result["sem_dados"]; ok {
				// Redirecionar para consulta com mensagem
				http.Redirect(w, r, "/consulta?msg_txt="+msg, http.StatusSeeOther)
				return
			}
		}

		w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
		w.Header().Set("Content-Disposition", resp.Header.Get("Content-Disposition"))
		io.Copy(w, resp.Body)
	})

	// POST /consulta/excluir
	http.HandleFunc("/consulta/excluir", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Redirect(w, r, "/consulta", http.StatusSeeOther)
			return
		}
		r.ParseForm()
		dataStr := r.FormValue("data")
		versaoStr := r.FormValue("versao")

		url := fmt.Sprintf("%s/api/v1/movimento-contabil?data=%s", apiURL, dataStr)
		if versaoStr != "" {
			url += "&versao=" + versaoStr
		}
		req, _ := http.NewRequest(http.MethodDelete, url, nil)
		resp, err := client.Do(req)
		d := consultaData{}
		if err != nil {
			d.ErroExclusao = fmt.Sprintf("Erro ao excluir: %v", err)
		} else {
			defer resp.Body.Close()
			if resp.StatusCode >= 400 {
				var errResp map[string]string
				json.NewDecoder(resp.Body).Decode(&errResp)
				d.ErroExclusao = fmt.Sprintf("Erro da API: %s", errResp["erro"])
			} else {
				if versaoStr != "" {
					d.MensagemExclusao = fmt.Sprintf("Movimento da data %s versão %s excluído com sucesso.", dataStr, versaoStr)
				} else {
					d.MensagemExclusao = fmt.Sprintf("Todos os movimentos da data %s excluídos com sucesso.", dataStr)
				}
			}
		}
		renderTemplate(tmpl, w, "consulta.html", d)
	})

	// GET /consulta
	http.HandleFunc("/consulta", func(w http.ResponseWriter, r *http.Request) {
		dataInicio := r.URL.Query().Get("data_inicio")
		dataFim := r.URL.Query().Get("data_fim")
		boleto := r.URL.Query().Get("boleto")
		versaoModo := r.URL.Query().Get("versao_modo")
		versao := r.URL.Query().Get("versao")
		paginaParam := r.URL.Query().Get("pagina")
		tamanhoParam := r.URL.Query().Get("tamanho")

		if versaoModo == "" {
			versaoModo = "vigente"
		}

		pagina := 1
		if p, err := strconv.Atoi(paginaParam); err == nil && p > 0 {
			pagina = p
		}
		tamanho := 100
		if t, err := strconv.Atoi(tamanhoParam); err == nil && t > 0 {
			tamanho = t
		}

		d := consultaData{DataInicio: dataInicio, DataFim: dataFim, Boleto: boleto, VersaoModo: versaoModo, Versao: versao, Pagina: pagina, Tamanho: tamanho, MensagemTxt: r.URL.Query().Get("msg_txt")}

		if dataInicio == "" && dataFim == "" && boleto == "" {
			renderTemplate(tmpl, w, "consulta.html", d)
			return
		}

		apiURL2 := fmt.Sprintf("%s/api/v1/movimento-contabil?data_inicio=%s&data_fim=%s&boleto=%s&versao_modo=%s&versao=%s&pagina=%d&tamanho=%d",
			apiURL, dataInicio, dataFim, boleto, versaoModo, versao, pagina, tamanho)
		resp, err := client.Get(apiURL2)
		if err != nil {
			d.Erro = fmt.Sprintf("Erro ao comunicar com a API: %v", err)
			renderTemplate(tmpl, w, "consulta.html", d)
			return
		}
		defer resp.Body.Close()

		var raw map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
			d.Erro = fmt.Sprintf("Erro ao decodificar resposta: %v", err)
			renderTemplate(tmpl, w, "consulta.html", d)
			return
		}
		if errMsg, ok := raw["erro"].(string); ok && errMsg != "" {
			d.Erro = errMsg
			renderTemplate(tmpl, w, "consulta.html", d)
			return
		}

		rawBytes, _ := json.Marshal(raw)
		var view paginaLancamentosView
		if err := json.Unmarshal(rawBytes, &view); err != nil {
			d.Erro = fmt.Sprintf("Erro ao processar dados: %v", err)
			renderTemplate(tmpl, w, "consulta.html", d)
			return
		}
		d.Resultado = &view
		if pagina > 1 {
			d.TemAnterior = true
			d.PaginaAnterior = pagina - 1
		}
		totalPages := (view.Total + tamanho - 1) / tamanho
		if pagina < totalPages {
			d.TemProxima = true
			d.ProximaPagina = pagina + 1
		}
		renderTemplate(tmpl, w, "consulta.html", d)
	})

	// GET /regras/condicao/editar — must be before /regras/
	http.HandleFunc("/regras/condicao/editar", func(w http.ResponseWriter, r *http.Request) {
		idRegra := r.URL.Query().Get("idRegra")
		http.Redirect(w, r, "/regras?id="+idRegra, http.StatusSeeOther)
	})

	// POST /regras/condicao/salvar
	http.HandleFunc("/regras/condicao/salvar", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Redirect(w, r, "/regras", http.StatusSeeOther)
			return
		}
		id := r.FormValue("id")
		idRegra := r.FormValue("idRegra")
		payload := map[string]string{
			"condicao":     r.FormValue("condicao"),
			"conta_debito": r.FormValue("conta_debito"),
			"conta_credito": r.FormValue("conta_credito"),
			"campo_valor":  r.FormValue("campo_valor"),
			"campo_moeda":  r.FormValue("campo_moeda"),
		}
		if err := proxyPut(client, fmt.Sprintf("%s/api/v1/condicoes/%s", apiURL, id), payload); err != nil {
			log.Printf("erro ao salvar condição: %v", err)
		}
		http.Redirect(w, r, "/regras?id="+idRegra, http.StatusSeeOther)
	})

	// POST /regras/nova — must be before /regras/
	http.HandleFunc("/regras/nova", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Redirect(w, r, "/regras", http.StatusSeeOther)
			return
		}
		payload := map[string]string{
			"descricao":                 r.FormValue("descricao"),
			"codigo_produto_corporativo": r.FormValue("codigo_produto_corporativo"),
		}
		result, err := proxyPost(client, apiURL+"/api/v1/regras", payload)
		if err != nil {
			renderTemplate(tmpl, w, "regras.html", regrasData{Erro: fmt.Sprintf("Erro ao comunicar com a API: %v", err)})
			return
		}
		if errMsg, ok := result["erro"].(string); ok && errMsg != "" {
			renderTemplate(tmpl, w, "regras.html", regrasData{Erro: errMsg})
			return
		}
		http.Redirect(w, r, "/regras", http.StatusSeeOther)
	})

	// /regras/ — handles /regras/{id}/editar and /regras/{id}/condicao/nova
	http.HandleFunc("/regras/", func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/regras/")
		// /regras/{id}/editar
		if strings.HasSuffix(path, "/editar") {
			idStr := strings.TrimSuffix(path, "/editar")
			payload := map[string]string{
				"descricao":                 r.FormValue("descricao"),
				"codigo_produto_corporativo": r.FormValue("codigo_produto_corporativo"),
			}
			if err := proxyPut(client, fmt.Sprintf("%s/api/v1/regras/%s", apiURL, idStr), payload); err != nil {
				log.Printf("erro ao editar regra: %v", err)
			}
			http.Redirect(w, r, "/regras?id="+idStr, http.StatusSeeOther)
			return
		}
		// /regras/{id}/condicao/nova
		if strings.HasSuffix(path, "/condicao/nova") {
			idStr := strings.TrimSuffix(path, "/condicao/nova")
			payload := map[string]string{
				"condicao":     r.FormValue("condicao"),
				"conta_debito": r.FormValue("conta_debito"),
				"conta_credito": r.FormValue("conta_credito"),
				"campo_valor":  r.FormValue("campo_valor"),
				"campo_moeda":  r.FormValue("campo_moeda"),
			}
			if _, err := proxyPost(client, fmt.Sprintf("%s/api/v1/regras/%s/condicoes", apiURL, idStr), payload); err != nil {
				log.Printf("erro ao criar condição: %v", err)
			}
			http.Redirect(w, r, "/regras?id="+idStr, http.StatusSeeOther)
			return
		}
		http.Redirect(w, r, "/regras", http.StatusSeeOther)
	})

	// GET /regras
	http.HandleFunc("/regras", func(w http.ResponseWriter, r *http.Request) {
		d := regrasData{}

		// Fetch all rules
		resp, err := client.Get(apiURL + "/api/v1/regras")
		if err != nil {
			d.Erro = fmt.Sprintf("Erro ao buscar regras: %v", err)
			renderTemplate(tmpl, w, "regras.html", d)
			return
		}
		defer resp.Body.Close()
		var regrasRaw []map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&regrasRaw); err != nil {
			d.Erro = fmt.Sprintf("Erro ao decodificar regras: %v", err)
			renderTemplate(tmpl, w, "regras.html", d)
			return
		}
		for _, rr := range regrasRaw {
			rv := regraView{
				ID:                       int64(toFloat(rr["id"])),
				Descricao:                toString(rr["descricao"]),
				CodigoProdutoCorporativo: toString(rr["codigo_produto_corporativo"]),
			}
			d.Regras = append(d.Regras, rv)
		}

		// If id param is set, fetch conditions
		idParam := r.URL.Query().Get("id")
		if idParam != "" {
			condResp, err := client.Get(fmt.Sprintf("%s/api/v1/regras/%s/condicoes", apiURL, idParam))
		
			if err != nil {
				d.Erro = fmt.Sprintf("Erro ao buscar condições: %v", err)
				renderTemplate(tmpl, w, "regras.html", d)
				return
			}
			defer condResp.Body.Close()
			var condsRaw []map[string]interface{}
			if err := json.NewDecoder(condResp.Body).Decode(&condsRaw); err != nil {
				d.Erro = fmt.Sprintf("Erro ao decodificar condições: %v", err)
				renderTemplate(tmpl, w, "regras.html", d)
				return
			}
			var condicoes []condicaoView
			for _, cr := range condsRaw {
				condicoes = append(condicoes, condicaoView{
					ID:           int64(toFloat(cr["id"])),
					IDRegra:      int64(toFloat(cr["id_regra"])),
					Condicao:     toString(cr["condicao"]),
					ContaDebito:  toString(cr["conta_debito"]),
					ContaCredito: toString(cr["conta_credito"]),
					CampoValor:   toString(cr["campo_valor"]),
					CampoMoeda:   toString(cr["campo_moeda"]),
				})
			}
			// Find the selected rule
			idInt, _ := strconv.ParseInt(idParam, 10, 64)
			for i, rv := range d.Regras {
				if rv.ID == idInt {
					d.Regras[i].Condicoes = condicoes
					d.RegraSelecionada = &d.Regras[i]
					break
				}
			}
		}

		renderTemplate(tmpl, w, "regras.html", d)
	})

	// GET /conciliacao
	http.HandleFunc("/conciliacao", func(w http.ResponseWriter, r *http.Request) {
		dataParam := r.URL.Query().Get("data")
		d := conciliacaoData{Data: dataParam}

		if dataParam == "" {
			renderTemplate(tmpl, w, "conciliacao.html", d)
			return
		}

		resp, err := client.Get(fmt.Sprintf("%s/api/v1/conciliacao?data=%s", apiURL, dataParam))
		if err != nil {
			d.Erro = fmt.Sprintf("Erro ao comunicar com a API: %v", err)
			renderTemplate(tmpl, w, "conciliacao.html", d)
			return
		}
		defer resp.Body.Close()

		var raw map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
			d.Erro = fmt.Sprintf("Erro ao decodificar resposta: %v", err)
			renderTemplate(tmpl, w, "conciliacao.html", d)
			return
		}
		if errMsg, ok := raw["erro"].(string); ok && errMsg != "" {
			d.Erro = errMsg
			renderTemplate(tmpl, w, "conciliacao.html", d)
			return
		}

		d.TotalPosicoes = int(toFloat(raw["TotalPosicoes"]))
		d.TotalMovimentos = int(toFloat(raw["TotalMovimentos"]))

		if incs, ok := raw["Inconsistencias"].([]interface{}); ok {
			for _, item := range incs {
				if m, ok := item.(map[string]interface{}); ok {
					d.Inconsistencias = append(d.Inconsistencias, inconsistenciaView{
						Tipo:                      toString(m["Tipo"]),
						CodigoIdentificadorBoleto: toString(m["CodigoIdentificadorBoleto"]),
						DescricaoRegra:            toString(m["DescricaoRegra"]),
						IndicadorReversao:         m["IndicadorReversao"] == true,
						Detalhe:                   toString(m["Detalhe"]),
					})
				}
			}
		}

		renderTemplate(tmpl, w, "conciliacao.html", d)
	})

	// GET/POST/DELETE /posicao
	http.HandleFunc("/posicao", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			// ParseForm uma única vez
			if err := r.ParseForm(); err != nil {
				renderTemplate(tmpl, w, "posicao.html", posicaoData{Erro: "Erro ao processar formulário."})
				return
			}

			// DELETE via POST com _method=DELETE
			if r.FormValue("_method") == "DELETE" {
				id := r.FormValue("id")
				req, _ := http.NewRequest(http.MethodDelete, fmt.Sprintf("%s/api/v1/posicao?id=%s", apiURL, id), nil)
				resp, err := client.Do(req)
				d := posicaoData{}
				if err != nil {
					d.Erro = fmt.Sprintf("Erro ao excluir: %v", err)
				} else {
					resp.Body.Close()
					d.Mensagem = "Registro excluído com sucesso."
				}
				renderTemplate(tmpl, w, "posicao.html", d)
				return
			}

			// POST = inserir novo registro
			afiliada := r.FormValue("indicador_contraparte_afiliada") == "true"
			versao, _ := strconv.Atoi(r.FormValue("codigo_versao_conteudo"))
			if versao == 0 {
				versao = 1
			}
			valorMTM, _ := strconv.ParseFloat(r.FormValue("valor_mtm"), 64)
			principal, _ := strconv.ParseFloat(r.FormValue("principal_remanescente"), 64)

			payload := map[string]interface{}{
				"data_posicao_carteira":          r.FormValue("data_posicao_carteira"),
				"codigo_versao_conteudo":         versao,
				"codigo_identificador_boleto":    r.FormValue("codigo_identificador_boleto"),
				"descricao_veiculo":              r.FormValue("descricao_veiculo"),
				"indicador_contraparte_afiliada": afiliada,
				"valor_mtm":                      valorMTM,
				"principal_remanescente":         principal,
				"moeda_principal_remanescente":   r.FormValue("moeda_principal_remanescente"),
			}
			body, _ := json.Marshal(payload)
			resp, err := client.Post(apiURL+"/api/v1/posicao", "application/json", bytes.NewReader(body))
			d := posicaoData{}
			if err != nil {
				d.Erro = fmt.Sprintf("Erro ao inserir: %v", err)
			} else {
				defer resp.Body.Close()
				if resp.StatusCode >= 400 {
					var errResp map[string]string
					json.NewDecoder(resp.Body).Decode(&errResp)
					d.Erro = fmt.Sprintf("Erro da API: %s", errResp["erro"])
				} else {
					d.Mensagem = "Registro inserido com sucesso."
				}
			}
			renderTemplate(tmpl, w, "posicao.html", d)
			return
		}

		// GET = consultar por data
		dataParam := r.URL.Query().Get("data")
		d := posicaoData{Data: dataParam}
		if dataParam == "" {
			renderTemplate(tmpl, w, "posicao.html", d)
			return
		}

		resp, err := client.Get(fmt.Sprintf("%s/api/v1/posicao?data=%s", apiURL, dataParam))
		if err != nil {
			d.Erro = fmt.Sprintf("Erro ao buscar posições: %v", err)
			renderTemplate(tmpl, w, "posicao.html", d)
			return
		}
		defer resp.Body.Close()

		var raw []map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
			d.Erro = fmt.Sprintf("Erro ao decodificar resposta: %v", err)
			renderTemplate(tmpl, w, "posicao.html", d)
			return
		}

		// Usar o mapa Campos para exibição dinâmica, excluindo campos internos da struct
		for _, item := range raw {
			if campos, ok := item["campos"].(map[string]interface{}); ok {
				d.Posicoes = append(d.Posicoes, campos)
			} else {
				// Fallback: remover chaves internas que não devem aparecer no grid
				delete(item, "campos")
				delete(item, "id")
				delete(item, "data_posicao_carteira")
				delete(item, "codigo_versao_conteudo")
				delete(item, "codigo_identificador_boleto")
				delete(item, "descricao_veiculo")
				delete(item, "indicador_contraparte_afiliada")
				delete(item, "valor_mtm")
				delete(item, "principal_remanescente")
				delete(item, "moeda_principal_remanescente")
				d.Posicoes = append(d.Posicoes, item)
			}
		}
		renderTemplate(tmpl, w, "posicao.html", d)
	})

	// Root redirect
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/operacao", http.StatusFound)
	})

	log.Printf("Frontend SRCOff iniciado na porta %s", frontendPort)
	if err := http.ListenAndServe(":"+frontendPort, nil); err != nil {
		log.Fatalf("erro ao iniciar servidor frontend: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Utility helpers for JSON map access
// ---------------------------------------------------------------------------

func toFloat(v interface{}) float64 {
	if f, ok := v.(float64); ok {
		return f
	}
	return 0
}

func toString(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
