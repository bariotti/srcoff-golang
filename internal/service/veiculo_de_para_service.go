package service

import (
	"context"
	"fmt"
	"strings"

	"srcoff/internal/model"
	"srcoff/internal/repository"
)

// VeiculoDeParaService gerencia o cadastro De/Para de nome do veículo → código (RF-035).
// A comparação de nome é case-insensitive e ignora espaços nas pontas.
type VeiculoDeParaService struct {
	repo repository.VeiculoDeParaRepository
}

func NewVeiculoDeParaService(repo repository.VeiculoDeParaRepository) *VeiculoDeParaService {
	return &VeiculoDeParaService{repo: repo}
}

// chaveVeiculo normaliza o nome do veículo para comparação (case-insensitive, sem espaços nas pontas).
func chaveVeiculo(nome string) string { return strings.ToLower(strings.TrimSpace(nome)) }

func (s *VeiculoDeParaService) Listar(ctx context.Context) ([]model.VeiculoDePara, error) {
	return s.repo.Listar(ctx)
}

// Criar valida e insere um De/Para. Nome e código são obrigatórios e não pode haver
// mais de um cadastro para o mesmo nome de veículo (comparação case-insensitive).
func (s *VeiculoDeParaService) Criar(ctx context.Context, v model.VeiculoDePara) (int64, error) {
	v.Veiculo = strings.TrimSpace(v.Veiculo)
	v.Codigo = strings.TrimSpace(v.Codigo)
	if v.Veiculo == "" {
		return 0, fmt.Errorf("informe o nome do veículo")
	}
	if v.Codigo == "" {
		return 0, fmt.Errorf("informe o código do veículo")
	}
	existentes, err := s.repo.Listar(ctx)
	if err != nil {
		return 0, err
	}
	alvo := chaveVeiculo(v.Veiculo)
	for _, e := range existentes {
		if chaveVeiculo(e.Veiculo) == alvo {
			return 0, fmt.Errorf("já existe um De/Para cadastrado para o veículo %q", e.Veiculo)
		}
	}
	return s.repo.Criar(ctx, v)
}

func (s *VeiculoDeParaService) Excluir(ctx context.Context, id int64) error {
	if id == 0 {
		return fmt.Errorf("id inválido")
	}
	return s.repo.Excluir(ctx, id)
}

// MapaVeiculoCodigo devolve o De/Para como mapa chave-normalizada(nome) → código, para a
// extração CSV do movimento. Em caso de erro, devolve mapa vazio (a extração cai no nome).
func (s *VeiculoDeParaService) MapaVeiculoCodigo(ctx context.Context) map[string]string {
	m := map[string]string{}
	itens, err := s.repo.Listar(ctx)
	if err != nil {
		return m
	}
	for _, i := range itens {
		m[chaveVeiculo(i.Veiculo)] = i.Codigo
	}
	return m
}
