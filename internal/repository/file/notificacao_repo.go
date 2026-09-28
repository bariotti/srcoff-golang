package file

import (
	"context"
	"time"

	"srcoff/internal/model"
)

// NotificacaoRepo implementa NotificacaoRepository usando arquivo JSON.
type NotificacaoRepo struct {
	st *store[model.Notificacao]
}

func NewNotificacaoRepo(dir string) *NotificacaoRepo {
	return &NotificacaoRepo{st: newStore[model.Notificacao](dir, "notificacoes.json")}
}

func (r *NotificacaoRepo) Criar(_ context.Context, n model.Notificacao) (int64, error) {
	all, err := r.st.load()
	if err != nil {
		return 0, err
	}
	maxID := int64(0)
	for _, x := range all {
		if x.ID > maxID {
			maxID = x.ID
		}
	}
	n.ID = maxID + 1
	if n.CriadoEm.IsZero() {
		n.CriadoEm = time.Now()
	}
	all = append(all, n)
	return n.ID, r.st.save(all)
}

// Listar retorna as notificações mais recentes primeiro (limite opcional).
func (r *NotificacaoRepo) Listar(_ context.Context, limite int) ([]model.Notificacao, error) {
	all, err := r.st.load()
	if err != nil {
		return nil, err
	}
	// ordena por ID desc
	for i, j := 0, len(all)-1; i < j; i, j = i+1, j-1 {
		all[i], all[j] = all[j], all[i]
	}
	if limite > 0 && len(all) > limite {
		all = all[:limite]
	}
	if all == nil {
		all = []model.Notificacao{}
	}
	return all, nil
}

func (r *NotificacaoRepo) ContarNaoLidas(_ context.Context) (int, error) {
	all, err := r.st.load()
	if err != nil {
		return 0, err
	}
	n := 0
	for _, x := range all {
		if !x.Lida {
			n++
		}
	}
	return n, nil
}

func (r *NotificacaoRepo) MarcarTodasLidas(_ context.Context) error {
	all, err := r.st.load()
	if err != nil {
		return err
	}
	for i := range all {
		all[i].Lida = true
	}
	return r.st.save(all)
}
