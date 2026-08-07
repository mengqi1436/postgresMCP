package tools

import (
	"fmt"
	"sort"
	"strings"

	"pg-mcp/database"
)

func init() { registerExecuteTools() }

func registerExecuteTools() {
	RegisterTool(ToolInfo{
		Name:        "insert",
		Category:    "dml",
		Description: "插入一条数据（值参数化）。参数: table(必需), data(必需, {字段:值} 对象)",
		Params:      []string{"table", "data"},
	}, handleInsert)

	RegisterTool(ToolInfo{
		Name:        "insert_batch",
		Category:    "dml",
		Description: "批量插入多条数据（单条多行 INSERT，值参数化）。参数: table(必需), rows(必需, [{字段:值},...] 数组)",
		Params:      []string{"table", "rows"},
	}, handleInsertBatch)

	RegisterTool(ToolInfo{
		Name:        "update",
		Category:    "dml",
		Description: "更新数据（SET 值参数化，WHERE 必填防误更新）。参数: table(必需), data(必需, {字段:新值}), where(必需)",
		Params:      []string{"table", "data", "where"},
	}, handleUpdate)

	RegisterTool(ToolInfo{
		Name:        "delete",
		Category:    "dml",
		Description: "删除数据（WHERE 必填防误删）。参数: table(必需), where(必需)",
		Params:      []string{"table", "where"},
	}, handleDelete)

	RegisterTool(ToolInfo{
		Name:        "merge",
		Category:    "dml",
		Description: "Upsert：INSERT ... ON CONFLICT (match_columns) DO UPDATE。参数: table(必需), data(必需), match_columns(必需, 冲突判定列数组，需有唯一约束/索引)",
		Params:      []string{"table", "data", "match_columns"},
	}, handleMerge)

	RegisterTool(ToolInfo{
		Name:        "batch_update",
		Category:    "dml",
		Description: "批量更新（多条不同 WHERE），事务包装全成功才提交。参数: table(必需), updates(必需, [{data:{字段:值}, where:条件}, ...])",
		Params:      []string{"table", "updates"},
	}, handleBatchUpdate)

	RegisterTool(ToolInfo{
		Name:        "batch_delete",
		Category:    "dml",
		Description: "批量删除（多条不同 WHERE 条件），事务包装全成功才提交。参数: table(必需), wheres(必需, WHERE 条件数组)",
		Params:      []string{"table", "wheres"},
	}, handleBatchDelete)
}

// validateTable 校验（可带 schema 的）表名并返回 schema/name。
func validateTable(table string) (string, string, error) {
	if table == "" {
		return "", "", fmt.Errorf("参数 table 是必需的")
	}
	schema, name := splitSchemaName(table)
	if err := validateIdentifier(name); err != nil {
		return "", "", fmt.Errorf("表名校验失败: %w", err)
	}
	if schema != "" {
		if err := validateIdentifier(schema); err != nil {
			return "", "", fmt.Errorf("schema 校验失败: %w", err)
		}
	}
	return schema, name, nil
}

// sortedKeys 返回对象键的排序列表（保证生成 SQL 的确定性）。
func sortedKeys(data map[string]interface{}) []string {
	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func handleInsert(params map[string]interface{}) (interface{}, error) {
	table := getString(params, "table")
	schema, name, err := validateTable(table)
	if err != nil {
		return nil, err
	}
	data := getObject(params, "data")
	if len(data) == 0 {
		return nil, fmt.Errorf("参数 data 是必需的（非空对象）")
	}

	cols := sortedKeys(data)
	quoted := make([]string, len(cols))
	placeholders := make([]string, len(cols))
	values := make([]interface{}, len(cols))
	for i, c := range cols {
		if err := validateIdentifier(c); err != nil {
			return nil, fmt.Errorf("列名 '%s' 校验失败: %w", c, err)
		}
		quoted[i] = quoteIdent(c)
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		values[i] = data[c]
	}

	sqlStr := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
		qualifiedTable(schema, name), strings.Join(quoted, ", "), strings.Join(placeholders, ", "))

	ctx, cancel := toolContext()
	defer cancel()

	affected, err := database.Execute(ctx, sqlStr, values...)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{"success": true, "affected_rows": affected}, nil
}

