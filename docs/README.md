# Documentação do SRCOff

Este diretório reúne a documentação do projeto. A partir de 2026-10-09 o SRCOff é
desenvolvido em **SDD (Spec Driven Development)**: a especificação é a fonte da verdade
e toda mudança de comportamento começa por ela.

## Mapa de documentos

| Documento | Papel | Quando consultar |
|-----------|-------|------------------|
| [`especificacao.md`](especificacao.md) | **Normativo.** Requisitos (`RF`/`RN`/`RNF`/`ADR`), modelo de dados, regras de negócio do motor contábil e contrato completo da API. | Antes de implementar/alterar qualquer comportamento; para saber "como DEVE funcionar". |
| [`../README.md`](../README.md) | **Guia de uso.** Passo a passo, telas, execução, estrutura do banco e proposta AWS. | Para instalar, rodar, navegar e entender o produto na prática. |

## Relação README ↔ Especificação

- O **README** descreve *como usar* e *como rodar*; a **especificação** descreve *o que o sistema deve fazer* e *por quê* (regras + decisões).
- Quando os dois falarem do mesmo assunto, a **especificação prevalece** sobre o README em caso de divergência de comportamento esperado. Se o README estiver desatualizado, corrija-o apontando para o RF correspondente.
- Rastreabilidade: cada rota/handler no código tem um comentário `// Spec: RF-xxx (docs/especificacao.md §N)`. O núcleo contábil (`internal/service/movimento_contabil_service.go`) referencia `RN-100..RN-160`.

## Fluxo de trabalho (SDD) em uma linha

> **Spec → Teste (critério de aceitação) → Código → Rastreabilidade (IDs no commit/PR).**

Detalhes e template de novos requisitos: [`especificacao.md` §0 e §13](especificacao.md).
