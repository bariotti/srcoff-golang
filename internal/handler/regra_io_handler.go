package handler

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"srcoff/internal/model"
)

// cabecalhoRegrasCSV é o cabeçalho (e a ordem canônica) do CSV de exportação/importação
// de regras e condições. Cada linha representa UMA condição com os dados da sua regra-pai
// repetidos; uma regra sem condições gera uma linha com as colunas de condição vazias.
// Na importação as colunas são localizadas pelo nome do cabeçalho (ordem flexível).
var cabecalhoRegrasCSV = []string{
	"regra_id",
	"descricao",
	"codigo_produto_corporativo",
	"dominio",
	"campo_produto",
	"pre_condicao",
	"regra_ativa",
	"posta_reverte",
	"condicao_id",
	"condicao",
	"conta_debito",
	"conta_credito",
	"campo_valor",
	"campo_moeda",
	"campo_boleto",
	"condicao_ativa",
}

// ExportarRegrasCSV trata GET /api/v1/regras/export.
// Gera um único CSV (separador ";", com BOM para o Excel) contendo todas as regras
// ativas e suas condições — uma linha por condição.
func (h *RegraContabilHandler) ExportarRegrasCSV(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	regras, err := h.svc.ListarRegras(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"erro": err.Error()})
		return
	}

	var buf bytes.Buffer
	buf.Write([]byte{0xEF, 0xBB, 0xBF}) // BOM UTF-8 para o Excel reconhecer acentos
	writer := csv.NewWriter(&buf)
	writer.Comma = ';'
	writer.Write(cabecalhoRegrasCSV)

	boolStr := func(b bool) string {
		if b {
			return "Sim"
		}
		return "Não"
	}

	for _, reg := range regras {
		base := []string{
			strconv.FormatInt(reg.ID, 10),
			reg.Descricao,
			reg.CodigoProdutoCorporativo,
			reg.Dominio,
			reg.CampoProduto,
			reg.PreCondicao,
			boolStr(reg.Ativo),
			boolStr(reg.PostaReverte),
		}
		if len(reg.Condicoes) == 0 {
			// Regra sem condições: colunas de condição vazias.
			writer.Write(append(append([]string{}, base...), "", "", "", "", "", "", "", ""))
			continue
		}
		for _, c := range reg.Condicoes {
			writer.Write(append(append([]string{}, base...),
				strconv.FormatInt(c.ID, 10),
				c.Condicao,
				c.ContaDebito,
				c.ContaCredito,
				c.CampoValor,
				c.CampoMoeda,
				c.CampoBoleto,
				boolStr(c.Ativo),
			))
		}
	}
	writer.Flush()

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=\"regras_contabeis.csv\"")
	w.WriteHeader(http.StatusOK)
	w.Write(buf.Bytes())
}

// ResultadoImportacaoRegras resume o efeito de uma importação em lote.
type ResultadoImportacaoRegras struct {
	RegrasCriadas        int      `json:"regras_criadas"`
	RegrasAtualizadas    int      `json:"regras_atualizadas"`
	RegrasInalteradas    int      `json:"regras_inalteradas"`
	CondicoesCriadas     int      `json:"condicoes_criadas"`
	CondicoesAtualizadas int      `json:"condicoes_atualizadas"`
	CondicoesInalteradas int      `json:"condicoes_inalteradas"`
	Erros                []string `json:"erros,omitempty"`
}

// condLinha associa uma condição parseada à sua linha no CSV (para mensagens de erro).
type condLinha struct {
	cond     model.CondicaoRegra
	linhaNum int
}

// regraNovaCSV é uma regra a ser criada (regra_id não-numérico ou vazio) com as condições
// informadas para ela no CSV.
type regraNovaCSV struct {
	regra     model.RegraContabil
	condicoes []condLinha
	linhaNum  int
}

