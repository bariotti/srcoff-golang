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

// configLeitura expõe a leitura de configurações necessárias ao watcher.
type configLeitura interface {
	Obter(ctx context.Context, chave string) (string, error)
}

// movimentoExecutor executa o contábil para um escopo (usado no auto-contábil).
type movimentoExecutor interface {
	GerarMovimentoEscopo(ctx context.Context, data time.Time, produto, dominio string) ([]model.ProdutoDominio, error)
}

// notificador cria notificações de eventos automáticos.
type notificador interface {
	Criar(ctx context.Context, n model.Notificacao) (int64, error)
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
	movSvc     movimentoExecutor
	notifSvc   notificador

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

func NewPastaWatcher(posicaoSvc posicaoCarteiraSvc, padraoSvc padraoArquivoResolver, configSvc configLeitura, movSvc movimentoExecutor, notifSvc notificador) *PastaWatcher {
	return &PastaWatcher{posicaoSvc: posicaoSvc, padraoSvc: padraoSvc, configSvc: configSvc, movSvc: movSvc, notifSvc: notifSvc}
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
			// Importação automática → notifica e executa o contábil automaticamente.
			pw.posImportacaoAutomatica(ctx, res)
		} else {
			log.Printf("[watcher] falha ao importar %q (mantido na pasta): %+v", e.Name(), res.Produtos)
		}
	}
	return pw.registrar(resumo), nil
}

// posImportacaoAutomatica, após uma importação automática bem-sucedida, notifica a
// importação e executa o contábil (também automaticamente) para cada (data, produto,
// domínio) importado, notificando cada execução.
func (pw *PastaWatcher) posImportacaoAutomatica(ctx context.Context, res ResultadoImportacaoArquivo) {
	for _, p := range res.Produtos {
		if p.Erro != "" {
			continue
		}
		for _, lote := range p.Lotes {
			// Notifica a importação da posição.
			pw.notificar(ctx, model.Notificacao{
				Tipo: model.NotificacaoPosicaoImportada, DataLote: lote.Data, Produto: p.Produto, Dominio: p.Dominio,
				Mensagem: fmt.Sprintf("Posição %s/%s importada automaticamente para %s (%d registro(s)).", p.Produto, p.Dominio, lote.Data, lote.Total),
			})
			// Executa o contábil automaticamente para o escopo importado.
			if pw.movSvc == nil {
				continue
			}
			data, err := time.Parse("2006-01-02", lote.Data)
			if err != nil {
				continue
			}
			bloqueios, err := pw.movSvc.GerarMovimentoEscopo(ctx, data, p.Produto, p.Dominio)
			if err != nil {
				log.Printf("[watcher] auto-contábil %s/%s %s falhou: %v", p.Produto, p.Dominio, lote.Data, err)
				pw.notificar(ctx, model.Notificacao{
					Tipo: model.NotificacaoContabilExecutado, DataLote: lote.Data, Produto: p.Produto, Dominio: p.Dominio,
					Mensagem: fmt.Sprintf("Contábil de %s/%s para %s NÃO executado: %v", p.Produto, p.Dominio, lote.Data, err),
				})
				continue
			}
			if len(bloqueios) > 0 {
				log.Printf("[watcher] auto-contábil %s/%s %s bloqueado pela obrigatoriedade de movimento de D-1", p.Produto, p.Dominio, lote.Data)
				pw.notificar(ctx, model.Notificacao{
					Tipo: model.NotificacaoContabilExecutado, DataLote: lote.Data, Produto: p.Produto, Dominio: p.Dominio,
					Mensagem: fmt.Sprintf("Contábil de %s/%s para %s NÃO executado: é necessário executar o contábil de datas anteriores (falta o movimento do dia útil anterior).", p.Produto, p.Dominio, lote.Data),
				})
				continue
			}
			pw.notificar(ctx, model.Notificacao{
				Tipo: model.NotificacaoContabilExecutado, DataLote: lote.Data, Produto: p.Produto, Dominio: p.Dominio,
				Mensagem: fmt.Sprintf("Contábil de %s/%s executado automaticamente para %s.", p.Produto, p.Dominio, lote.Data),
			})
		}
	}
}

func (pw *PastaWatcher) notificar(ctx context.Context, n model.Notificacao) {
	if pw.notifSvc == nil {
		return
	}
	if _, err := pw.notifSvc.Criar(ctx, n); err != nil {
		log.Printf("[watcher] falha ao criar notificação: %v", err)
	}
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
