package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"srcoff/internal/model"
)

type PosicaoCarteiraRepo struct {
	db *sql.DB
}

func NewPosicaoCarteiraRepo(db *sql.DB) *PosicaoCarteiraRepo {
	return &PosicaoCarteiraRepo{db: db}
}

func (r *PosicaoCarteiraRepo) BuscarPorDataEVersaoMaxima(ctx context.Context, data time.Time) ([]model.PosicaoCarteira, error) {
	dataStr := data.Format("2006-01-02")

	var maxVersao int
	err := r.db.QueryRowContext(ctx,
		"SELECT ISNULL(MAX(codigo_versao_conteudo), 0) FROM posicao_carteira WHERE data_posicao_carteira = '"+dataStr+"'",
	).Scan(&maxVersao)
	if err != nil {
		return nil, err
	}

	rows, err := r.db.QueryContext(ctx,
		"SELECT * FROM posicao_carteira WHERE data_posicao_carteira = '"+dataStr+"' AND codigo_versao_conteudo = "+fmt.Sprintf("%d", maxVersao),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanPosicoes(rows)
}

func (r *PosicaoCarteiraRepo) ListarPorData(ctx context.Context, data time.Time) ([]model.PosicaoCarteira, error) {
	dataStr := data.Format("2006-01-02")
	rows, err := r.db.QueryContext(ctx,
		"SELECT * FROM posicao_carteira WHERE data_posicao_carteira = '"+dataStr+"' ORDER BY codigo_versao_conteudo, id",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanPosicoes(rows)
}

func (r *PosicaoCarteiraRepo) Inserir(ctx context.Context, p model.PosicaoCarteira) (int64, error) {
	afiliada := 0
	if p.IndicadorContraparteAfiliada {
		afiliada = 1
	}
	query := fmt.Sprintf(
		"INSERT INTO posicao_carteira (data_posicao_carteira, codigo_versao_conteudo, codigo_identificador_boleto, descricao_veiculo, indicador_contraparte_afiliada, valor_mtm, principal_remanescente, moeda_principal_remanescente) VALUES ('%s', %d, '%s', '%s', %d, %f, %f, '%s'); SELECT SCOPE_IDENTITY()",
		p.DataPosicaoCarteira.Format("2006-01-02"),
		p.CodigoVersaoConteudo,
		strings.ReplaceAll(p.CodigoIdentificadorBoleto, "'", "''"),
		strings.ReplaceAll(p.DescricaoVeiculo, "'", "''"),
		afiliada,
		p.ValorMTM,
		p.PrincipalRemanescente,
		strings.ReplaceAll(p.MoedaPrincipalRemanescente, "'", "''"),
	)
	var id int64
	err := r.db.QueryRowContext(ctx, query).Scan(&id)
	return id, err
}

func (r *PosicaoCarteiraRepo) Deletar(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx,
		"DELETE FROM posicao_carteira WHERE id = "+fmt.Sprintf("%d", id),
	)
	return err
}

// scanPosicoes lê todas as linhas usando ColumnTypes para alocar o tipo Go correto
// para cada coluna, evitando conversões ambíguas via []byte.
func scanPosicoes(rows *sql.Rows) ([]model.PosicaoCarteira, error) {
	colTypes, err := rows.ColumnTypes()
	if err != nil {
		return nil, err
	}

	var result []model.PosicaoCarteira
	for rows.Next() {
		// Alocar ponteiros tipados conforme o tipo do banco
		ptrs := make([]interface{}, len(colTypes))
		for i, ct := range colTypes {
			nullable, _ := ct.Nullable()
			ptrs[i] = allocForType(ct.DatabaseTypeName(), nullable)
		}

		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}

		campos := make(map[string]interface{}, len(colTypes))
		for i, ct := range colTypes {
			campos[strings.ToLower(ct.Name())] = deref(ptrs[i])
		}

		p := model.PosicaoCarteira{Campos: campos}
		p.ID = toInt64(campos["id"])
		p.CodigoVersaoConteudo = int(toInt64(campos["codigo_versao_conteudo"]))
		p.CodigoIdentificadorBoleto = toStr(campos["codigo_identificador_boleto"])
		p.DescricaoVeiculo = toStr(campos["descricao_veiculo"])
		p.IndicadorContraparteAfiliada = toBool(campos["indicador_contraparte_afiliada"])
		p.ValorMTM = toFloat64(campos["valor_mtm"])
		p.PrincipalRemanescente = toFloat64(campos["principal_remanescente"])
		p.MoedaPrincipalRemanescente = toStr(campos["moeda_principal_remanescente"])
		if t, ok := campos["data_posicao_carteira"].(time.Time); ok {
			p.DataPosicaoCarteira = t
		}

		result = append(result, p)
	}
	return result, rows.Err()
}