// importRegrasCSV é o resultado classificado do parse do CSV.
type importRegrasCSV struct {
	novas              []*regraNovaCSV       // regras novas (com suas condições novas agrupadas)
	updRegras          []model.RegraContabil // regras existentes (regra_id numérico), desduplicadas por id
	updConds           []condLinha           // condições existentes (condicao_id numérico) a avaliar
	novasCondExistente []condLinha           // condições novas (condicao_id vazio) em regra existente
}

// ImportarRegrasCSV trata POST /api/v1/regras/import (multipart, campo "arquivo").
// Cria e atualiza em lote regras e condições a partir do CSV. Convenção da coluna regra_id:
//   - número existente → atualiza a regra (e suas condições conforme condicao_id);
//   - número inexistente na base → erro (provável engano);
//   - rótulo de texto (ex.: "NOVA-1") → cria uma regra nova; linhas com o mesmo rótulo
//     viram UMA regra com várias condições;
//   - vazio → cria uma regra nova isolada (uma linha = uma regra + uma condição).
//
// Para condições: condicao_id numérico existente → atualiza (se mudou); condicao_id vazio
// com campos de condição preenchidos → cria condição (em regra existente ou nova).
// Linhas idênticas ao cadastro não são atualizadas; registros ausentes do CSV não são
// tocados (nenhuma exclusão).
func (h *RegraContabilHandler) ImportarRegrasCSV(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseMultipartForm(16 << 20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"erro": "arquivo inválido ou muito grande"})
		return
	}
	file, _, err := r.FormFile("arquivo")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"erro": "informe o arquivo no campo 'arquivo'"})
		return
	}
	defer file.Close()

	imp, err := parseRegrasCSV(file)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"erro": err.Error()})
		return
	}

	// Estado atual (regras ativas com suas condições ativas), indexado por id.
	atuais, err := h.svc.ListarRegras(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"erro": err.Error()})
		return
	}
	regraAtual := map[int64]model.RegraContabil{}
	condicaoAtual := map[int64]model.CondicaoRegra{}
	for _, reg := range atuais {
		regraAtual[reg.ID] = reg
		for _, c := range reg.Condicoes {
			condicaoAtual[c.ID] = c
		}
	}

	res := ResultadoImportacaoRegras{}

	// 1) Regras existentes: atualiza apenas as que existem e que mudaram.
	for _, nova := range imp.updRegras {
		atual, ok := regraAtual[nova.ID]
		if !ok {
			res.Erros = append(res.Erros, fmt.Sprintf("regra %d: id não existe no cadastro atual", nova.ID))
			continue
		}
		if regraIgual(atual, nova) {
			res.RegrasInalteradas++
			continue
		}
		if err := h.svc.EditarRegra(r.Context(), nova); err != nil {
			res.Erros = append(res.Erros, fmt.Sprintf("regra %d: %v", nova.ID, err))
			continue
		}
		res.RegrasAtualizadas++
	}

	// 2) Condições existentes: atualiza apenas as que existem e que mudaram.
	for _, cl := range imp.updConds {
		atual, ok := condicaoAtual[cl.cond.ID]
		if !ok {
			res.Erros = append(res.Erros, fmt.Sprintf("linha %d: condição %d não existe no cadastro atual", cl.linhaNum, cl.cond.ID))
			continue
		}
		if condicaoIgual(atual, cl.cond) {
			res.CondicoesInalteradas++
			continue
		}
		if err := h.svc.EditarCondicao(r.Context(), cl.cond); err != nil {
			res.Erros = append(res.Erros, fmt.Sprintf("linha %d: condição %d: %v", cl.linhaNum, cl.cond.ID, err))
			continue
		}
		res.CondicoesAtualizadas++
	}

	// 3) Condições novas em regra existente.
	for _, cl := range imp.novasCondExistente {
		if _, ok := regraAtual[cl.cond.IDRegra]; !ok {
			res.Erros = append(res.Erros, fmt.Sprintf("linha %d: regra %d não existe para a nova condição", cl.linhaNum, cl.cond.IDRegra))
			continue
		}
		if _, err := h.svc.CriarCondicao(r.Context(), cl.cond); err != nil {
			res.Erros = append(res.Erros, fmt.Sprintf("linha %d: nova condição: %v", cl.linhaNum, err))
			continue
		}
		res.CondicoesCriadas++
	}

	// 4) Regras novas (e suas condições novas).
	for _, g := range imp.novas {
		novoID, err := h.svc.CriarRegra(r.Context(), g.regra)
		if err != nil {
			res.Erros = append(res.Erros, fmt.Sprintf("linha %d: nova regra: %v", g.linhaNum, err))
			continue
		}
		res.RegrasCriadas++
		for _, cl := range g.condicoes {
			cl.cond.IDRegra = novoID
			if _, err := h.svc.CriarCondicao(r.Context(), cl.cond); err != nil {
				res.Erros = append(res.Erros, fmt.Sprintf("linha %d: condição da regra nova (linha %d): %v", cl.linhaNum, g.linhaNum, err))
				continue
			}
			res.CondicoesCriadas++
		}
	}

	writeJSON(w, http.StatusOK, res)
}

