package handler

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"
)

// detectarColunaData descobre qual coluna do arquivo contém a data da posição.
// Prefere o nome canônico `data_posicao_carteira`; senão, a primeira coluna cujo
// nome normalizado começa com "data". Retorna "" se nenhuma for encontrada.
func detectarColunaData(colunas []string) string {
	for _, c := range colunas {
		if c == "data_posicao_carteira" {
			return c
		}
	}
	for _, c := range colunas {
		if c == "data" || strings.HasPrefix(c, "data_") || strings.Contains(c, "data") {
			return c
		}
	}
	return ""
}

// colunaExiste indica se a coluna informada está presente na lista de colunas.
func colunaExiste(colunas []string, coluna string) bool {
	for _, c := range colunas {
		if c == coluna {
			return true
		}
	}
	return false
}

// formatosData são os formatos de data reconhecidos na importação.
var formatosData = []string{
	"2006-01-02",
	"02/01/2006",
	"2006-01-02T15:04:05Z07:00",
	"2006-01-02 15:04:05",
	"02-01-2006",
	"2006/01/02",
}

// parseDataString tenta interpretar uma string como data em um dos formatos conhecidos.
func parseDataString(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	for _, f := range formatosData {
		if t, err := time.Parse(f, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// parseDataCampo converte o valor de uma célula em data. Aceita strings em vários
// formatos comuns e o número serial de data do Excel.
func parseDataCampo(v interface{}) (time.Time, error) {
	switch val := v.(type) {
	case float64:
		// Serial de data do Excel: dias desde 1899-12-30.
		if val > 0 {
			return time.Date(1899, 12, 30, 0, 0, 0, 0, time.UTC).AddDate(0, 0, int(val)), nil
		}
	case string:
		s := strings.TrimSpace(val)
		if s == "" {
			return time.Time{}, fmt.Errorf("data vazia")
		}
		if t, ok := parseDataString(s); ok {
			return t, nil
		}
		return time.Time{}, fmt.Errorf("formato de data não reconhecido: %q", s)
	}
	return time.Time{}, fmt.Errorf("valor de data inválido: %v", v)
}

// parseConfig carrega os parâmetros opcionais de parsing do CSV (vindos do padrão
// de arquivo). Campos vazios/zero significam comportamento automático.
type parseConfig struct {
	delimitador  rune   // 0 = auto-detecta
	sepDecimal   string // "" = auto
	sepMilhar    string // "" = nenhum
	formatoData  string // layout Go das datas (ex: "02/01/2006"); "" = automático (lista multi-formato)
	colunaBoleto string // nome normalizado da coluna do boleto; sempre persistida como texto
}

// parsePosicaoArquivo lê um arquivo de posição (.csv ou .xlsx) com detecção automática.
func parsePosicaoArquivo(r io.Reader, filename string) ([]map[string]interface{}, []string, error) {
	return parsePosicaoArquivoCfg(r, filename, parseConfig{})
}

// parsePosicaoArquivoCfg lê um arquivo de posição aplicando a configuração de parsing
// informada (delimitador, separador decimal e de milhar).
func parsePosicaoArquivoCfg(r io.Reader, filename string, cfg parseConfig) ([]map[string]interface{}, []string, error) {
	lower := strings.ToLower(filename)
	switch {
	case strings.HasSuffix(lower, ".xlsx") || strings.HasSuffix(lower, ".xlsm"):
		return parseXLSX(r, cfg)
	case strings.HasSuffix(lower, ".csv") || strings.HasSuffix(lower, ".txt"):
		return parseCSV(r, cfg)
	default:
		return nil, nil, fmt.Errorf("formato não suportado: use .csv ou .xlsx")
	}
}

func parseCSV(r io.Reader, cfg parseConfig) ([]map[string]interface{}, []string, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, nil, err
	}
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF}) // remove BOM UTF-8

	delim := cfg.delimitador
	if delim == 0 {
		delim = detectDelimiter(data)
	}
	reader := csv.NewReader(bytes.NewReader(data))
	reader.Comma = delim
	reader.FieldsPerRecord = -1
	reader.TrimLeadingSpace = true

	linhas, err := reader.ReadAll()
	if err != nil {
		return nil, nil, fmt.Errorf("erro ao ler CSV: %v", err)
	}
	if len(linhas) < 2 {
		return nil, nil, fmt.Errorf("CSV sem linhas de dados")
	}

	colunas := normalizarCabecalho(linhas[0])
	var registros []map[string]interface{}
	for _, linha := range linhas[1:] {
		if linhaVazia(linha) {
			continue
		}
		reg := make(map[string]interface{}, len(colunas))
		for i, col := range colunas {
			if col == "" {
				continue
			}
			var celula string
			if i < len(linha) {
				celula = strings.TrimSpace(linha[i])
			}
			reg[col] = inferirCelula(col, celula, cfg)
		}
		registros = append(registros, reg)
	}
	return registros, colunas, nil
}

