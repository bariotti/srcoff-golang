package handler

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"srcoff/internal/model"
	"srcoff/internal/service"
)

type posicaoCarteiraSvc interface {
	ListarPorData(ctx context.Context, data time.Time) ([]model.PosicaoCarteira, error)
	ListarPorPeriodo(ctx context.Context, dataInicio, dataFim time.Time) ([]model.PosicaoCarteira, error)
	ImportarArquivo(ctx context.Context, registros []map[string]interface{}) ([]service.LoteImportado, error)
	CamposDisponiveis(ctx context.Context, data time.Time) ([]string, error)
	ResolverCampoData(ctx context.Context, produto string) (string, error)
}

type PosicaoCarteiraHandler struct {
	svc       posicaoCarteiraSvc
	padraoSvc padraoArquivoResolver
}

func NewPosicaoCarteiraHandler(svc posicaoCarteiraSvc, padraoSvc padraoArquivoResolver) *PosicaoCarteiraHandler {
	return &PosicaoCarteiraHandler{svc: svc, padraoSvc: padraoSvc}
}

func (h *PosicaoCarteiraHandler) Listar(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	dataInicioStr := q.Get("data_inicio")
	dataFimStr := q.Get("data_fim")
	dataStr := q.Get("data") // retrocompatibilidade
	produto := strings.TrimSpace(q.Get("produto"))
	dominio := strings.TrimSpace(q.Get("dominio"))

	// Produto é obrigatório para consultar a posição.
	if produto == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"erro": "informe o produto para consultar a posição"})
		return
	}

	// Domínio é obrigatório para consultar a posição.
	if dominio == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"erro": "informe o domínio para consultar a posição"})
		return
	}

	if dataStr != "" && dataInicioStr == "" {
		dataInicioStr = dataStr
	}

	if dataInicioStr == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"erro": "informe data_inicio"})
		return
	}

	dataInicio, err := time.Parse("2006-01-02", dataInicioStr)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"erro": "data_inicio inválida"})
		return
	}

	dataFim := dataInicio
	if dataFimStr != "" {
		dataFim, err = time.Parse("2006-01-02", dataFimStr)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"erro": "data_fim inválida"})
			return
		}
	}

	var posicoes []model.PosicaoCarteira
	if dataFim.Equal(dataInicio) {
		posicoes, err = h.svc.ListarPorData(r.Context(), dataInicio)
	} else {
		posicoes, err = h.svc.ListarPorPeriodo(r.Context(), dataInicio, dataFim)
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"erro": err.Error()})
		return
	}

	// Filtrar pelo produto e domínio informados.
	filtradas := make([]model.PosicaoCarteira, 0, len(posicoes))
	for _, p := range posicoes {
		pv, okP := p.Campos["produto"].(string)
		dv, okD := p.Campos["dominio"].(string)
		if okP && pv == produto && okD && dv == dominio {
			filtradas = append(filtradas, p)
		}
	}

	// Paginação (idêntica à consulta de movimento contábil).
	pagina := 1
	if v, err := strconv.Atoi(q.Get("pagina")); err == nil && v > 0 {
		pagina = v
	}
	tamanho := 100
	if v, err := strconv.Atoi(q.Get("tamanho")); err == nil && v > 0 {
		tamanho = v
	}
	total := len(filtradas)
	offset := (pagina - 1) * tamanho
	registros := []model.PosicaoCarteira{}
	if offset < total {
		end := offset + tamanho
		if end > total {
			end = total
		}
		registros = filtradas[offset:end]
	}
	writeJSON(w, http.StatusOK, model.PaginaPosicoes{
		Total: total, Pagina: pagina, Tamanho: tamanho, Registros: registros,
	})
}

