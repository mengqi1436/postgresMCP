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
		Description: "执行 SELECT 查询。参数: sql(必需), params(可选, 对应 $1,$2... 占位符的绑定值数组), limit(可选, 返回行数上限, 默认500, 范围1-10000), detail_level(可选, summary|detail|full; summary 只返回 count/total/示例行最省 token, detail 返回完整行(默认), full 提高默认行数上限到10000); 超限返回 truncated=true 与精确 total, 可用 query_paginated 翻页",
		Params:      []string{"sql", "params", "limit", "detail_level"},
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
		Description: "执行分页查询（自动追加 LIMIT/OFFSET，返回 has_more 指示是否还有下一页）。参数: sql(必需, 不要自带 LIMIT), page(默认1), page_size(默认20, 最大1000)",
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
		Description: "批量执行多条 SELECT 查询，逐条返回结果（单条失败不中断；每条最多返回 200 行，超限标记 truncated）。参数: queries(必需, SQL 数组)",
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

	detail := getDetailLevel(params)
	args := getArray(params, "params")
	qr, err := database.QueryLimit(ctx, maxRowsForDetailLevel(params, detail), sqlStr, args...)
	if err != nil {
		return nil, err
	}

	return queryResultPayload(qr, detail), nil
}

// queryResultPayload 按 detail_level 组装查询返回结构。
// summary：只返回 count/total/truncated/sample（前几行示例），省 token；
// detail/full：返回完整 rows。
func queryResultPayload(qr *database.QueryResult, detail string) map[string]interface{} {
	if detail == "summary" {
		payload := map[string]interface{}{
			"count": len(qr.Rows), "total": qr.Total, "truncated": qr.Truncated,
			"hint": "detail_level=summary 仅返回概览与示例行；需要完整数据请用 detail_level=detail（默认）或 full",
		}
		if len(qr.Rows) > 0 {
			payload["sample"] = qr.Rows
		}
		return payload
	}
	return map[string]interface{}{
		"rows": qr.Rows, "count": len(qr.Rows),
		"total": qr.Total, "truncated": qr.Truncated,
	}
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

	// 多取一行判断是否还有下一页（不额外 COUNT 查询，避免大结果集成本）
	paged := fmt.Sprintf("%s LIMIT %d OFFSET %d", sqlStr, pageSize+1, offset)
	results, err := database.Query(ctx, paged)
	if err != nil {
		return nil, err
	}
	hasMore := len(results) > pageSize
	if hasMore {
		results = results[:pageSize]
	}

	return map[string]interface{}{
		"rows":      results,
		"count":     len(results),
		"page":      page,
		"page_size": pageSize,
		"offset":    offset,
		"has_more":  hasMore,
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
		qr, err := database.QueryLimit(ctx, DefaultBatchMaxRows, sqlStr)
		cancel()

		if err != nil {
			entry["ok"] = false
			entry["error"] = err.Error()
			failCount++
		} else {
			entry["ok"] = true
			entry["rows"] = qr.Rows
			entry["count"] = len(qr.Rows)
			entry["total"] = qr.Total
			entry["truncated"] = qr.Truncated
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
