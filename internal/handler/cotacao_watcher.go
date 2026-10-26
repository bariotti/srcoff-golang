package handler

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"srcoff/internal/model"
	"srcoff/internal/service"
)

// cotacaoCriador insere cotações (usado na importação automática).
type cotacaoCriador interface {
	Criar(ctx context.Context, c model.Cotacao) (int64, error)
}

// CotacaoWatcher monitora periodicamente uma pasta parametrizada, importando os
// arquivos CSV de cotação e movendo-os para a pasta de processados. Espelha o
// PastaWatcher da posição; reusa o intervalo de varredura configurado.
type CotacaoWatcher struct {
	cotacaoSvc cotacaoCriador
	configSvc  configLeitura

	mu              sync.Mutex
	ultimaVarredura time.Time
	ultimoResumo    ResumoVarreduraCotacao
}

// ResumoVarreduraCotacao descreve o resultado de uma varredura da pasta de cotações.
type ResumoVarreduraCotacao struct {
	PastaMonitorada  string                       `json:"pasta_monitorada"`
	PastaProcessados string                       `json:"pasta_processados"`
	Executada        bool                         `json:"executada"`
	Motivo           string                       `json:"motivo,omitempty"`
	Arquivos         []ResultadoImportacaoCotacao `json:"arquivos"`
	Movidos          int                          `json:"movidos"`
	UltimaVarredura  string                       `json:"ultima_varredura,omitempty"`
}

// ResultadoImportacaoCotacao descreve a importação de um arquivo de cotação.
type ResultadoImportacaoCotacao struct {
	Arquivo   string `json:"arquivo"`
	Inseridas int    `json:"inseridas"`
	Erro      string `json:"erro,omitempty"`
}

func NewCotacaoWatcher(cotacaoSvc cotacaoCriador, configSvc configLeitura) *CotacaoWatcher {
	return &CotacaoWatcher{cotacaoSvc: cotacaoSvc, configSvc: configSvc}
}

func (cw *CotacaoWatcher) intervaloConfigurado(ctx context.Context) time.Duration {
	v, _ := cw.configSvc.Obter(ctx, service.ConfigIntervaloScanMinutos)
	v = strings.TrimSpace(v)
	if v == "" {
		return 0
	}
	min, err := strconv.Atoi(v)
	if err != nil || min <= 0 {
		return 0
	}
	return time.Duration(min) * time.Minute
}

// Start dispara a varredura periódica em segundo plano (mesmo intervalo da posição).
func (cw *CotacaoWatcher) Start(ctx context.Context) {
	go func() {
		for {
			intervalo := cw.intervaloConfigurado(ctx)
			if intervalo <= 0 {
				select {
				case <-ctx.Done():
					return
				case <-time.After(baseRecheck):
					continue
				}
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(intervalo):
				if _, err := cw.Scan(ctx); err != nil {
					log.Printf("[watcher-cotacao] erro na varredura: %v", err)
				}
			}
		}
	}()
}

// Scan executa uma varredura imediata e retorna o resumo.
func (cw *CotacaoWatcher) Scan(ctx context.Context) (ResumoVarreduraCotacao, error) {
	monitorada, _ := cw.configSvc.Obter(ctx, service.ConfigPastaCotacaoMonitorada)
	processados, _ := cw.configSvc.Obter(ctx, service.ConfigPastaCotacaoProcessados)
	resumo := ResumoVarreduraCotacao{PastaMonitorada: monitorada, PastaProcessados: processados, Arquivos: []ResultadoImportacaoCotacao{}}

	if monitorada == "" {
		resumo.Motivo = "pasta monitorada não configurada"
		return cw.registrar(resumo), nil
	}
	if processados == "" {
		resumo.Motivo = "pasta de processados não configurada"
		return cw.registrar(resumo), nil
	}
	info, err := os.Stat(monitorada)
	if err != nil || !info.IsDir() {
		resumo.Motivo = "pasta monitorada inexistente ou inacessível"
		return cw.registrar(resumo), nil
	}
	if err := os.MkdirAll(processados, 0755); err != nil {
		resumo.Motivo = "não foi possível criar/abrir a pasta de processados: " + err.Error()
		return cw.registrar(resumo), nil
	}

	entradas, err := os.ReadDir(monitorada)
	if err != nil {
		return resumo, err
	}
	resumo.Executada = true

	for _, e := range entradas {
		if e.IsDir() {
			continue
		}
		nome := e.Name()
		if !strings.HasSuffix(strings.ToLower(nome), ".csv") {
			continue // apenas .csv; outros arquivos permanecem intactos
		}
		caminho := filepath.Join(monitorada, nome)
		f, err := os.Open(caminho)
		if err != nil {
			continue
		}
		itens, perr := parseCotacoesCSV(f)
		f.Close()

		res := ResultadoImportacaoCotacao{Arquivo: nome}
		if perr != nil {
			res.Erro = perr.Error()
			resumo.Arquivos = append(resumo.Arquivos, res)
			log.Printf("[watcher-cotacao] falha ao ler %q (mantido): %v", nome, perr)
			continue
		}
		falhou := false
		for i, c := range itens {
			if _, err := cw.cotacaoSvc.Criar(ctx, c); err != nil {
				res.Erro = fmt.Sprintf("linha %d: %v", i+1, err)
				falhou = true
				break
			}
			res.Inseridas++
		}
		resumo.Arquivos = append(resumo.Arquivos, res)
		if falhou {
			log.Printf("[watcher-cotacao] falha ao importar %q (mantido): %s", nome, res.Erro)
			continue
		}
		if err := moverArquivo(caminho, filepath.Join(processados, nome)); err != nil {
			log.Printf("[watcher-cotacao] importado mas falha ao mover %q: %v", nome, err)
		} else {
			resumo.Movidos++
		}
	}
	return cw.registrar(resumo), nil
}

func (cw *CotacaoWatcher) UltimoResumo() ResumoVarreduraCotacao {
	cw.mu.Lock()
	defer cw.mu.Unlock()
	return cw.ultimoResumo
}

func (cw *CotacaoWatcher) registrar(r ResumoVarreduraCotacao) ResumoVarreduraCotacao {
	cw.mu.Lock()
	defer cw.mu.Unlock()
	cw.ultimaVarredura = time.Now()
	r.UltimaVarredura = cw.ultimaVarredura.Format("2006-01-02 15:04:05")
	cw.ultimoResumo = r
	return r
}

// ScanAgora trata POST /api/v1/cotacoes/scan-pasta — varredura imediata.
func (cw *CotacaoWatcher) ScanAgora(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	resumo, err := cw.Scan(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"erro": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, resumo)
}

// Status trata GET /api/v1/cotacoes/scan-pasta — resumo da última varredura.
func (cw *CotacaoWatcher) Status(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, cw.UltimoResumo())
}
