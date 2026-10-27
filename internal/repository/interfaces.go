package repository

import (
	"context"
	"time"

	"srcoff/internal/model"
)

// PosicaoCarteiraRepository define o contrato de acesso à posição de carteira.
type PosicaoCarteiraRepository interface {
	BuscarPorDataEVersaoMaxima(ctx context.Context, data time.Time) ([]model.PosicaoCarteira, error)
	ListarPorData(ctx context.Context, data time.Time) ([]model.PosicaoCarteira, error)
	ListarPorPeriodo(ctx context.Context, dataInicio, dataFim time.Time) ([]model.PosicaoCarteira, error)
	ImportarLote(ctx context.Context, data time.Time, versao int, registros []map[string]interface{}) error
}

// RegraContabilRepository define o contrato de acesso às regras contábeis.
type RegraContabilRepository interface {
	ListarRegrasAtivas(ctx context.Context) ([]model.RegraContabil, error)
	CriarRegra(ctx context.Context, regra model.RegraContabil) (int64, error)
	EditarRegra(ctx context.Context, regra model.RegraContabil) error
	ExcluirRegra(ctx context.Context, id int64) error
	ListarCondicoes(ctx context.Context, idRegra int64) ([]model.CondicaoRegra, error)
	CriarCondicao(ctx context.Context, condicao model.CondicaoRegra) (int64, error)
	EditarCondicao(ctx context.Context, condicao model.CondicaoRegra) error
	ExcluirCondicao(ctx context.Context, id int64) error
}

// JobRepository persiste a fila de jobs assíncronos (RF-121): criação, atualização
// de status/log, listagem paginada e reivindicação do próximo pendente.
type JobRepository interface {
	Criar(ctx context.Context, j model.Job) (int64, error)
	Atualizar(ctx context.Context, j model.Job) error
	ObterPorID(ctx context.Context, id int64) (*model.Job, error)
	ProximoPendente(ctx context.Context) (*model.Job, error)
	ListarPaginado(ctx context.Context, status string, pagina, tamanho int) (*model.JobPagina, error)
}

// InconsistenciaRepository persiste as inconsistências detectadas na geração do
// movimento contábil, chaveadas pela data do lote.
type InconsistenciaRepository interface {
	SubstituirPorEscopo(ctx context.Context, data time.Time, combos []model.ProdutoDominio, itens []model.InconsistenciaProcessamento) error
	ListarPorData(ctx context.Context, data time.Time) ([]model.InconsistenciaProcessamento, error)
}

// ParametrizacaoRepository define o contrato de acesso às opções parametrizáveis
// (ex: valores possíveis para os combos de Produto e Domínio).
type ParametrizacaoRepository interface {
	ListarOpcoes(ctx context.Context, categoria string) ([]string, error)
	AdicionarOpcao(ctx context.Context, categoria, valor string) error
	RemoverOpcao(ctx context.Context, categoria, valor string) error
}

// PadraoArquivoRepository mapeia padrões de nome de arquivo a produtos.
type PadraoArquivoRepository interface {
	Listar(ctx context.Context) ([]model.PadraoArquivo, error)
	Criar(ctx context.Context, p model.PadraoArquivo) (int64, error)
	Excluir(ctx context.Context, id int64) error
}

// VeiculoDeParaRepository persiste o De/Para de nome do veículo → código (RF-035).
type VeiculoDeParaRepository interface {
	Listar(ctx context.Context) ([]model.VeiculoDePara, error)
	Criar(ctx context.Context, v model.VeiculoDePara) (int64, error)
	Excluir(ctx context.Context, id int64) error
}

// CotacaoRepository persiste as taxas de conversão entre moedas por data (RF-037).
type CotacaoRepository interface {
	Listar(ctx context.Context) ([]model.Cotacao, error)
	Criar(ctx context.Context, c model.Cotacao) (int64, error)
	Excluir(ctx context.Context, id int64) error
}

// ConfiguracaoRepository persiste configurações chave→valor (ex: pastas monitoradas).
type ConfiguracaoRepository interface {
	Obter(ctx context.Context, chave string) (string, error)
	Definir(ctx context.Context, chave, valor string) error
	ListarTodas(ctx context.Context) (map[string]string, error)
}

// ExecucaoRepository persiste o log de execução do contábil por (data, produto, domínio).
type ExecucaoRepository interface {
	RegistrarExecucao(ctx context.Context, e model.MovimentoExecucao) error
	ListarPorData(ctx context.Context, data time.Time) ([]model.MovimentoExecucao, error)
	// DatasExecutadas retorna as datas distintas com contábil executado para o par
	// (produto, domínio), em ordem crescente.
	DatasExecutadas(ctx context.Context, produto, dominio string) ([]time.Time, error)
	// DatasComMovimento retorna as datas distintas que efetivamente geraram movimento
	// contábil (qtd_lancamentos > 0) para o par (produto, domínio). Execuções que não
	// geraram lançamentos (ex.: barradas por inconsistências) não entram.
	DatasComMovimento(ctx context.Context, produto, dominio string) ([]time.Time, error)
}

// NotificacaoRepository persiste as notificações de eventos automáticos.
type NotificacaoRepository interface {
	Criar(ctx context.Context, n model.Notificacao) (int64, error)
	Listar(ctx context.Context, limite int) ([]model.Notificacao, error)
	ContarNaoLidas(ctx context.Context) (int, error)
	MarcarTodasLidas(ctx context.Context) error
}

// SaldoContaRepository persiste a projeção materializada de saldo por chave
// (produto, domínio, boleto, id_regra, contas). É um cache derivado do movimento
// contábil (RN-170); nunca é fonte da verdade.
type SaldoContaRepository interface {
	// AplicarDeltas soma (upsert) os deltas aos saldos das chaves informadas.
	AplicarDeltas(ctx context.Context, deltas []model.SaldoConta) error
	// SubstituirCombos zera e regrava os saldos das combinações informadas (usado na
	// recomputação após exclusão e na reconstrução).
	SubstituirCombos(ctx context.Context, combos []model.ProdutoDominio, saldos []model.SaldoConta) error
	// Listar retorna os saldos conforme o filtro (sem paginação — usado pela baixa).
	Listar(ctx context.Context, filtro model.SaldoFiltro) ([]model.SaldoConta, error)
	// ListarPaginado retorna uma página de saldos conforme o filtro (consulta do menu).
	ListarPaginado(ctx context.Context, filtro model.SaldoFiltro, pagina, tamanho int) (*model.SaldoPagina, error)
	// EstaVazio informa se a tabela ainda não tem nenhum registro (base recém-criada).
	EstaVazio(ctx context.Context) (bool, error)
}

// MovimentoContabilRepository define o contrato de acesso ao movimento contábil.
type MovimentoContabilRepository interface {
	BulkInsert(ctx context.Context, lancamentos []model.LancamentoContabil) error
	BuscarPorDataEIndicador(ctx context.Context, data time.Time, indicadorReversao bool) ([]model.LancamentoContabil, error)
	ObterProximaVersao(ctx context.Context, data time.Time) (int, error)
	ObterVersaoAtual(ctx context.Context, data time.Time) (int, error)
	ConsultarPaginado(ctx context.Context, data time.Time, pagina, tamanho int) (*model.PaginaLancamentos, error)
	ConsultarPaginadoFiltrado(ctx context.Context, f model.FiltroMovimento) (*model.PaginaLancamentos, error)
	ConsultarPaginadoFiltradoSemCancelados(ctx context.Context, f model.FiltroMovimento) (*model.PaginaLancamentos, error)
	ExcluirPorDataEVersao(ctx context.Context, data time.Time, versao int) error
}
