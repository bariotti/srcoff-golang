package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"srcoff/internal/model"
)

// ReprocessarSeletivo reprocessa o contábil de um escopo restrito (produto e/ou domínio e,
// opcionalmente, um boleto) para a data, SEM refazer o dia inteiro (RF-137). Como a versão é por
// combinação (produto, domínio), o reprocessamento é no nível do combo e REAPROVEITA a geração
// padrão (GerarMovimentoEscopo), que já cuida de estorno, incremental, saldo e trava de
// fechamento. Com boleto informado, resolve os combos em que ele aparece na posição da data e
// reprocessa apenas esses.
func (s *MovimentoContabilService) ReprocessarSeletivo(ctx context.Context, data time.Time, produto, dominio, boleto string) (*model.ReprocessamentoResultado, error) {
	produto, dominio, boleto = strings.TrimSpace(produto), strings.TrimSpace(dominio), strings.TrimSpace(boleto)
	res := &model.ReprocessamentoResultado{
		Data: data.Format("2006-01-02"), Produto: produto, Dominio: dominio, Boleto: boleto,
		CombosProcessados: []model.ReprocessamentoCombo{}, Bloqueios: []model.ProdutoDominio{},
	}

	// Determina os combos a reprocessar.
	var combos []model.ProdutoDominio
	if boleto == "" {
		combos = []model.ProdutoDominio{{Produto: produto, Dominio: dominio}} // escopo direto (produto/dominio podem ser vazios = todos)
	} else {
		posicoes, err := s.posicaoRepo.BuscarPorDataEVersaoMaxima(ctx, data)
		if err != nil {
			return nil, fmt.Errorf("erro ao buscar posição: %w", err)
		}
		vistos := map[string]bool{}
		for _, p := range posicoes {
			prod := campoStrModel(p, "produto")
			dom := campoStrModel(p, "dominio")
			if produto != "" && prod != produto {
				continue
			}
			if dominio != "" && dom != dominio {
				continue
			}
			if campoStrModel(p, "codigo_identificador_boleto") != boleto {
				continue
			}
			k := prod + "\x00" + dom
			if vistos[k] {
				continue
			}
			vistos[k] = true
			combos = append(combos, model.ProdutoDominio{Produto: prod, Dominio: dom})
		}
		if len(combos) == 0 {
			return nil, fmt.Errorf("boleto %q não encontrado na posição de %s no escopo informado", boleto, res.Data)
		}
	}

	// Reprocessa cada combo reaproveitando a geração padrão.
	bloqMotivo := map[string]string{}
	bloqueado := map[string]bool{}
	for _, c := range combos {
		bloqueios, err := s.GerarMovimentoEscopo(ctx, data, c.Produto, c.Dominio)
		if err != nil {
			return nil, fmt.Errorf("erro ao reprocessar %s: %w", escopoRotulo(c.Produto, c.Dominio), err)
		}
		for _, b := range bloqueios {
			k := b.Produto + "\x00" + b.Dominio
			if !bloqueado[k] {
				bloqueado[k] = true
				bloqMotivo[k] = b.Motivo
				res.Bloqueios = append(res.Bloqueios, b)
			}
		}
	}
	for _, c := range combos {
		k := c.Produto + "\x00" + c.Dominio
		res.CombosProcessados = append(res.CombosProcessados, model.ReprocessamentoCombo{
			Produto: c.Produto, Dominio: c.Dominio, Bloqueado: bloqueado[k], Motivo: bloqMotivo[k], Lancamentos: -1,
		})
	}

	ok := len(res.CombosProcessados) - len(res.Bloqueios)
	if boleto != "" {
		res.Mensagem = fmt.Sprintf("Boleto %s reprocessado em %d combinação(ões) (%d bloqueada(s)).", boleto, ok, len(res.Bloqueios))
	} else {
		res.Mensagem = fmt.Sprintf("Reprocessado %s em %s (%d bloqueada(s)).", escopoRotulo(produto, dominio), res.Data, len(res.Bloqueios))
	}

	alvo := res.Data + " · " + escopoRotulo(produto, dominio)
	if boleto != "" {
		alvo += " · boleto " + boleto
	}
	s.auditoria.Log(ctx, model.AudGeracao, "movimento.reprocessado", alvo, "sistema", res.Mensagem,
		map[string]interface{}{"data": res.Data, "produto": produto, "dominio": dominio, "boleto": boleto, "combos": len(combos)})
	return res, nil
}
