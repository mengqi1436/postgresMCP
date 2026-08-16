package tools

import (
	"fmt"
	"strings"

	"pg-mcp/database"
)

func init() { registerAdvancedTools() }

func registerAdvancedTools() {
	RegisterTool(ToolInfo{
		Name:        "execute_sql",
		Category:    "advanced",
		Description: "执行任意 SQL，按语句前缀自动分流 SELECT/DDL/DML。参数: sql(必需), params(可选, $n 绑定值数组), limit(可选, SELECT 分支返回行数上限, 默认500), detail_level(可选, summary|detail|full; summary 只返回概览最省 token, detail 返回完整行(默认), full 提高默认行数上限到10000); 超限返回 truncated=true 与精确 total",
		Params:      []string{"sql", "params", "limit", "detail_level"},
	}, handleExecuteSQL)

	RegisterTool(ToolInfo{
		Name:        "execute_transaction",
		Category:    "advanced",
		Description: "在同一事务中执行多条语句（全成功才提交）。PostgreSQL 的 DDL 也参与事务回滚，" +
			"但 VACUUM / CREATE INDEX CONCURRENTLY 等不能在事务块中执行。参数: statements(必需, SQL 数组)",
		Params:      []string{"statements"},
	}, handleExecuteTransaction)

	RegisterTool(ToolInfo{
		Name:        "call_function",
		Category:    "advanced",
		Description: "调用函数并返回结果（SELECT func(...)）。参数: function_name(必需), params(可选, 按位置传参数组)",
		Params:      []string{"function_name", "params"},
	}, handleCallFunction)

	RegisterTool(ToolInfo{
		Name:        "call_procedure",
		Category:    "advanced",
		Description: "调用存储过程（CALL proc(...)，PG11+）。参数: procedure_name(必需), params(可选)",
		Params:      []string{"procedure_name", "params"},
	}, handleCallProcedure)

	RegisterTool(ToolInfo{
		Name:        "explain_plan",
		Category:    "advanced",
		Description: "执行计划分析（EXPLAIN FORMAT JSON）。参数: sql(必需), analyze(可选, 真实执行; 写语句自动包裹 BEGIN/ROLLBACK 不落盘), buffers(可选)",
		Params:      []string{"sql", "analyze", "buffers"},
	}, handleExplainPlan)
}

// classifySQL 按前缀粗分类语句。
func classifySQL(sqlStr string) string {
	upper := strings.ToUpper(strings.TrimSpace(sqlStr))
	switch {
	case strings.HasPrefix(upper, "SELECT"), strings.HasPrefix(upper, "WITH"),
		strings.HasPrefix(upper, "VALUES"), strings.HasPrefix(upper, "TABLE"),
		strings.HasPrefix(upper, "SHOW"), strings.HasPrefix(upper, "EXPLAIN"):
		return "query"
	case strings.HasPrefix(upper, "CREATE"), strings.HasPrefix(upper, "ALTER"),
		strings.HasPrefix(upper, "DROP"), strings.HasPrefix(upper, "TRUNCATE"),
		strings.HasPrefix(upper, "GRANT"), strings.HasPrefix(upper, "REVOKE"),
		strings.HasPrefix(upper, "COMMENT"), strings.HasPrefix(upper, "VACUUM"),
		strings.HasPrefix(upper, "ANALYZE"), strings.HasPrefix(upper, "REINDEX"):
		return "ddl"
	default:
		return "dml"
	}
}

// isWriteStatement 判断语句是否会产生数据变更（EXPLAIN ANALYZE 需要包裹回滚事务）。
func isWriteStatement(sqlStr string) bool {
	upper := strings.ToUpper(strings.TrimSpace(sqlStr))
	return strings.HasPrefix(upper, "INSERT") || strings.HasPrefix(upper, "UPDATE") ||
		strings.HasPrefix(upper, "DELETE") || strings.HasPrefix(upper, "MERGE") ||
		strings.HasPrefix(upper, "COPY") || strings.HasPrefix(upper, "TRUNCATE")
}

func handleExecuteSQL(params map[string]interface{}) (interface{}, error) {
	sqlStr := getString(params, "sql")
	if sqlStr == "" {
		return nil, fmt.Errorf("参数 sql 是必需的")
	}

	ctx, cancel := toolContext()
	defer cancel()

	args := getArray(params, "params")
	switch classifySQL(sqlStr) {
	case "query":
		detail := getDetailLevel(params)
		qr, err := database.QueryLimit(ctx, maxRowsForDetailLevel(params, detail), sqlStr, args...)
		if err != nil {
			return nil, err
		}
		payload := queryResultPayload(qr, detail)
		payload["type"] = "query"
		return payload, nil
	case "ddl":
		if len(args) == 0 {
			if err := database.ExecuteDDL(ctx, sqlStr); err != nil {
				return nil, err
			}
		} else if _, err := database.Execute(ctx, sqlStr, args...); err != nil {
			return nil, err
		}
		return map[string]interface{}{"type": "ddl", "success": true}, nil
	default:
		affected, err := database.Execute(ctx, sqlStr, args...)
		if err != nil {
			return nil, err
		}
		return map[string]interface{}{"type": "dml", "success": true, "affected_rows": affected}, nil
	}
}

