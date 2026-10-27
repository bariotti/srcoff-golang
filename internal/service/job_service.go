package service

import (
	"context"
	"fmt"
	"log"
	"time"

	"srcoff/internal/model"
	"srcoff/internal/repository"
)

// JobExecutor executa um job de um dado tipo. logf acumula mensagens no log do job.
type JobExecutor func(ctx context.Context, j *model.Job, logf func(string)) error

// JobService enfileira e processa jobs assíncronos (RF-121) com um único worker
// em segundo plano, registrando status, tentativas (retry) e log.
type JobService struct {
	repo      repository.JobRepository
	execs     map[string]JobExecutor
	intervalo time.Duration
}

func NewJobService(repo repository.JobRepository) *JobService {
	return &JobService{repo: repo, execs: map[string]JobExecutor{}, intervalo: 2 * time.Second}
}

// Registrar associa um executor a um tipo de job. Deve ser chamado antes de Start.
func (s *JobService) Registrar(tipo string, fn JobExecutor) { s.execs[tipo] = fn }

// Enfileirar cria um novo job pendente.
func (s *JobService) Enfileirar(ctx context.Context, tipo, descricao, payload string, maxTentativas int) (int64, error) {
	if tipo == "" {
		return 0, fmt.Errorf("tipo do job é obrigatório")
	}
	if maxTentativas <= 0 {
		maxTentativas = 3
	}
	return s.repo.Criar(ctx, model.Job{
		Tipo: tipo, Descricao: descricao, Payload: payload,
		Status: model.JobPendente, MaxTentativas: maxTentativas,
	})
}

func (s *JobService) Listar(ctx context.Context, status string, pagina, tamanho int) (*model.JobPagina, error) {
	return s.repo.ListarPaginado(ctx, status, pagina, tamanho)
}

func (s *JobService) Obter(ctx context.Context, id int64) (*model.Job, error) {
	return s.repo.ObterPorID(ctx, id)
}

// Retry recoloca um job com erro na fila (tentativas zeradas), preservando o log.
func (s *JobService) Retry(ctx context.Context, id int64) error {
	j, err := s.repo.ObterPorID(ctx, id)
	if err != nil {
		return err
	}
	if j == nil {
		return fmt.Errorf("job %d não encontrado", id)
	}
	if j.Status != model.JobErro {
		return fmt.Errorf("apenas jobs com erro podem ser reprocessados")
	}
	j.Status = model.JobPendente
	j.Tentativas = 0
	s.appendLog(j, "reprocessamento manual solicitado")
	return s.repo.Atualizar(ctx, *j)
}

// Start dispara o worker em segundo plano (loop de polling por jobs pendentes).
func (s *JobService) Start(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(s.intervalo)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.processarUm(ctx)
			}
		}
	}()
}

// processarUm reivindica e processa o próximo job pendente (se houver).
func (s *JobService) processarUm(ctx context.Context) {
	j, err := s.repo.ProximoPendente(ctx)
	if err != nil {
		log.Printf("[job] erro ao buscar pendente: %v", err)
		return
	}
	if j == nil {
		return
	}

	j.Status = model.JobExecutando
	j.Tentativas++
	s.appendLog(j, fmt.Sprintf("tentativa %d/%d iniciada", j.Tentativas, j.MaxTentativas))
	if err := s.repo.Atualizar(ctx, *j); err != nil {
		log.Printf("[job] erro ao marcar executando (id=%d): %v", j.ID, err)
		return
	}

	fn := s.execs[j.Tipo]
	if fn == nil {
		s.appendLog(j, "nenhum executor registrado para o tipo "+j.Tipo)
		j.Status = model.JobErro
		_ = s.repo.Atualizar(ctx, *j)
		return
	}

	logf := func(m string) { s.appendLog(j, m) }
	runErr := func() (e error) {
		defer func() {
			if r := recover(); r != nil {
				e = fmt.Errorf("panic: %v", r)
			}
		}()
		return fn(ctx, j, logf)
	}()

	if runErr != nil {
		s.appendLog(j, "ERRO: "+runErr.Error())
		if j.Tentativas >= j.MaxTentativas {
			j.Status = model.JobErro
			s.appendLog(j, "máximo de tentativas atingido")
		} else {
			j.Status = model.JobPendente
			s.appendLog(j, "reagendado para nova tentativa")
		}
	} else {
		j.Status = model.JobSucesso
		s.appendLog(j, "concluído com sucesso")
	}
	if err := s.repo.Atualizar(ctx, *j); err != nil {
		log.Printf("[job] erro ao finalizar (id=%d): %v", j.ID, err)
	}
}

func (s *JobService) appendLog(j *model.Job, m string) {
	j.Log += time.Now().Format("2006-01-02 15:04:05") + " | " + m + "\n"
}
