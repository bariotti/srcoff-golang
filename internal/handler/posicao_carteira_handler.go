package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"srcoff/internal/model"
)

type posicaoCarteiraSvc interface {
	ListarPorData(ctx context.Context, data time.Time) ([]model.PosicaoCarteira, error)
	ListarPorPeriodo(ctx context.Context, dataInicio, dataFim time.Time) ([]model.PosicaoCarteira, error)
	Inserir(ctx context.Context, p model.PosicaoCarteira) (int64, error)
	Deletar(ctx context.Context, id int64) error
}

type PosicaoCarteiraHandler struct {
	svc posicaoCarteiraSvc
}

func NewPosicaoCarteiraHandler(svc posicaoCarteiraSvc) *PosicaoCarteiraHandler {
	return &PosicaoCarteiraHandler{svc: svc}
}

func (h *PosicaoCarteiraHandler) Listar(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	dataInicioStr := q.Get("data_inicio")
	dataFimStr := q.Get("data_fim")
	dataStr := q.Get("data") // retrocompatibilidade

	// Retrocompatibilidade com parâmetro data único
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

	// Se data_fim não informada, usa data_inicio (consulta de dia único)
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
	writeJSON(w, http.StatusOK, posicoes)
}

func (h *PosicaoCarteiraHandler) Inserir(w http.ResponseWriter, r *http.Request) {
	// DTO para receber data como string
	var dto struct {
		DataPosicaoCarteira          string  `json:"data_posicao_carteira"`
		CodigoVersaoConteudo         int     `json:"codigo_versao_conteudo"`
		CodigoIdentificadorBoleto    string  `json:"codigo_identificador_boleto"`
		DescricaoVeiculo             string  `json:"descricao_veiculo"`
		IndicadorContraparteAfiliada bool    `json:"indicador_contraparte_afiliada"`
		ValorMTM                     float64 `json:"valor_mtm"`
		PrincipalRemanescente        float64 `json:"principal_remanescente"`
		MoedaPrincipalRemanescente   string  `json:"moeda_principal_remanescente"`
		Produto                      string  `json:"produto"`
	}
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"erro": err.Error()})
		return
	}
	data, err := time.Parse("2006-01-02", dto.DataPosicaoCarteira)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"erro": "data_posicao_carteira inválida: use YYYY-MM-DD"})
		return
	}
	p := model.PosicaoCarteira{
		DataPosicaoCarteira:          data,
		CodigoVersaoConteudo:         dto.CodigoVersaoConteudo,
		CodigoIdentificadorBoleto:    dto.CodigoIdentificadorBoleto,
		DescricaoVeiculo:             dto.DescricaoVeiculo,
		IndicadorContraparteAfiliada: dto.IndicadorContraparteAfiliada,
		ValorMTM:                     dto.ValorMTM,
		PrincipalRemanescente:        dto.PrincipalRemanescente,
		MoedaPrincipalRemanescente:   dto.MoedaPrincipalRemanescente,
		Produto:                      dto.Produto,
	}
	id, err := h.svc.Inserir(r.Context(), p)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"erro": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]int64{"id": id})
}

func (h *PosicaoCarteiraHandler) Deletar(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Query().Get("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"erro": "id inválido"})
		return
	}
	if err := h.svc.Deletar(r.Context(), id); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"erro": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"mensagem": "registro excluído com sucesso"})
}