func parseXLSX(r io.Reader, cfg parseConfig) ([]map[string]interface{}, []string, error) {
	f, err := excelize.OpenReader(r)
	if err != nil {
		return nil, nil, fmt.Errorf("erro ao abrir XLSX: %v", err)
	}
	defer f.Close()

	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		return nil, nil, fmt.Errorf("XLSX sem planilhas")
	}
	linhas, err := f.GetRows(sheets[0])
	if err != nil {
		return nil, nil, fmt.Errorf("erro ao ler planilha: %v", err)
	}
	if len(linhas) < 2 {
		return nil, nil, fmt.Errorf("planilha sem linhas de dados")
	}

	colunas := normalizarCabecalho(linhas[0])
	var registros []map[string]interface{}
	for _, linha := range linhas[1:] {
		if linhaVazia(linha) {
			continue
		}
		reg := make(map[string]interface{}, len(colunas))
		for i, col := range colunas {
			if col == "" {
				continue
			}
			var celula string
			if i < len(linha) {
				celula = strings.TrimSpace(linha[i])
			}
			reg[col] = inferirCelula(col, celula, cfg)
		}
		registros = append(registros, reg)
	}
	return registros, colunas, nil
}

// detectDelimiter escolhe entre ';' e ',' com base na primeira linha do CSV.
func detectDelimiter(data []byte) rune {
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)
	if sc.Scan() {
		linha := sc.Text()
		if strings.Count(linha, ";") >= strings.Count(linha, ",") && strings.Contains(linha, ";") {
			return ';'
		}
	}
	return ','
}

// normalizarCabecalho converte nomes de coluna para snake_case sem acento.
func normalizarCabecalho(cols []string) []string {
	out := make([]string, len(cols))
	vistos := map[string]int{}
	for i, c := range cols {
		nome := normalizarNome(c)
		if nome == "" {
			continue
		}
		if n, ok := vistos[nome]; ok {
			vistos[nome] = n + 1
			nome = fmt.Sprintf("%s_%d", nome, n+1)
		} else {
			vistos[nome] = 1
		}
		out[i] = nome
	}
	return out
}

func normalizarNome(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	s = removerAcentos(s)
	var b strings.Builder
	prevUnderscore := false
	for _, r := range s {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			prevUnderscore = false
		default:
			if !prevUnderscore && b.Len() > 0 {
				b.WriteRune('_')
				prevUnderscore = true
			}
		}
	}
	return strings.Trim(b.String(), "_")
}

var acentos = map[rune]rune{
	'á': 'a', 'à': 'a', 'ã': 'a', 'â': 'a', 'ä': 'a',
	'é': 'e', 'ê': 'e', 'è': 'e', 'ë': 'e',
	'í': 'i', 'î': 'i', 'ì': 'i', 'ï': 'i',
	'ó': 'o', 'õ': 'o', 'ô': 'o', 'ò': 'o', 'ö': 'o',
	'ú': 'u', 'û': 'u', 'ù': 'u', 'ü': 'u',
	'ç': 'c', 'ñ': 'n',
}

func removerAcentos(s string) string {
	var b strings.Builder
	for _, r := range s {
		if sub, ok := acentos[r]; ok {
			b.WriteRune(sub)
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// inferirCelula converte o valor de uma célula aplicando a configuração do padrão.
// A coluna do boleto configurada é SEMPRE mantida como texto (sem conversão para
// número/data), preservando zeros à esquerda e a precisão de identificadores longos;
// célula vazia vira nil. As demais colunas seguem a inferência automática.
func inferirCelula(col, celula string, cfg parseConfig) interface{} {
	if cfg.colunaBoleto != "" && col == cfg.colunaBoleto {
		if celula == "" {
			return nil
		}
		return celula
	}
	return inferirValorCfg(celula, cfg)
}

// inferirValorCfg converte o texto de uma célula para bool, float64 ou string,
// aplicando os separadores configurados no padrão de arquivo. Se os separadores
// não estiverem configurados, usa a heurística automática. Se o valor não puder ser
// convertido para número, é mantido como texto (comportamento padrão).
func inferirValorCfg(s string, cfg parseConfig) interface{} {
	if s == "" {
		return nil
	}
	switch strings.ToLower(s) {
	case "true", "verdadeiro", "sim":
		return true
	case "false", "falso", "nao", "não":
		return false
	}

	num := s
	if cfg.sepDecimal != "" || cfg.sepMilhar != "" {
		// Configuração explícita: remove o separador de milhar e normaliza o decimal para ".".
		if cfg.sepMilhar != "" {
			num = strings.ReplaceAll(num, cfg.sepMilhar, "")
		}
		if cfg.sepDecimal == "," {
			num = strings.ReplaceAll(num, ",", ".")
		}
		// sepDecimal == "." → já está no formato esperado.
	} else {
		// Heurística automática: vírgula decimal quando não houver ponto (formato BR).
		if strings.Contains(num, ",") && !strings.Contains(num, ".") {
			num = strings.ReplaceAll(num, ",", ".")
		}
	}
	if f, err := strconv.ParseFloat(num, 64); err == nil {
		return f
	}
	// Datas em qualquer coluna são normalizadas para ISO (AAAA-MM-DD), para que
	// comparações entre colunas de data (ex: data_posicao_carteira == data_efetiva)
	// funcionem independentemente do formato de origem.
	// Com formato configurado no padrão, ele é a única forma aceita (determinístico,
	// resolve a ambiguidade DD/MM vs MM/DD); sem formato, tenta a lista automática.
	if cfg.formatoData != "" {
		if t, err := time.Parse(cfg.formatoData, s); err == nil {
			return t.Format("2006-01-02")
		}
	} else if t, ok := parseDataString(s); ok {
		return t.Format("2006-01-02")
	}
	return s
}

func linhaVazia(linha []string) bool {
	for _, c := range linha {
		if strings.TrimSpace(c) != "" {
			return false
		}
	}
	return true
}