// temConteudoCondicao indica se a linha traz, de fato, dados de condição (além do id).
func temConteudoCondicao(condicao, cDeb, cCred, cVal, cMoe, cBol string) bool {
	return condicao != "" || cDeb != "" || cCred != "" || cVal != "" || cMoe != "" || cBol != ""
}

// parseRegrasCSV lê o CSV de regras/condições e classifica cada linha em: atualização de
// regra existente, atualização de condição existente, nova condição em regra existente, ou
// regra nova (com condições novas). Ver ImportarRegrasCSV para a convenção da coluna regra_id.
func parseRegrasCSV(r io.Reader) (importRegrasCSV, error) {
	var imp importRegrasCSV

	data, err := io.ReadAll(r)
	if err != nil {
		return imp, err
	}
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})

	reader := csv.NewReader(bytes.NewReader(data))
	reader.Comma = ';'
	reader.FieldsPerRecord = -1
	reader.TrimLeadingSpace = true

	linhas, err := reader.ReadAll()
	if err != nil {
		return imp, fmt.Errorf("erro ao ler CSV: %v", err)
	}
	if len(linhas) < 2 {
		return imp, fmt.Errorf("CSV sem linhas de dados")
	}

	idx := map[string]int{}
	for i, col := range linhas[0] {
		idx[strings.TrimSpace(strings.ToLower(col))] = i
	}
	// Colunas mínimas obrigatórias.
	for _, obrig := range []string{"regra_id", "descricao"} {
		if _, ok := idx[obrig]; !ok {
			return imp, fmt.Errorf("coluna obrigatória ausente no CSV: %q", obrig)
		}
	}
	get := func(linha []string, nome string) string {
		i, ok := idx[nome]
		if !ok || i >= len(linha) {
			return ""
		}
		return strings.TrimSpace(linha[i])
	}

	vistaRegra := map[int64]bool{}              // regras existentes já adicionadas a updRegras
	novasPorChave := map[string]*regraNovaCSV{} // agrupamento de regras novas por rótulo

	for n, linha := range linhas[1:] {
		linhaNum := n + 2 // 1-based + cabeçalho

		campos := model.RegraContabil{
			Descricao:                get(linha, "descricao"),
			CodigoProdutoCorporativo: get(linha, "codigo_produto_corporativo"),
			Dominio:                  get(linha, "dominio"),
			CampoProduto:             get(linha, "campo_produto"),
			PreCondicao:              get(linha, "pre_condicao"),
			Ativo:                    parseBoolCSV(get(linha, "regra_ativa"), true),
			PostaReverte:             parseBoolCSV(get(linha, "posta_reverte"), false),
		}

		cCond := get(linha, "condicao")
		cDeb := get(linha, "conta_debito")
		cCred := get(linha, "conta_credito")
		cVal := get(linha, "campo_valor")
		cMoe := get(linha, "campo_moeda")
		cBol := get(linha, "campo_boleto")
		condIDStr := get(linha, "condicao_id")
		temCond := condIDStr != "" || temConteudoCondicao(cCond, cDeb, cCred, cVal, cMoe, cBol)

		novaCond := func(idRegra int64) model.CondicaoRegra {
			return model.CondicaoRegra{
				IDRegra: idRegra, Condicao: cCond, ContaDebito: cDeb, ContaCredito: cCred,
				CampoValor: cVal, CampoMoeda: cMoe, CampoBoleto: cBol,
			}
		}

		rid := get(linha, "regra_id")
		regraID, errID := strconv.ParseInt(rid, 10, 64)

		if errID == nil && regraID > 0 {
			// Regra existente (regra_id numérico).
			if !vistaRegra[regraID] {
				vistaRegra[regraID] = true
				campos.ID = regraID
				imp.updRegras = append(imp.updRegras, campos)
			}
			if temCond {
				if condIDStr == "" {
					// Nova condição em regra existente.
					imp.novasCondExistente = append(imp.novasCondExistente, condLinha{cond: novaCond(regraID), linhaNum: linhaNum})
				} else {
					condID, err := strconv.ParseInt(condIDStr, 10, 64)
					if err != nil || condID == 0 {
						return imp, fmt.Errorf("linha %d: condicao_id inválido", linhaNum)
					}
					c := novaCond(regraID)
					c.ID = condID
					imp.updConds = append(imp.updConds, condLinha{cond: c, linhaNum: linhaNum})
				}
			}
			continue
		}

		if rid != "" && errID == nil {
			// regra_id numérico igual a 0 não é aceito.
			return imp, fmt.Errorf("linha %d: regra_id inválido (0)", linhaNum)
		}

		// Regra nova: regra_id vazio (isolada) ou rótulo de texto (agrupa por rótulo).
		var grp *regraNovaCSV
		if rid == "" {
			grp = &regraNovaCSV{regra: campos, linhaNum: linhaNum}
			imp.novas = append(imp.novas, grp)
		} else if g, ok := novasPorChave[rid]; ok {
			grp = g
		} else {
			grp = &regraNovaCSV{regra: campos, linhaNum: linhaNum}
			novasPorChave[rid] = grp
			imp.novas = append(imp.novas, grp)
		}
		if temCond {
			grp.condicoes = append(grp.condicoes, condLinha{cond: novaCond(0), linhaNum: linhaNum})
		}
	}
	return imp, nil
}