func handleInsertBatch(params map[string]interface{}) (interface{}, error) {
	table := getString(params, "table")
	schema, name, err := validateTable(table)
	if err != nil {
		return nil, err
	}
	arr := getArray(params, "rows")
	if len(arr) == 0 {
		return nil, fmt.Errorf("参数 rows 是必需的（对象数组）")
	}

	// 以第一行的键集合作为列集合
	first, ok := arr[0].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("rows[0] 必须是对象")
	}
	cols := sortedKeys(first)
	if len(cols) == 0 {
		return nil, fmt.Errorf("rows[0] 不能为空对象")
	}
	for _, c := range cols {
		if err := validateIdentifier(c); err != nil {
			return nil, fmt.Errorf("列名 '%s' 校验失败: %w", c, err)
		}
	}

	quoted := make([]string, len(cols))
	for i, c := range cols {
		quoted[i] = quoteIdent(c)
	}

	var valueClauses []string
	var args []interface{}
	idx := 1
	for i, item := range arr {
		row, ok := item.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("rows[%d] 必须是对象", i)
		}
		placeholders := make([]string, len(cols))
		for j, c := range cols {
			placeholders[j] = fmt.Sprintf("$%d", idx)
			args = append(args, row[c]) // 缺失列插入 NULL
			idx++
		}
		valueClauses = append(valueClauses, "("+strings.Join(placeholders, ", ")+")")
	}

	sqlStr := fmt.Sprintf("INSERT INTO %s (%s) VALUES %s",
		qualifiedTable(schema, name), strings.Join(quoted, ", "), strings.Join(valueClauses, ", "))

	ctx, cancel := toolContext()
	defer cancel()

	affected, err := database.Execute(ctx, sqlStr, args...)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"success":       true,
		"inserted_rows": affected,
		"submitted":     len(arr),
	}, nil
}

func handleUpdate(params map[string]interface{}) (interface{}, error) {
	table := getString(params, "table")
	schema, name, err := validateTable(table)
	if err != nil {
		return nil, err
	}
	data := getObject(params, "data")
	if len(data) == 0 {
		return nil, fmt.Errorf("参数 data 是必需的（非空对象）")
	}
	where := getString(params, "where")
	if where == "" {
		return nil, fmt.Errorf("参数 where 是必需的（防止全表误更新）")
	}

	cols := sortedKeys(data)
	setClauses := make([]string, len(cols))
	values := make([]interface{}, len(cols))
	for i, c := range cols {
		if err := validateIdentifier(c); err != nil {
			return nil, fmt.Errorf("列名 '%s' 校验失败: %w", c, err)
		}
		setClauses[i] = fmt.Sprintf("%s = $%d", quoteIdent(c), i+1)
		values[i] = data[c]
	}

	sqlStr := fmt.Sprintf("UPDATE %s SET %s WHERE %s",
		qualifiedTable(schema, name), strings.Join(setClauses, ", "), where)

	ctx, cancel := toolContext()
	defer cancel()

	affected, err := database.Execute(ctx, sqlStr, values...)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{"success": true, "affected_rows": affected}, nil
}

func handleDelete(params map[string]interface{}) (interface{}, error) {
	table := getString(params, "table")
	schema, name, err := validateTable(table)
	if err != nil {
		return nil, err
	}
	where := getString(params, "where")
	if where == "" {
		return nil, fmt.Errorf("参数 where 是必需的（防止全表误删）")
	}

	sqlStr := fmt.Sprintf("DELETE FROM %s WHERE %s", qualifiedTable(schema, name), where)

	ctx, cancel := toolContext()
	defer cancel()

	affected, err := database.Execute(ctx, sqlStr)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{"success": true, "affected_rows": affected}, nil
}

