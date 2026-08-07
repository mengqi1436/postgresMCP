package tools

import (
	"fmt"
	"strings"

	"pg-mcp/database"
)

func init() { registerQueryTools() }

func registerQueryTools() {
	RegisterTool(ToolInfo{
		Name:        "query",
		Category:    "query",
		Description: "执行 SELECT 查询。参数: sql(必需), params(可选, 对应 $1,$2... 占位符的绑定值数组)",
		Params:      []string{"sql", "params"},
	}, handleQuery)

	RegisterTool(ToolInfo{
		Name:        "query_one",
		Category:    "query",
		Description: "执行查询并只返回第一条记录。参数: sql(必需), params(可选)",
		Params:      []string{"sql", "params"},
	}, handleQueryOne)

	RegisterTool(ToolInfo{
		Name:        "query_paginated",
		Category:    "query",
		Description: "执行分页查询（自动追加 LIMIT/OFFSET）。参数: sql(必需, 不要自带 LIMIT), page(默认1), page_size(默认20)",
		Params:      []string{"sql", "page", "page_size"},
	}, handleQueryPaginated)

	RegisterTool(ToolInfo{
		Name:        "count",
		Category:    "query",
		Description: "统计表记录数。参数: table(必需, 可带 schema 前缀), where(可选过滤条件)",
		Params:      []string{"table", "where"},
	}, handleCount)

	RegisterTool(ToolInfo{
		Name:        "batch_query",
		Category:    "query",
		Description: "批量执行多条 SELECT 查询，逐条返回结果（单条失败不中断）。参数: queries(必需, SQL 数组)",
		Params:      []string{"queries"},
	}, handleBatchQuery)
}

func handleQuery(params map[string]interface{}) (interface{}, error) {
	sqlStr := getString(params, "sql")
	if sqlStr == "" {
		return nil, fmt.Errorf("参数 sql 是必需的")
	}

	ctx, cancel := toolContext()
	defer cancel()

	args := getArray(params, "params")
	results, err := database.Query(ctx, sqlStr, args...)
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{"rows": results, "count": len(results)}, nil
}

func handleQueryOne(params map[string]interface{}) (interface{}, error) {
	sqlStr := strings.TrimSuffix(strings.TrimSpace(getString(params, "sql")), ";")
	if sqlStr == "" {
		return nil, fmt.Errorf("参数 sql 是必需的")
	}

	wrapped := fmt.Sprintf("SELECT * FROM ( %s ) _pg_mcp_q LIMIT 1", sqlStr)

	ctx, cancel := toolContext()
	defer cancel()

	args := getArray(params, "params")
	results, err := database.Query(ctx, wrapped, args...)
	if err != nil {
		return nil, err
	}

	if len(results) == 0 {
		return map[string]interface{}{"found": false, "row": nil}, nil
	}
	return map[string]interface{}{"found": true, "row": results[0]}, nil
}

func handleQueryPaginated(params map[string]interface{}) (interface{}, error) {
	sqlStr := strings.TrimSuffix(strings.TrimSpace(getString(params, "sql")), ";")
	if sqlStr == "" {
		return nil, fmt.Errorf("参数 sql 是必需的")
	}

	page := getIntDefault(params, "page", 1)
	pageSize := getIntDefault(params, "page_size", 20)
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 1000 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize

	ctx, cancel := toolContext()
	defer cancel()

	paged := fmt.Sprintf("%s LIMIT %d OFFSET %d", sqlStr, pageSize, offset)
	results, err := database.Query(ctx, paged)
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"rows":      results,
		"count":     len(results),
		"page":      page,
		"page_size": pageSize,
		"offset":    offset,
	}, nil
}

func handleCount(params map[string]interface{}) (interface{}, error) {
	table := getString(params, "table")
	if table == "" {
		return nil, fmt.Errorf("参数 table 是必需的")
	}
	schema, name := splitSchemaName(table)
	if err := validateIdentifier(name); err != nil {
		return nil, err
	}
	if schema != "" {
		if err := validateIdentifier(schema); err != nil {
			return nil, err
		}
	}

	sqlStr := fmt.Sprintf("SELECT COUNT(*) AS cnt FROM %s", qualifiedTable(schema, name))
	if where := getString(params, "where"); where != "" {
		sqlStr += " WHERE " + where
	}

	ctx, cancel := toolContext()
	defer cancel()

	results, err := database.Query(ctx, sqlStr)
	if err != nil {
		return nil, err
	}

	var count interface{}
	if len(results) > 0 {
		count = results[0]["cnt"]
	}
	return map[string]interface{}{"table": table, "count": count}, nil
}

func handleBatchQuery(params map[string]interface{}) (interface{}, error) {
	arr := getArray(params, "queries")
	if len(arr) == 0 {
		return nil, fmt.Errorf("参数 queries 是必需的（SQL 数组）")
	}

	var results []map[string]interface{}
	okCount, failCount := 0, 0

	for i, item := range arr {
		sqlStr, ok := item.(string)
		entry := map[string]interface{}{"index": i, "sql": sqlStr}
		if !ok || strings.TrimSpace(sqlStr) == "" {
			entry["ok"] = false
			entry["error"] = "查询语句为空或不是字符串"
			failCount++
			results = append(results, entry)
			continue
		}

		ctx, cancel := toolContext()
		rows, err := database.Query(ctx, sqlStr)
		cancel()

		if err != nil {
			entry["ok"] = false
			entry["error"] = err.Error()
			failCount++
		} else {
			entry["ok"] = true
			entry["rows"] = rows
			entry["count"] = len(rows)
			okCount++
		}
		results = append(results, entry)
	}

	return map[string]interface{}{
		"success":    failCount == 0,
		"total":      len(arr),
		"ok_count":   okCount,
		"fail_count": failCount,
		"results":    results,
	}, nil
}