// allocForType aloca um ponteiro do tipo Go adequado para o tipo SQL Server informado.
// nullable=true usa sql.NullXxx para evitar panic em valores NULL.
func allocForType(dbType string, nullable bool) interface{} {
	switch strings.ToUpper(dbType) {
	case "BIGINT", "INT", "SMALLINT", "TINYINT":
		if nullable {
			return new(sql.NullInt64)
		}
		return new(int64)
	case "DECIMAL", "NUMERIC", "FLOAT", "REAL", "MONEY", "SMALLMONEY":
		if nullable {
			return new(sql.NullFloat64)
		}
		return new(float64)
	case "BIT":
		if nullable {
			return new(sql.NullBool)
		}
		return new(bool)
	case "DATE", "DATETIME", "DATETIME2", "SMALLDATETIME", "DATETIMEOFFSET":
		if nullable {
			return new(sql.NullTime)
		}
		return new(time.Time)
	default:
		// VARCHAR, NVARCHAR, CHAR, TEXT e qualquer outro tipo desconhecido
		if nullable {
			return new(sql.NullString)
		}
		return new(string)
	}
}

// deref extrai o valor de um ponteiro alocado por allocForType.
// Valores NULL são convertidos para zero values do tipo correspondente
// para que o avaliador de expressões não receba nil em comparações numéricas.
func deref(ptr interface{}) interface{} {
	switch v := ptr.(type) {
	case *int64:
		return float64(*v)
	case *float64:
		return *v
	case *bool:
		return *v
	case *string:
		return *v
	case *time.Time:
		return *v
	case *sql.NullInt64:
		if v.Valid {
			return float64(v.Int64)
		}
		return float64(0) // NULL numérico → 0
	case *sql.NullFloat64:
		if v.Valid {
			return v.Float64
		}
		return float64(0) // NULL numérico → 0
	case *sql.NullBool:
		if v.Valid {
			return v.Bool
		}
		return false // NULL bool → false
	case *sql.NullString:
		if v.Valid {
			return v.String
		}
		return "" // NULL string → ""
	case *sql.NullTime:
		if v.Valid {
			return v.Time
		}
		return time.Time{} // NULL time → zero time
	}
	return nil
}

func toInt64(v interface{}) int64 {
	switch val := v.(type) {
	case int64:
		return val
	case float64:
		return int64(val)
	case int:
		return int64(val)
	case string:
		var n int64
		fmt.Sscanf(val, "%d", &n)
		return n
	}
	return 0
}

func toFloat64(v interface{}) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case int64:
		return float64(val)
	case int:
		return float64(val)
	case string:
		var f float64
		fmt.Sscanf(val, "%f", &f)
		return f
	}
	return 0
}

func toStr(v interface{}) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}

func toBool(v interface{}) bool {
	switch val := v.(type) {
	case bool:
		return val
	case int64:
		return val != 0
	case float64:
		return val != 0
	case int:
		return val != 0
	}
	return false
}
