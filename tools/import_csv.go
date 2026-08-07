package tools

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"pg-mcp/database"
)

func init() { registerImportTools() }

func registerImportTools() {
	RegisterTool(ToolInfo{
		Name:        "export_table_data",
		Category:    "import",
		Description: "导出表数据为 JSON 或 INSERT 语句。参数: table_name(必需), format(insert|json, 默认json), where(可选), limit(默认1000)",
		Params:      []string{"table_name", "format", "where", "limit"},
	}, handleExportTableData)

	RegisterTool(ToolInfo{
		Name:        "batch_import_csv",
		Category:    "import",
		Description: "批量并行导入 CSV 文件（PG 原生 COPY 协议流式推送，服务器端解析类型，无需外部工具）。" +
			"参数: files(必需, [{csv_file, table, schema, columns, delimiter, header}, ...]), max_parallel(可选, 默认2), timeout_seconds(可选, 默认300)",
		Params:      []string{"files", "max_parallel", "timeout_seconds"},
	}, handleBatchImportCSV)
}

func handleExportTableData(params map[string]interface{}) (interface{}, error) {
	tableName := getString(params, "table_name")
	schema, name, err := validateTable(tableName)
	if err != nil {
		return nil, err
	}
	format := strings.ToLower(getString(params, "format"))
	if format == "" {
		format = "json"
	}
	if format != "json" && format != "insert" {
		return nil, fmt.Errorf("format 必须是 insert 或 json")
	}
	limit := getIntDefault(params, "limit", 1000)
	if limit < 1 || limit > 100000 {
		limit = 1000
	}

	sqlStr := fmt.Sprintf("SELECT * FROM %s", qualifiedTable(schema, name))
	if where := getString(params, "where"); where != "" {
		sqlStr += " WHERE " + where
	}
	sqlStr += fmt.Sprintf(" LIMIT %d", limit)

	ctx, cancel := toolContext()
	defer cancel()

	rows, err := database.Query(ctx, sqlStr)
	if err != nil {
		return nil, err
	}

	if format == "json" {
		return map[string]interface{}{
			"table": tableName, "format": "json",
			"rows": rows, "count": len(rows),
		}, nil
	}

	// INSERT 语句导出
	var stmts []string
	for _, row := range rows {
		cols := sortedKeys(row)
		quoted := make([]string, len(cols))
		values := make([]string, len(cols))
		for i, c := range cols {
			quoted[i] = quoteIdent(c)
			values[i] = valueToLiteral(row[c])
		}
		stmts = append(stmts, fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s);",
			qualifiedTable(schema, name), strings.Join(quoted, ", "), strings.Join(values, ", ")))
	}
	return map[string]interface{}{
		"table": tableName, "format": "insert",
		"statements": stmts, "count": len(stmts),
	}, nil
}

// valueToLiteral 把通用行值转成 SQL 字面量（INSERT 导出用）。
func valueToLiteral(v interface{}) string {
	switch x := v.(type) {
	case nil:
		return "NULL"
	case bool:
		if x {
			return "TRUE"
		}
		return "FALSE"
	case float64, int, int64, float32:
		return fmt.Sprintf("%v", x)
	case string:
		// numeric 已被解码为字符串，无法与文本区分——统一按字面量引用，
		// PG 会按目标列类型做隐式转换
		return quoteLiteral(x)
	default:
		return quoteLiteral(fmt.Sprintf("%v", x))
	}
}

// importCSVResult 单个文件导入结果。
type importCSVResult struct {
	Index      int                    `json:"index"`
	CSVFile    string                 `json:"csv_file"`
	Table      string                 `json:"table"`
	Status     string                 `json:"status"`
	RowsCopied int64                  `json:"rows_copied"`
	DurationMS int64                  `json:"duration_ms"`
	Error      string                 `json:"error,omitempty"`
	Extra      map[string]interface{} `json:"extra,omitempty"`
}