// parseBoolCSV interpreta valores booleanos do CSV ("Sim"/"Não", "true"/"false",
// "1"/"0"). Vazio ou desconhecido retorna o default informado.
func parseBoolCSV(s string, def bool) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "sim", "true", "verdadeiro", "1", "s":
		return true
	case "não", "nao", "false", "falso", "0", "n":
		return false
	default:
		return def
	}
}

// regraIgual indica se os campos editáveis de duas regras são idênticos.
func regraIgual(a, b model.RegraContabil) bool {
	return a.Descricao == b.Descricao &&
		a.CodigoProdutoCorporativo == b.CodigoProdutoCorporativo &&
		a.Dominio == b.Dominio &&
		a.CampoProduto == b.CampoProduto &&
		a.PreCondicao == b.PreCondicao &&
		a.Ativo == b.Ativo &&
		a.PostaReverte == b.PostaReverte
}

// condicaoIgual indica se os campos editáveis de duas condições são idênticos.
// O campo Ativo não entra na comparação: a edição de condição não altera esse estado.
func condicaoIgual(a, b model.CondicaoRegra) bool {
	return a.Condicao == b.Condicao &&
		a.ContaDebito == b.ContaDebito &&
		a.ContaCredito == b.ContaCredito &&
		a.CampoValor == b.CampoValor &&
		a.CampoMoeda == b.CampoMoeda &&
		a.CampoBoleto == b.CampoBoleto
}
