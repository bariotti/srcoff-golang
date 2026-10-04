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
}

// NotificacaoRepository persiste as notificações de eventos automáticos.
type NotificacaoRepository interface {
	Criar(ctx context.Context, n model.Notificacao) (int64, error)
	Listar(ctx context.Context, limite int) ([]model.Notificacao, error)
	ContarNaoLidas(ctx context.Context) (int, error)
	MarcarTodasLidas(ctx context.Context) error
}

// MovimentoContabilRepository define o contrato de acesso ao movimento contábil.
type MovimentoContabilRepository interface {
	BulkInsert(ctx context.Context, lancamentos []model.LancamentoContabil) error
	BuscarPorDataEIndicador(ctx context.Context, data time.Time, indicadorReversao bool) ([]model.LancamentoContabil, error)
	ObterProximaVersao(ctx context.Context, data time.Time) (int, error)
	ObterVersaoAtual(ctx context.Context, data time.Time) (int, error)
	ConsultarPaginado(ctx context.Context, data time.Time, pagina, tamanho int) (*model.PaginaLancamentos, error)
	ConsultarPaginadoFiltrado(ctx context.Context, dataInicio, dataFim time.Time, boleto string, versao int, versaoModo string, pagina, tamanho int) (*model.PaginaLancamentos, error)
	ConsultarPaginadoFiltradoSemCancelados(ctx context.Context, dataInicio, dataFim time.Time, boleto string, versao int, versaoModo string, pagina, tamanho int) (*model.PaginaLancamentos, error)
	ExcluirPorDataEVersao(ctx context.Context, data time.Time, versao int) error
}