func handleBatchImportCSV(params map[string]interface{}) (interface{}, error) {
	arr := getArray(params, "files")
	if len(arr) == 0 {
		return nil, fmt.Errorf("参数 files 是必需的（[{csv_file, table, ...}, ...]）")
	}
	maxParallel := getIntDefault(params, "max_parallel", 2)
	if maxParallel < 1 {
		maxParallel = 1
	}
	if maxParallel > 8 {
		maxParallel = 8
	}
	timeoutSecs := getIntDefault(params, "timeout_seconds", 300)

	// 预解析全部文件定义，提前暴露参数错误
	type fileDef struct {
		csvFile   string
		schema    string
		table     string
		columns   []string
		delimiter string
		header    bool
	}
	defs := make([]fileDef, 0, len(arr))
	for i, item := range arr {
		f, ok := item.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("files[%d] 必须是对象", i)
		}
		def := fileDef{
			csvFile:   getString(f, "csv_file"),
			table:     getString(f, "table"),
			schema:    getString(f, "schema"),
			columns:   getStringSlice(f, "columns"),
			delimiter: getString(f, "delimiter"),
			header:    getBool(f, "header"),
		}
		if def.csvFile == "" || def.table == "" {
			return nil, fmt.Errorf("files[%d] 需要 csv_file 和 table", i)
		}
		if def.delimiter == "" {
			def.delimiter = ","
		}
		if _, _, err := validateTable(def.table); err != nil {
			return nil, fmt.Errorf("files[%d]: %w", i, err)
		}
		for _, c := range def.columns {
			if err := validateIdentifier(c); err != nil {
				return nil, fmt.Errorf("files[%d] 列名 '%s' 校验失败: %w", i, c, err)
			}
		}
		defs = append(defs, def)
	}

	results := make([]importCSVResult, len(defs))
	sem := make(chan struct{}, maxParallel)
	var wg sync.WaitGroup

	for i, def := range defs {
		wg.Add(1)
		go func(i int, def fileDef) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			start := time.Now()
			rowsCopied, err := importSingleCSV(def.csvFile, def.schema, def.table, def.columns, def.delimiter, def.header, timeoutSecs)

			r := importCSVResult{
				Index:      i,
				CSVFile:    def.csvFile,
				Table:      def.table,
				DurationMS: time.Since(start).Milliseconds(),
			}
			if err != nil {
				r.Status = "error"
				r.Error = err.Error()
			} else {
				r.Status = "ok"
				r.RowsCopied = rowsCopied
			}
			results[i] = r
		}(i, def)
	}
	wg.Wait()

	okCount := 0
	for _, r := range results {
		if r.Status == "ok" {
			okCount++
		}
	}
	return map[string]interface{}{
		"success": okCount == len(results),
		"total":   len(results), "ok_count": okCount, "fail_count": len(results) - okCount,
		"results": results,
	}, nil
}

// importSingleCSV 用 pgconn.CopyFrom 把 CSV 字节流直接推给服务器，
// 由服务器按 COPY ... WITH (FORMAT csv) 解析（原生类型转换，等价 \copy）。
func importSingleCSV(csvFile, schema, table string, columns []string, delimiter string, header bool, timeoutSecs int) (int64, error) {
	f, err := os.Open(csvFile)
	if err != nil {
		return 0, fmt.Errorf("打开 CSV 失败: %w", err)
	}
	defer f.Close()

	schemaName, tableName := splitSchemaName(table)
	if schema == "" {
		schema = schemaName
	}

	var opts []string
	opts = append(opts, "FORMAT csv")
	if header {
		opts = append(opts, "HEADER true")
	}
	if delimiter != "," {
		opts = append(opts, fmt.Sprintf("DELIMITER %s", quoteLiteral(delimiter)))
	}

	colList := ""
	if len(columns) > 0 {
		quoted := make([]string, len(columns))
		for i, c := range columns {
			quoted[i] = quoteIdent(c)
		}
		colList = " (" + strings.Join(quoted, ", ") + ")"
	}

	copySQL := fmt.Sprintf("COPY %s%s FROM STDIN WITH (%s)",
		qualifiedTable(schema, tableName), colList, strings.Join(opts, ", "))

	ctx, cancel := toolContextWithTimeout(time.Duration(timeoutSecs) * time.Second)
	defer cancel()

	return database.CopyFromReader(ctx, f, copySQL)
}