func handleMerge(params map[string]interface{}) (interface{}, error) {
	table := getString(params, "table")
	schema, name, err := validateTable(table)
	if err != nil {
		return nil, err
	}
	data := getObject(params, "data")
	if len(data) == 0 {
		return nil, fmt.Errorf("参数 data 是必需的（非空对象）")
	}
	matchCols := getStringSlice(params, "match_columns")
	if len(matchCols) == 0 {
		return nil, fmt.Errorf("参数 match_columns 是必需的（冲突判定列，需有唯一约束/索引）")
	}

	cols := sortedKeys(data)
	quoted := make([]string, len(cols))
	placeholders := make([]string, len(cols))
	values := make([]interface{}, len(cols))
	for i, c := range cols {
		if err := validateIdentifier(c); err != nil {
			return nil, fmt.Errorf("列名 '%s' 校验失败: %w", c, err)
		}
		quoted[i] = quoteIdent(c)
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		values[i] = data[c]
	}

	matchQuoted := make([]string, len(matchCols))
	matchSet := make(map[string]bool, len(matchCols))
	for i, c := range matchCols {
		if err := validateIdentifier(c); err != nil {
			return nil, fmt.Errorf("match_columns 中列名 '%s' 校验失败: %w", c, err)
		}
		matchQuoted[i] = quoteIdent(c)
		matchSet[c] = true
	}

	// 非冲突列走 DO UPDATE（EXCLUDED 引用新值）；全部是冲突列则 DO NOTHING
	var updateClauses []string
	for _, c := range cols {
		if !matchSet[c] {
			updateClauses = append(updateClauses, fmt.Sprintf("%s = EXCLUDED.%s", quoteIdent(c), quoteIdent(c)))
		}
	}

	var sqlStr string
	if len(updateClauses) == 0 {
		sqlStr = fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s) ON CONFLICT (%s) DO NOTHING",
			qualifiedTable(schema, name), strings.Join(quoted, ", "), strings.Join(placeholders, ", "),
			strings.Join(matchQuoted, ", "))
	} else {
		sqlStr = fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s) ON CONFLICT (%s) DO UPDATE SET %s",
			qualifiedTable(schema, name), strings.Join(quoted, ", "), strings.Join(placeholders, ", "),
			strings.Join(matchQuoted, ", "), strings.Join(updateClauses, ", "))
	}

	ctx, cancel := toolContext()
	defer cancel()

	affected, err := database.Execute(ctx, sqlStr, values...)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{"success": true, "affected_rows": affected}, nil
}

func handleBatchUpdate(params map[string]interface{}) (interface{}, error) {
	table := getString(params, "table")
	schema, name, err := validateTable(table)
	if err != nil {
		return nil, err
	}
	arr := getArray(params, "updates")
	if len(arr) == 0 {
		return nil, fmt.Errorf("参数 updates 是必需的（[{data:{...}, where:\"...\"}, ...]）")
	}

	var statements []database.Statement
	for i, item := range arr {
		entry, ok := item.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("updates[%d] 必须是对象", i)
		}
		data, _ := entry["data"].(map[string]interface{})
		if len(data) == 0 {
			return nil, fmt.Errorf("updates[%d].data 不能为空", i)
		}
		where, _ := entry["where"].(string)
		if strings.TrimSpace(where) == "" {
			return nil, fmt.Errorf("updates[%d].where 是必需的", i)
		}

		cols := sortedKeys(data)
		setClauses := make([]string, len(cols))
		values := make([]interface{}, len(cols))
		for j, c := range cols {
			if err := validateIdentifier(c); err != nil {
				return nil, fmt.Errorf("updates[%d] 列名 '%s' 校验失败: %w", i, c, err)
			}
			setClauses[j] = fmt.Sprintf("%s = $%d", quoteIdent(c), j+1)
			values[j] = data[c]
		}
		statements = append(statements, database.Statement{
			SQL: fmt.Sprintf("UPDATE %s SET %s WHERE %s",
				qualifiedTable(schema, name), strings.Join(setClauses, ", "), where),
			Args: values,
		})
	}

	ctx, cancel := toolContext()
	defer cancel()

	if err := database.ExecuteStatements(ctx, statements); err != nil {
		return nil, err
	}
	return map[string]interface{}{"success": true, "total": len(statements)}, nil
}

func handleBatchDelete(params map[string]interface{}) (interface{}, error) {
	table := getString(params, "table")
	schema, name, err := validateTable(table)
	if err != nil {
		return nil, err
	}
	wheres := getStringSlice(params, "wheres")
	if len(wheres) == 0 {
		return nil, fmt.Errorf("参数 wheres 是必需的（WHERE 条件数组）")
	}

	var statements []string
	for i, where := range wheres {
		if strings.TrimSpace(where) == "" {
			return nil, fmt.Errorf("wheres[%d] 不能为空", i)
		}
		statements = append(statements, fmt.Sprintf("DELETE FROM %s WHERE %s",
			qualifiedTable(schema, name), where))
	}

	ctx, cancel := toolContext()
	defer cancel()

	if err := database.ExecuteTransaction(ctx, statements); err != nil {
		return nil, err
	}
	return map[string]interface{}{"success": true, "total": len(statements)}, nil
}
