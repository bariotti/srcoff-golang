package service

import (
	"context"
	"testing"

	"srcoff/internal/model"
	filerepo "srcoff/internal/repository/file"
)

// TestVeiculoDePara_RegrasCadastro valida obrigatoriedade de nome/código, unicidade
// case-insensitive do nome e o mapa usado na extração CSV. Spec: RF-035/RN-181.
func TestVeiculoDePara_RegrasCadastro(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	svc := NewVeiculoDeParaService(filerepo.NewVeiculoDeParaRepo(dir))

	// Nome, código e moeda base obrigatórios.
	if _, err := svc.Criar(ctx, model.VeiculoDePara{Veiculo: "", Codigo: "001", MoedaBase: "USD"}); err == nil {
		t.Fatal("esperava erro: nome obrigatório")
	}
	if _, err := svc.Criar(ctx, model.VeiculoDePara{Veiculo: "NASSAU", Codigo: "", MoedaBase: "USD"}); err == nil {
		t.Fatal("esperava erro: código obrigatório")
	}
	if _, err := svc.Criar(ctx, model.VeiculoDePara{Veiculo: "NASSAU", Codigo: "001", MoedaBase: ""}); err == nil {
		t.Fatal("esperava erro: moeda base obrigatória")
	}

	// Primeiro cadastro OK.
	if _, err := svc.Criar(ctx, model.VeiculoDePara{Veiculo: "NASSAU", Codigo: "001", MoedaBase: "USD"}); err != nil {
		t.Fatalf("criar NASSAU: %v", err)
	}
	// Duplicado case-insensitive deve ser rejeitado (nassau / Nassau == NASSAU).
	if _, err := svc.Criar(ctx, model.VeiculoDePara{Veiculo: "nassau", Codigo: "002", MoedaBase: "USD"}); err == nil {
		t.Fatal("esperava erro: duplicado case-insensitive (nassau)")
	}
	if _, err := svc.Criar(ctx, model.VeiculoDePara{Veiculo: "  Nassau ", Codigo: "003", MoedaBase: "USD"}); err == nil {
		t.Fatal("esperava erro: duplicado com espaços/caixa (Nassau)")
	}

	// Mapa veículo → moeda base.
	mb, _ := svc.MapaVeiculoMoedaBase(ctx)
	if mb["nassau"] != "USD" {
		t.Fatalf("mapa moeda base deveria ter nassau=USD, obteve %q", mb["nassau"])
	}

	// Mapa: chave normalizada (lower+trim) → código.
	m := svc.MapaVeiculoCodigo(ctx)
	if m["nassau"] != "001" {
		t.Fatalf("mapa deveria ter nassau=001, obteve %q", m["nassau"])
	}
}