// Campos trata GET /api/v1/posicao/campos?data=YYYY-MM-DD
// Retorna os nomes de campos disponíveis na posição da data — usado para sugerir
// os campos no cadastro de regras/condições, sem exigir conhecimento da estrutura.
func (h *PosicaoCarteiraHandler) Campos(w http.ResponseWriter, r *http.Request) {
	dataStr := r.URL.Query().Get("data")
	if dataStr == "" {
		writeJSON(w, http.StatusOK, []string{})
		return
	}
	data, err := time.Parse("2006-01-02", dataStr)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"erro": "data inválida: use YYYY-MM-DD"})
		return
	}
	campos, err := h.svc.CamposDisponiveis(r.Context(), data)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"erro": err.Error()})
		return
	}
	if campos == nil {
		campos = []string{}
	}
	writeJSON(w, http.StatusOK, campos)
}

// Upload trata POST /api/v1/posicao/upload (multipart/form-data)
// Campos: produto (nome do produto da posição), arquivo (.csv | .xlsx), preview (opcional "1").
// A data de cada registro vem de uma coluna do próprio arquivo. O produto informado
// é persistido em cada registro (campo `produto`). Com preview=1, apenas faz o parse.
func (h *PosicaoCarteiraHandler) Upload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"erro": "arquivo inválido ou muito grande"})
		return
	}

	preview := r.FormValue("preview") == "1"
	produto := strings.TrimSpace(r.FormValue("produto"))
	dominio := strings.TrimSpace(r.FormValue("dominio"))
	if !preview && produto == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"erro": "informe o produto da posição"})
		return
	}
	if !preview && dominio == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"erro": "informe o domínio da posição"})
		return
	}

	file, header, err := r.FormFile("arquivo")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"erro": "informe o arquivo no campo 'arquivo'"})
		return
	}
	defer file.Close()

	registros, colunas, err := parsePosicaoArquivo(file, header.Filename)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"erro": err.Error()})
		return
	}
	if len(registros) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"erro": "arquivo sem registros de dados"})
		return
	}

	if preview {
		// Resolve a coluna de data apenas para exibir na pré-visualização.
		colunaData := ""
		if produto != "" {
			if c, _ := h.svc.ResolverCampoData(r.Context(), produto); c != "" {
				colunaData = normalizarNome(c)
			}
		}
		if colunaData == "" {
			colunaData = detectarColunaData(colunas)
		}
		amostra := registros
		if len(amostra) > 10 {
			amostra = amostra[:10]
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"preview":     true,
			"colunas":     colunas,
			"coluna_data": colunaData,
			"total":       len(registros),
			"amostra":     amostra,
		})
		return
	}

	lotes, err := importarConteudoParaProduto(r.Context(), h.svc, registros, colunas, produto, dominio)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"erro": err.Error()})
		return
	}
	total := 0
	for _, l := range lotes {
		total += l.Total
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"mensagem": "posição importada com sucesso",
		"produto":  produto,
		"dominio":  dominio,
		"total":    total,
		"lotes":    lotes,
	})
}

// UploadLote trata POST /api/v1/posicao/upload-lote (multipart, vários arquivos no
// campo "arquivos"). Para cada arquivo, o produto é resolvido pelo nome via padrões
// cadastrados; se casar com mais de um padrão, a posição é importada para cada produto.
func (h *PosicaoCarteiraHandler) UploadLote(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseMultipartForm(64 << 20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"erro": "arquivos inválidos ou muito grandes"})
		return
	}
	arquivos := r.MultipartForm.File["arquivos"]
	if len(arquivos) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"erro": "envie ao menos um arquivo no campo 'arquivos'"})
		return
	}

	resultados := make([]ResultadoImportacaoArquivo, 0, len(arquivos))
	for _, fh := range arquivos {
		f, err := fh.Open()
		if err != nil {
			resultados = append(resultados, ResultadoImportacaoArquivo{
				Arquivo:  fh.Filename,
				Produtos: []ResultadoImportacaoProduto{{Erro: "não foi possível abrir o arquivo"}},
			})
			continue
		}
		resultados = append(resultados, importarArquivoPorPadrao(r.Context(), h.svc, h.padraoSvc, f, fh.Filename))
		f.Close()
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"resultados": resultados})
}