func handleExecuteTransaction(params map[string]interface{}) (interface{}, error) {
	statements := getStringSlice(params, "statements")
	if len(statements) == 0 {
		return nil, fmt.Errorf("参数 statements 是必需的（SQL 数组）")
	}

	ctx, cancel := toolContext()
	defer cancel()

	if err := database.ExecuteTransaction(ctx, statements); err != nil {
		return nil, err
	}
	return map[string]interface{}{"success": true, "total": len(statements)}, nil
}

func handleCallFunction(params map[string]interface{}) (interface{}, error) {
	fnName := getString(params, "function_name")
	if fnName == "" {
		return nil, fmt.Errorf("参数 function_name 是必需的")
	}
	// 允许 schema.func
	schema, name := splitSchemaName(fnName)
	if err := validateIdentifier(name); err != nil {
		return nil, fmt.Errorf("函数名校验失败: %w", err)
	}
	if schema != "" {
		if err := validateIdentifier(schema); err != nil {
			return nil, fmt.Errorf("schema 校验失败: %w", err)
		}
	}

	args := getArray(params, "params")
	placeholders := make([]string, len(args))
	for i := range args {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
	}
	sqlStr := fmt.Sprintf("SELECT %s(%s) AS result", qualifiedTable(schema, name), strings.Join(placeholders, ", "))

	ctx, cancel := toolContext()
	defer cancel()

	qr, err := database.QueryLimit(ctx, DefaultMaxRows, sqlStr, args...)
	if err != nil {
		return nil, err
	}
	var result interface{}
	if len(qr.Rows) > 0 {
		result = qr.Rows[0]["result"]
	}
	return map[string]interface{}{
		"success": true, "result": result, "rows": qr.Rows,
		"total": qr.Total, "truncated": qr.Truncated,
	}, nil
}

func handleCallProcedure(params map[string]interface{}) (interface{}, error) {
	procName := getString(params, "procedure_name")
	if procName == "" {
		return nil, fmt.Errorf("参数 procedure_name 是必需的")
	}
	schema, name := splitSchemaName(procName)
	if err := validateIdentifier(name); err != nil {
		return nil, fmt.Errorf("过程名校验失败: %w", err)
	}
	if schema != "" {
		if err := validateIdentifier(schema); err != nil {
			return nil, fmt.Errorf("schema 校验失败: %w", err)
		}
	}

	args := getArray(params, "params")
	placeholders := make([]string, len(args))
	for i := range args {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
	}
	sqlStr := fmt.Sprintf("CALL %s(%s)", qualifiedTable(schema, name), strings.Join(placeholders, ", "))

	ctx, cancel := toolContext()
	defer cancel()

	// CALL 可能返回结果集（INOUT 参数），按查询执行（默认最多 500 行）
	qr, err := database.QueryLimit(ctx, DefaultMaxRows, sqlStr, args...)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"success": true, "rows": qr.Rows, "count": len(qr.Rows),
		"total": qr.Total, "truncated": qr.Truncated,
	}, nil
}

func handleExplainPlan(params map[string]interface{}) (interface{}, error) {
	sqlStr := strings.TrimSuffix(strings.TrimSpace(getString(params, "sql")), ";")
	if sqlStr == "" {
		return nil, fmt.Errorf("参数 sql 是必需的")
	}

	analyze := getBool(params, "analyze")
	buffers := getBool(params, "buffers")

	var opts []string
	if analyze {
		opts = append(opts, "ANALYZE")
	}
	if buffers {
		opts = append(opts, "BUFFERS")
	}
	opts = append(opts, "FORMAT JSON")

	explainSQL := fmt.Sprintf("EXPLAIN (%s) %s", strings.Join(opts, ", "), sqlStr)

	ctx, cancel := toolContext()
	defer cancel()

	var (
		results []map[string]any
		err     error
	)
	if analyze && isWriteStatement(sqlStr) {
		// 官方范式：真实执行写语句但整体回滚，不落盘
		results, err = database.QueryInAbortedTx(ctx, explainSQL)
	} else {
		results, err = database.Query(ctx, explainSQL)
	}
	if err != nil {
		return nil, err
	}

	var plan interface{}
	if len(results) > 0 {
		plan = results[0]["QUERY PLAN"]
	}
	return map[string]interface{}{"plan": plan, "analyze": analyze, "buffers": buffers}, nil
}
