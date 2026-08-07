package tools

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"pg-mcp/database"
)

func init() { registerDDLTools() }

// ddlAtomicNote 说明 PG 批量 DDL 的事务语义（与达梦 DM 的关键差异）。
const ddlAtomicNote = "PostgreSQL 的 DDL 支持事务回滚，atomic=true 时为真正的单事务全有或全无；" +
	"例外：VACUUM、CREATE INDEX CONCURRENTLY、REINDEX CONCURRENTLY 等不能在事务块中执行。"

// typePattern 校验列类型（varchar(100)/numeric(10,2)/timestamp with time zone/integer[] 等）。
var typePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_ ]*(\(\s*\d+\s*(,\s*\d+\s*)?\))?(\[\])?$`)

func registerDDLTools() {
	RegisterTool(ToolInfo{
		Name:        "create_table",
		Category:    "ddl",
		Description: "创建表。参数: table_name(必需, 可带 schema), columns(必需, [{name, type, length, primary_key, not_null, default, unique}])",
		Params:      []string{"table_name", "columns"},
	}, handleCreateTable)

	RegisterTool(ToolInfo{
		Name:        "alter_table",
		Category:    "ddl",
		Description: "修改表结构。参数: table_name, operation(ADD/MODIFY/DROP), column, type(ADD/MODIFY 时需要)",
		Params:      []string{"table_name", "operation", "column", "type"},
	}, handleAlterTable)

	RegisterTool(ToolInfo{
		Name:        "drop_table",
		Category:    "ddl",
		Description: "删除表。参数: table_name(必需), if_exists(可选)",
		Params:      []string{"table_name", "if_exists"},
	}, handleDropTable)

	RegisterTool(ToolInfo{
		Name:        "create_index",
		Category:    "ddl",
		Description: "创建索引。参数: index_name, table_name, columns(列名数组), unique(可选)",
		Params:      []string{"index_name", "table_name", "columns", "unique"},
	}, handleCreateIndex)

	RegisterTool(ToolInfo{
		Name:        "drop_index",
		Category:    "ddl",
		Description: "删除索引。参数: index_name(可为 schema.index), if_exists(可选)",
		Params:      []string{"index_name", "if_exists"},
	}, handleDropIndex)

	RegisterTool(ToolInfo{
		Name:        "execute_ddl",
		Category:    "ddl",
		Description: "执行任意 DDL 语句（CREATE/ALTER/DROP/TRUNCATE 等）。参数: sql(必需)",
		Params:      []string{"sql"},
	}, handleExecuteDDL)

	RegisterTool(ToolInfo{
		Name:        "batch_create_tables",
		Category:    "ddl",
		Description: "批量创建表。参数: tables(必需, [{table_name, columns}, ...]), atomic(可选, 默认false)。" + ddlAtomicNote,
		Params:      []string{"tables", "atomic"},
	}, handleBatchCreateTables)

	RegisterTool(ToolInfo{
		Name:        "batch_create_indexes",
		Category:    "ddl",
		Description: "批量创建索引。参数: indexes(必需, [{index_name, table_name, columns, unique}, ...]), atomic(可选, 默认false)。" + ddlAtomicNote,
		Params:      []string{"indexes", "atomic"},
	}, handleBatchCreateIndexes)

	RegisterTool(ToolInfo{
		Name:        "batch_drop_tables",
		Category:    "ddl",
		Description: "批量删除表。参数: table_names(必需, 数组), if_exists(可选), atomic(可选, 默认false)。" + ddlAtomicNote,
		Params:      []string{"table_names", "if_exists", "atomic"},
	}, handleBatchDropTables)

	RegisterTool(ToolInfo{
		Name:        "batch_drop_indexes",
		Category:    "ddl",
		Description: "批量删除索引。参数: index_names(必需, 数组，可为 schema.index), if_exists(可选), atomic(可选, 默认false)。" + ddlAtomicNote,
		Params:      []string{"index_names", "if_exists", "atomic"},
	}, handleBatchDropIndexes)
}

// buildCreateTableSQL 由结构化定义生成 CREATE TABLE 语句。
func buildCreateTableSQL(def map[string]interface{}) (string, error) {
	tableName, _ := def["table_name"].(string)
	schema, name, err := validateTable(strings.TrimSpace(tableName))
	if err != nil {
		return "", err
	}

	colsArr, _ := def["columns"].([]interface{})
	if len(colsArr) == 0 {
		return "", fmt.Errorf("表 '%s' 的 columns 不能为空", tableName)
	}

	var colDefs []string
	hasPK := false
	for _, item := range colsArr {
		col, ok := item.(map[string]interface{})
		if !ok {
			return "", fmt.Errorf("columns 中每项必须是对象")
		}
		colName, _ := col["name"].(string)
		if err := validateIdentifier(strings.TrimSpace(colName)); err != nil {
			return "", fmt.Errorf("列名校验失败: %w", err)
		}
		colType, _ := col["type"].(string)
		colType = strings.TrimSpace(colType)
		if colType == "" {
			return "", fmt.Errorf("列 '%s' 缺少 type", colName)
		}
		if !typePattern.MatchString(colType) {
			return "", fmt.Errorf("列 '%s' 的类型 '%s' 含非法字符", colName, colType)
		}

		defStr := fmt.Sprintf("%s %s", quoteIdent(colName), colType)

		if length := getIntDefault(col, "length", 0); length > 0 && !strings.Contains(colType, "(") {
			defStr = fmt.Sprintf("%s(%d)", defStr, length)
		}
		if dv, ok := col["default"]; ok && dv != nil && dv != "" {
			defStr += " DEFAULT " + defaultExpr(dv)
		}
		if isTruthy(col["not_null"]) {
			defStr += " NOT NULL"
		}
		if isTruthy(col["unique"]) {
			defStr += " UNIQUE"
		}
		if isTruthy(col["primary_key"]) {
			if hasPK {
				return "", fmt.Errorf("表 '%s' 定义了多个 primary_key 列", tableName)
			}
			defStr += " PRIMARY KEY"
			hasPK = true
		}
		colDefs = append(colDefs, defStr)
	}

	return fmt.Sprintf("CREATE TABLE %s (%s)", qualifiedTable(schema, name), strings.Join(colDefs, ", ")), nil
}

func isTruthy(v interface{}) bool {
	b, _ := v.(bool)
	return b
}

// defaultKeyword 允许裸写的 DEFAULT 关键字/函数白名单。
var defaultKeyword = map[string]bool{
	"CURRENT_TIMESTAMP": true, "CURRENT_DATE": true, "CURRENT_TIME": true,
	"LOCALTIMESTAMP": true, "NOW()": true, "GEN_RANDOM_UUID()": true,
	"NULL": true, "TRUE": true, "FALSE": true,
}

// numberLiteral 匹配数字字面量。
var numberLiteral = regexp.MustCompile(`^-?\d+(\.\d+)?$`)

// defaultExpr 安全地生成 DEFAULT 表达式：数字/白名单关键字直出，
// 其余一律按字符串字面量引用，防注入。
func defaultExpr(v interface{}) string {
	switch d := v.(type) {
	case nil:
		return "NULL"
	case bool:
		if d {
			return "TRUE"
		}
		return "FALSE"
	case float64:
		return strconv.FormatFloat(d, 'f', -1, 64)
	case string:
		s := strings.TrimSpace(d)
		if defaultKeyword[strings.ToUpper(s)] {
			return strings.ToUpper(s)
		}
		if numberLiteral.MatchString(s) {
			return s
		}
		return quoteLiteral(s)
	default:
		return quoteLiteral(fmt.Sprintf("%v", d))
	}
}

func handleCreateTable(params map[string]interface{}) (interface{}, error) {
	sqlStr, err := buildCreateTableSQL(params)
	if err != nil {
		return nil, err
	}

	ctx, cancel := toolContext()
	defer cancel()

	if err := database.ExecuteDDL(ctx, sqlStr); err != nil {
		return nil, err
	}
	return map[string]interface{}{"success": true, "sql": sqlStr}, nil
}

func handleAlterTable(params map[string]interface{}) (interface{}, error) {
	table := getString(params, "table_name")
	schema, name, err := validateTable(table)
	if err != nil {
		return nil, err
	}
	operation := strings.ToUpper(getString(params, "operation"))
	column := getString(params, "column")
	if err := validateIdentifier(column); err != nil {
		return nil, fmt.Errorf("列名校验失败: %w", err)
	}

	tbl := qualifiedTable(schema, name)
	var sqlStr string
	switch operation {
	case "ADD":
		colType := strings.TrimSpace(getString(params, "type"))
		if colType == "" || !typePattern.MatchString(colType) {
			return nil, fmt.Errorf("ADD 操作需要合法的 type 参数")
		}
		sqlStr = fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", tbl, quoteIdent(column), colType)
	case "MODIFY":
		colType := strings.TrimSpace(getString(params, "type"))
		if colType == "" || !typePattern.MatchString(colType) {
			return nil, fmt.Errorf("MODIFY 操作需要合法的 type 参数")
		}
		sqlStr = fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s TYPE %s", tbl, quoteIdent(column), colType)
	case "DROP":
		sqlStr = fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s", tbl, quoteIdent(column))
	default:
		return nil, fmt.Errorf("operation 必须是 ADD/MODIFY/DROP 之一")
	}

	ctx, cancel := toolContext()
	defer cancel()

	if err := database.ExecuteDDL(ctx, sqlStr); err != nil {
		return nil, err
	}
	return map[string]interface{}{"success": true, "sql": sqlStr}, nil
}

func handleDropTable(params map[string]interface{}) (interface{}, error) {
	table := getString(params, "table_name")
	schema, name, err := validateTable(table)
	if err != nil {
		return nil, err
	}

	sqlStr := "DROP TABLE "
	if getBool(params, "if_exists") {
		sqlStr += "IF EXISTS "
	}
	sqlStr += qualifiedTable(schema, name)

	ctx, cancel := toolContext()
	defer cancel()

	if err := database.ExecuteDDL(ctx, sqlStr); err != nil {
		return nil, err
	}
	return map[string]interface{}{"success": true, "sql": sqlStr}, nil
}

// buildCreateIndexSQL 生成 CREATE INDEX 语句。
func buildCreateIndexSQL(def map[string]interface{}) (string, error) {
	indexName, _ := def["index_name"].(string)
	idxSchema, idxName, err := validateTable(strings.TrimSpace(indexName))
	if err != nil {
		return "", fmt.Errorf("index_name 校验失败: %w", err)
	}
	tableName, _ := def["table_name"].(string)
	tblSchema, tblName, err := validateTable(strings.TrimSpace(tableName))
	if err != nil {
		return "", fmt.Errorf("table_name 校验失败: %w", err)
	}
	cols, _ := def["columns"].([]interface{})
	if len(cols) == 0 {
		return "", fmt.Errorf("索引 '%s' 的 columns 不能为空", indexName)
	}

	quoted := make([]string, len(cols))
	for i, c := range cols {
		cname, _ := c.(string)
		if err := validateIdentifier(strings.TrimSpace(cname)); err != nil {
			return "", fmt.Errorf("索引列校验失败: %w", err)
		}
		quoted[i] = quoteIdent(cname)
	}

	sqlStr := "CREATE "
	if isTruthy(def["unique"]) {
		sqlStr += "UNIQUE "
	}
	sqlStr += fmt.Sprintf("INDEX %s ON %s (%s)",
		quoteIdent(idxName), qualifiedTable(tblSchema, tblName), strings.Join(quoted, ", "))
	_ = idxSchema // PG 索引建在表所在 schema，schema 前缀仅用于校验/drop
	return sqlStr, nil
}

func handleCreateIndex(params map[string]interface{}) (interface{}, error) {
	sqlStr, err := buildCreateIndexSQL(params)
	if err != nil {
		return nil, err
	}

	ctx, cancel := toolContext()
	defer cancel()

	if err := database.ExecuteDDL(ctx, sqlStr); err != nil {
		return nil, err
	}
	return map[string]interface{}{"success": true, "sql": sqlStr}, nil
}

func handleDropIndex(params map[string]interface{}) (interface{}, error) {
	indexName := getString(params, "index_name")
	schema, name, err := validateTable(indexName)
	if err != nil {
		return nil, err
	}

	sqlStr := "DROP INDEX "
	if getBool(params, "if_exists") {
		sqlStr += "IF EXISTS "
	}
	sqlStr += qualifiedTable(schema, name)

	ctx, cancel := toolContext()
	defer cancel()

	if err := database.ExecuteDDL(ctx, sqlStr); err != nil {
		return nil, err
	}
	return map[string]interface{}{"success": true, "sql": sqlStr}, nil
}

func handleExecuteDDL(params map[string]interface{}) (interface{}, error) {
	sqlStr := getString(params, "sql")
	if sqlStr == "" {
		return nil, fmt.Errorf("参数 sql 是必需的")
	}

	ctx, cancel := toolContext()
	defer cancel()

	if err := database.ExecuteDDL(ctx, sqlStr); err != nil {
		return nil, err
	}
	return map[string]interface{}{"success": true}, nil
}

// runBatchDDL 执行批量 DDL：atomic=true 整体事务（PG DDL 可回滚），否则逐条执行。
func runBatchDDL(statements []string, atomic bool) (interface{}, error) {
	ctx, cancel := toolContext()
	defer cancel()

	if atomic {
		if err := database.ExecuteTransaction(ctx, statements); err != nil {
			return nil, err
		}
		return map[string]interface{}{
			"success": true, "total": len(statements),
			"ok_count": len(statements), "fail_count": 0,
			"atomic": true,
		}, nil
	}

	var results []map[string]interface{}
	okCount, failCount := 0, 0
	for i, stmt := range statements {
		entry := map[string]interface{}{"index": i, "sql": stmt}
		if err := database.ExecuteDDL(ctx, stmt); err != nil {
			entry["ok"] = false
			entry["error"] = err.Error()
			failCount++
		} else {
			entry["ok"] = true
			okCount++
		}
		results = append(results, entry)
	}
	return map[string]interface{}{
		"success": failCount == 0, "total": len(statements),
		"ok_count": okCount, "fail_count": failCount,
		"atomic": false, "results": results,
	}, nil
}

func handleBatchCreateTables(params map[string]interface{}) (interface{}, error) {
	arr := getArray(params, "tables")
	if len(arr) == 0 {
		return nil, fmt.Errorf("参数 tables 是必需的")
	}
	var statements []string
	for i, item := range arr {
		def, ok := item.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("tables[%d] 必须是对象", i)
		}
		sqlStr, err := buildCreateTableSQL(def)
		if err != nil {
			return nil, fmt.Errorf("tables[%d]: %w", i, err)
		}
		statements = append(statements, sqlStr)
	}
	return runBatchDDL(statements, getBool(params, "atomic"))
}

func handleBatchCreateIndexes(params map[string]interface{}) (interface{}, error) {
	arr := getArray(params, "indexes")
	if len(arr) == 0 {
		return nil, fmt.Errorf("参数 indexes 是必需的")
	}
	var statements []string
	for i, item := range arr {
		def, ok := item.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("indexes[%d] 必须是对象", i)
		}
		sqlStr, err := buildCreateIndexSQL(def)
		if err != nil {
			return nil, fmt.Errorf("indexes[%d]: %w", i, err)
		}
		statements = append(statements, sqlStr)
	}
	return runBatchDDL(statements, getBool(params, "atomic"))
}

func handleBatchDropTables(params map[string]interface{}) (interface{}, error) {
	names := getStringSlice(params, "table_names")
	if len(names) == 0 {
		return nil, fmt.Errorf("参数 table_names 是必需的")
	}
	ifExists := ""
	if getBool(params, "if_exists") {
		ifExists = "IF EXISTS "
	}
	var statements []string
	for _, t := range names {
		schema, name, err := validateTable(t)
		if err != nil {
			return nil, err
		}
		statements = append(statements, "DROP TABLE "+ifExists+qualifiedTable(schema, name))
	}
	return runBatchDDL(statements, getBool(params, "atomic"))
}

func handleBatchDropIndexes(params map[string]interface{}) (interface{}, error) {
	names := getStringSlice(params, "index_names")
	if len(names) == 0 {
		return nil, fmt.Errorf("参数 index_names 是必需的")
	}
	ifExists := ""
	if getBool(params, "if_exists") {
		ifExists = "IF EXISTS "
	}
	var statements []string
	for _, idx := range names {
		schema, name, err := validateTable(idx)
		if err != nil {
			return nil, err
		}
		statements = append(statements, "DROP INDEX "+ifExists+qualifiedTable(schema, name))
	}
	return runBatchDDL(statements, getBool(params, "atomic"))
}
