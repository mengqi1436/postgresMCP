package tools

import (
	"fmt"

	"pg-mcp/database"
)

func init() { registerBatchOpsTools() }

func registerBatchOpsTools() {
	RegisterTool(ToolInfo{
		Name:        "batch_execute_sql",
		Category:    "advanced",
		Description: "批量执行多条任意 SQL。参数: statements(必需, SQL 数组), atomic(可选, 默认false; true 时整体事务全有或全无——PG 的 DDL 也可回滚)",
		Params:      []string{"statements", "atomic"},
	}, handleBatchExecuteSQL)
}

func handleBatchExecuteSQL(params map[string]interface{}) (interface{}, error) {
	statements := getStringSlice(params, "statements")
	if len(statements) == 0 {
		return nil, fmt.Errorf("参数 statements 是必需的（SQL 数组）")
	}

	ctx, cancel := toolContext()
	defer cancel()

	if getBool(params, "atomic") {
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
		switch classifySQL(stmt) {
		case "query":
			rows, err := database.Query(ctx, stmt)
			if err != nil {
				entry["ok"] = false
				entry["error"] = err.Error()
				failCount++
			} else {
				entry["ok"] = true
				entry["type"] = "query"
				entry["count"] = len(rows)
				okCount++
			}
		default:
			affected, err := database.Execute(ctx, stmt)
			if err != nil {
				entry["ok"] = false
				entry["error"] = err.Error()
				failCount++
			} else {
				entry["ok"] = true
				entry["type"] = classifySQL(stmt)
				entry["affected_rows"] = affected
				okCount++
			}
		}
		results = append(results, entry)
	}

	return map[string]interface{}{
		"success": failCount == 0, "total": len(statements),
		"ok_count": okCount, "fail_count": failCount,
		"atomic": false, "results": results,
	}, nil
}
