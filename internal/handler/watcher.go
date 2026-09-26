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

	"srcoff/internal/service"
)

// configLeitura expõe a leitura de configurações necessárias ao watcher.
type configLeitura interface {
	Obter(ctx context.Context, chave string) (string, error)
}

// PastaWatcher monitora periodicamente uma pasta parametrizada, importando os
// arquivos de posição cujo nome casa com um padrão cadastrado. Arquivos sem padrão
// correspondente permanecem intactos; os importados são movidos para a pasta de
// processados (também parametrizada).
// baseRecheck é o período de reavaliação da configuração quando o intervalo não
// está parametrizado (permite ativar o monitoramento sem reiniciar a aplicação).
const baseRecheck = 30 * time.Second

type PastaWatcher struct {
	posicaoSvc posicaoCarteiraSvc
	padraoSvc  padraoArquivoResolver
	configSvc  configLeitura

	mu              sync.Mutex
	ultimaVarredura time.Time
	ultimoResumo    ResumoVarredura
}

// ResumoVarredura descreve o resultado de uma varredura da pasta.
type ResumoVarredura struct {
	PastaMonitorada  string                       `json:"pasta_monitorada"`
	PastaProcessados string                       `json:"pasta_processados"`
	Executada        bool                         `json:"executada"`
	Motivo           string                       `json:"motivo,omitempty"`
	Arquivos         []ResultadoImportacaoArquivo `json:"arquivos"`
	Movidos          int                          `json:"movidos"`
	UltimaVarredura  string                       `json:"ultima_varredura,omitempty"`
}

func NewPastaWatcher(posicaoSvc posicaoCarteiraSvc, padraoSvc padraoArquivoResolver, configSvc configLeitura) *PastaWatcher {
	return &PastaWatcher{posicaoSvc: posicaoSvc, padraoSvc: padraoSvc, configSvc: configSvc}
}

// intervaloConfigurado lê o intervalo de varredura (em minutos) da configuração.
// Retorna 0 quando não parametrizado, vazio ou inválido — nesse caso não há
// varredura automática.
func (pw *PastaWatcher) intervaloConfigurado(ctx context.Context) time.Duration {
	v, _ := pw.configSvc.Obter(ctx, service.ConfigIntervaloScanMinutos)
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

// Start dispara a varredura periódica em segundo plano. O intervalo (em minutos)
// vem da configuração e é relido a cada ciclo; se não estiver parametrizado, o
// monitoramento fica inativo (nenhuma varredura automática é feita).
func (pw *PastaWatcher) Start(ctx context.Context) {
	go func() {
		for {
			intervalo := pw.intervaloConfigurado(ctx)
			if intervalo <= 0 {
				// Não parametrizado: aguarda um período base e reavalia (permite ativar sem reiniciar).
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
				if _, err := pw.Scan(ctx); err != nil {
					log.Printf("[watcher] erro na varredura: %v", err)
				}
			}
		}
	}()
}

// Scan executa uma varredura imediata e retorna o resumo.
func (pw *PastaWatcher) Scan(ctx context.Context) (ResumoVarredura, error) {
	monitorada, _ := pw.configSvc.Obter(ctx, service.ConfigPastaMonitorada)
	processados, _ := pw.configSvc.Obter(ctx, service.ConfigPastaProcessados)

	resumo := ResumoVarredura{PastaMonitorada: monitorada, PastaProcessados: processados, Arquivos: []ResultadoImportacaoArquivo{}}

	if monitorada == "" {
		resumo.Motivo = "pasta monitorada não configurada"
		return pw.registrar(resumo), nil
	}
	if processados == "" {
		resumo.Motivo = "pasta de processados não configurada"
		return pw.registrar(resumo), nil
	}
	info, err := os.Stat(monitorada)
	if err != nil || !info.IsDir() {
		resumo.Motivo = "pasta monitorada inexistente ou inacessível"
		return pw.registrar(resumo), nil
	}
	if err := os.MkdirAll(processados, 0755); err != nil {
		resumo.Motivo = "não foi possível criar/abrir a pasta de processados: " + err.Error()
		return pw.registrar(resumo), nil
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
		caminho := filepath.Join(monitorada, e.Name())
		f, err := os.Open(caminho)
		if err != nil {
			continue
		}
		res := importarArquivoPorPadrao(ctx, pw.posicaoSvc, pw.padraoSvc, f, e.Name())
		f.Close()

		if res.SemPadrao {
			// Arquivo sem padrão correspondente permanece intacto.
			continue
		}
		resumo.Arquivos = append(resumo.Arquivos, res)
		if res.sucesso() {
			if err := moverArquivo(caminho, filepath.Join(processados, e.Name())); err != nil {
				log.Printf("[watcher] importado mas falha ao mover %q: %v", e.Name(), err)
			} else {
				resumo.Movidos++
			}
		} else {
			log.Printf("[watcher] falha ao importar %q (mantido na pasta): %+v", e.Name(), res.Produtos)
		}
	}
	return pw.registrar(resumo), nil
}

// UltimoResumo retorna o resumo da última varredura (para status na UI).
func (pw *PastaWatcher) UltimoResumo() ResumoVarredura {
	pw.mu.Lock()
	defer pw.mu.Unlock()
	return pw.ultimoResumo
}

func (pw *PastaWatcher) registrar(r ResumoVarredura) ResumoVarredura {
	pw.mu.Lock()
	defer pw.mu.Unlock()
	pw.ultimaVarredura = time.Now()
	r.UltimaVarredura = pw.ultimaVarredura.Format("2006-01-02 15:04:05")
	pw.ultimoResumo = r
	return r
}

// ScanAgora trata POST /api/v1/posicao/scan-pasta — dispara uma varredura imediata.
func (pw *PastaWatcher) ScanAgora(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	resumo, err := pw.Scan(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"erro": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, resumo)
}

// Status trata GET /api/v1/posicao/scan-pasta — retorna o resumo da última varredura.
func (pw *PastaWatcher) Status(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, pw.UltimoResumo())
}

// moverArquivo move origem→destino; em caso de colisão de nome, anexa um sufixo.
func moverArquivo(origem, destino string) error {
	if _, err := os.Stat(destino); err == nil {
		ext := filepath.Ext(destino)
		base := destino[:len(destino)-len(ext)]
		destino = fmt.Sprintf("%s_%d%s", base, time.Now().Unix(), ext)
	}
	return os.Rename(origem, destino)
}
