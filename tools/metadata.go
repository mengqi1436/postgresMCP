package tools

import (
	"fmt"
	"strings"

	"pg-mcp/database"
)

func init() { registerMetadataTools() }

// nonSystemSchemas 过滤系统 schema 的条件（所有元数据查询统一使用）。
const nonSystemSchemas = `(n.nspname <> 'pg_catalog' AND n.nspname <> 'information_schema' AND n.nspname NOT LIKE 'pg_toast%' AND n.nspname NOT LIKE 'pg_temp%')`

func registerMetadataTools() {
	RegisterTool(ToolInfo{
		Name:        "list_databases",
		Category:    "metadata",
		Description: "列出当前实例的所有数据库。无参数",
		Params:      []string{},
	}, handleListDatabases)

	RegisterTool(ToolInfo{
		Name:        "list_schemas",
		Category:    "metadata",
		Description: "列出当前数据库的非系统 schema。参数: include_system(可选, 默认false)",
		Params:      []string{"include_system"},
	}, handleListSchemas)

	RegisterTool(ToolInfo{
		Name:        "list_tables",
		Category:    "metadata",
		Description: "列出表。参数: schema(可选, 不传则列出所有非系统 schema 的表)",
		Params:      []string{"schema"},
	}, handleListTables)

	RegisterTool(ToolInfo{
		Name:        "list_views",
		Category:    "metadata",
		Description: "列出视图。参数: schema(可选)",
		Params:      []string{"schema"},
	}, handleListViews)

	RegisterTool(ToolInfo{
		Name:        "describe_table",
		Category:    "metadata",
		Description: "获取表结构（列/类型/可空/默认值/注释）。参数: table_name(必需), schema(可选)",
		Params:      []string{"table_name", "schema"},
	}, handleDescribeTable)

	RegisterTool(ToolInfo{
		Name:        "batch_describe_tables",
		Category:    "metadata",
		Description: "批量获取多表结构。参数: table_names(必需, 数组，可含 schema.前缀), schema(可选, 统一限定)",
		Params:      []string{"table_names", "schema"},
	}, handleBatchDescribeTables)

	RegisterTool(ToolInfo{
		Name:        "list_indexes",
		Category:    "metadata",
		Description: "列出索引及定义。参数: table_name(可选), schema(可选)",
		Params:      []string{"table_name", "schema"},
	}, handleListIndexes)

	RegisterTool(ToolInfo{
		Name:        "search_indexes",
		Category:    "metadata",
		Description: "按名称检索索引。参数: index_name(可选), index_match(exact|prefix|like, 默认prefix), table_name(可选), schema(可选)",
		Params:      []string{"index_name", "index_match", "table_name", "schema"},
	}, handleSearchIndexes)

	RegisterTool(ToolInfo{
		Name:        "describe_index",
		Category:    "metadata",
		Description: "索引详情（类型/唯一/主键/列/定义/过滤条件）。参数: index_name(必需), schema(可选)",
		Params:      []string{"index_name", "schema"},
	}, handleDescribeIndex)

	RegisterTool(ToolInfo{
		Name:        "list_sequences",
		Category:    "metadata",
		Description: "列出序列。参数: schema(可选)",
		Params:      []string{"schema"},
	}, handleListSequences)

	RegisterTool(ToolInfo{
		Name:        "list_functions",
		Category:    "metadata",
		Description: "列出非系统 schema 的函数。参数: schema(可选)",
		Params:      []string{"schema"},
	}, handleListFunctions)

	RegisterTool(ToolInfo{
		Name:        "list_procedures",
		Category:    "metadata",
		Description: "列出存储过程（PG11+ 的 PROCEDURE）。参数: schema(可选)",
		Params:      []string{"schema"},
	}, handleListProcedures)

	RegisterTool(ToolInfo{
		Name:        "list_triggers",
		Category:    "metadata",
		Description: "列出触发器。参数: table_name(可选)",
		Params:      []string{"table_name"},
	}, handleListTriggers)

	RegisterTool(ToolInfo{
		Name:        "list_constraints",
		Category:    "metadata",
		Description: "列出表约束（主键/外键/唯一/检查）。参数: table_name(必需), schema(可选)",
		Params:      []string{"table_name", "schema"},
	}, handleListConstraints)

	RegisterTool(ToolInfo{
		Name:        "list_table_partitions",
		Category:    "metadata",
		Description: "列出分区表的分区信息（PG 声明式分区）。参数: table_name(必需), schema(可选)",
		Params:      []string{"table_name", "schema"},
	}, handleListTablePartitions)

	RegisterTool(ToolInfo{
		Name:        "get_table_ddl",
		Category:    "metadata",
		Description: "反推建表 DDL（列/主键/唯一约束 + 索引定义）。参数: table_name(必需), schema(可选)",
		Params:      []string{"table_name", "schema"},
	}, handleGetTableDDL)

	RegisterTool(ToolInfo{
		Name:        "list_extensions",
		Category:    "metadata",
		Description: "列出已安装扩展。无参数",
		Params:      []string{},
	}, handleListExtensions)
}

func handleListDatabases(params map[string]interface{}) (interface{}, error) {
	ctx, cancel := toolContext()
	defer cancel()

	rows, err := database.Query(ctx, `
SELECT d.datname AS database_name,
       pg_catalog.pg_get_userbyid(d.datdba) AS owner,
       pg_catalog.pg_encoding_to_char(d.encoding) AS encoding,
       d.datcollate AS collate,
       d.datctype AS ctype,
       pg_catalog.pg_size_pretty(pg_catalog.pg_database_size(d.datname)) AS size
FROM pg_catalog.pg_database d
WHERE d.datistemplate = false
ORDER BY d.datname`)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{"databases": rows, "count": len(rows)}, nil
}

func handleListSchemas(params map[string]interface{}) (interface{}, error) {
	ctx, cancel := toolContext()
	defer cancel()

	sqlStr := `SELECT n.nspname AS schema_name, pg_catalog.pg_get_userbyid(n.nspowner) AS owner
FROM pg_catalog.pg_namespace n`
	if !getBool(params, "include_system") {
		sqlStr += " WHERE " + nonSystemSchemas
	}
	sqlStr += " ORDER BY n.nspname"

	rows, err := database.Query(ctx, sqlStr)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{"schemas": rows, "count": len(rows)}, nil
}

func handleListTables(params map[string]interface{}) (interface{}, error) {
	ctx, cancel := toolContext()
	defer cancel()

	sqlStr := `
SELECT n.nspname AS schema_name, c.relname AS table_name,
       pg_catalog.pg_get_userbyid(c.relowner) AS owner,
       pg_catalog.pg_size_pretty(pg_catalog.pg_total_relation_size(c.oid)) AS total_size,
       obj_description(c.oid) AS comment
FROM pg_catalog.pg_class c
JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
WHERE c.relkind IN ('r','p') AND ` + nonSystemSchemas

	var args []interface{}
	if schema := getString(params, "schema"); schema != "" {
		args = append(args, schema)
		sqlStr += fmt.Sprintf(" AND n.nspname = $%d", len(args))
	}
	sqlStr += " ORDER BY n.nspname, c.relname"

	rows, err := database.Query(ctx, sqlStr, args...)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{"tables": rows, "count": len(rows)}, nil
}

func handleListViews(params map[string]interface{}) (interface{}, error) {
	ctx, cancel := toolContext()
	defer cancel()

	sqlStr := `
SELECT n.nspname AS schema_name, c.relname AS view_name,
       pg_catalog.pg_get_viewdef(c.oid) AS definition
FROM pg_catalog.pg_class c
JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
WHERE c.relkind = 'v' AND ` + nonSystemSchemas

	var args []interface{}
	if schema := getString(params, "schema"); schema != "" {
		args = append(args, schema)
		sqlStr += fmt.Sprintf(" AND n.nspname = $%d", len(args))
	}
	sqlStr += " ORDER BY n.nspname, c.relname"

	rows, err := database.Query(ctx, sqlStr, args...)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{"views": rows, "count": len(rows)}, nil
}

// resolveTableRef 返回用于 $1::regclass 的表引用（schema 可选）。
func resolveTableRef(tableName, schema string) (string, error) {
	if tableName == "" {
		return "", fmt.Errorf("参数 table_name 是必需的")
	}
	if schema != "" {
		return quoteIdent(schema) + "." + quoteIdent(tableName), nil
	}
	return tableName, nil
}

func handleDescribeTable(params map[string]interface{}) (interface{}, error) {
	return describeOneTable(getString(params, "table_name"), getString(params, "schema"))
}

func describeOneTable(tableName, schema string) (interface{}, error) {
	ref, err := resolveTableRef(tableName, schema)
	if err != nil {
		return nil, err
	}

	ctx, cancel := toolContext()
	defer cancel()

	// 表级信息
	tableInfo, err := database.Query(ctx, `
SELECT n.nspname AS schema_name, c.relname AS table_name,
       CASE c.relkind WHEN 'r' THEN 'table' WHEN 'p' THEN 'partitioned' WHEN 'v' THEN 'view' ELSE c.relkind::text END AS kind,
       pg_catalog.pg_get_userbyid(c.relowner) AS owner,
       obj_description(c.oid) AS comment,
       pg_catalog.pg_size_pretty(pg_catalog.pg_total_relation_size(c.oid)) AS total_size
FROM pg_catalog.pg_class c
JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
WHERE c.oid = $1::regclass`, ref)
	if err != nil {
		return nil, fmt.Errorf("表 '%s' 不存在或不可访问: %w", ref, err)
	}
	if len(tableInfo) == 0 {
		return nil, fmt.Errorf("表 '%s' 不存在", ref)
	}

	// 列信息（含注释）
	columns, err := database.Query(ctx, `
SELECT a.attname AS column_name,
       pg_catalog.format_type(a.atttypid, a.atttypmod) AS data_type,
       NOT a.attnotnull AS nullable,
       pg_catalog.pg_get_expr(d.adbin, d.adrelid) AS default_expr,
       col_description(a.attrelid, a.attnum) AS comment
FROM pg_catalog.pg_attribute a
LEFT JOIN pg_catalog.pg_attrdef d ON d.adrelid = a.attrelid AND d.adnum = a.attnum
WHERE a.attrelid = $1::regclass AND a.attnum > 0 AND NOT a.attisdropped
ORDER BY a.attnum`, ref)
	if err != nil {
		return nil, err
	}

	result := map[string]interface{}{
		"table":   tableInfo[0],
		"columns": columns,
	}
	return result, nil
}

func handleBatchDescribeTables(params map[string]interface{}) (interface{}, error) {
	names := getStringSlice(params, "table_names")
	if len(names) == 0 {
		return nil, fmt.Errorf("参数 table_names 是必需的")
	}
	defaultSchema := getString(params, "schema")

	var results []map[string]interface{}
	for _, t := range names {
		schema, name := splitSchemaName(t)
		if schema == "" {
			schema = defaultSchema
		}
		entry := map[string]interface{}{"table": t}
		desc, err := describeOneTable(name, schema)
		if err != nil {
			entry["ok"] = false
			entry["error"] = err.Error()
		} else {
			entry["ok"] = true
			entry["structure"] = desc
		}
		results = append(results, entry)
	}
	return map[string]interface{}{"results": results, "total": len(results)}, nil
}

func handleListIndexes(params map[string]interface{}) (interface{}, error) {
	ctx, cancel := toolContext()
	defer cancel()

	sqlStr := `
SELECT i.schemaname AS schema_name, i.tablename AS table_name,
       i.indexname AS index_name, i.indexdef AS definition
FROM pg_catalog.pg_indexes i
JOIN pg_catalog.pg_namespace n ON n.nspname = i.schemaname
WHERE ` + nonSystemSchemas

	var args []interface{}
	if table := getString(params, "table_name"); table != "" {
		args = append(args, table)
		sqlStr += fmt.Sprintf(" AND i.tablename = $%d", len(args))
	}
	if schema := getString(params, "schema"); schema != "" {
		args = append(args, schema)
		sqlStr += fmt.Sprintf(" AND i.schemaname = $%d", len(args))
	}
	sqlStr += " ORDER BY i.schemaname, i.tablename, i.indexname"

	rows, err := database.Query(ctx, sqlStr, args...)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{"indexes": rows, "count": len(rows)}, nil
}

func handleSearchIndexes(params map[string]interface{}) (interface{}, error) {
	ctx, cancel := toolContext()
	defer cancel()

	match := strings.ToLower(getString(params, "index_match"))
	if match == "" {
		match = "prefix"
	}

	sqlStr := `
SELECT i.schemaname AS schema_name, i.tablename AS table_name,
       i.indexname AS index_name, i.indexdef AS definition
FROM pg_catalog.pg_indexes i
JOIN pg_catalog.pg_namespace n ON n.nspname = i.schemaname
WHERE ` + nonSystemSchemas

	var args []interface{}
	if name := getString(params, "index_name"); name != "" {
		args = append(args, name)
		switch match {
		case "exact":
			sqlStr += fmt.Sprintf(" AND i.indexname = $%d", len(args))
		case "like":
			sqlStr += fmt.Sprintf(" AND i.indexname LIKE '%%' || $%d || '%%'", len(args))
		default: // prefix
			sqlStr += fmt.Sprintf(" AND i.indexname LIKE $%d || '%%'", len(args))
		}
	}
	if table := getString(params, "table_name"); table != "" {
		args = append(args, table)
		sqlStr += fmt.Sprintf(" AND i.tablename = $%d", len(args))
	}
	if schema := getString(params, "schema"); schema != "" {
		args = append(args, schema)
		sqlStr += fmt.Sprintf(" AND i.schemaname = $%d", len(args))
	}
	sqlStr += " ORDER BY i.indexname LIMIT 200"

	rows, err := database.Query(ctx, sqlStr, args...)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{"indexes": rows, "count": len(rows)}, nil
}

func handleDescribeIndex(params map[string]interface{}) (interface{}, error) {
	indexName := getString(params, "index_name")
	if indexName == "" {
		return nil, fmt.Errorf("参数 index_name 是必需的")
	}

	ctx, cancel := toolContext()
	defer cancel()

	sqlStr := `
SELECT n.nspname AS schema_name, i.relname AS index_name, t.relname AS table_name,
       am.amname AS index_type, ix.indisunique AS is_unique, ix.indisprimary AS is_primary,
       pg_catalog.pg_get_indexdef(ix.indexrelid) AS definition,
       pg_catalog.pg_get_expr(ix.indpred, ix.indrelid) AS filter_predicate
FROM pg_catalog.pg_index ix
JOIN pg_catalog.pg_class i ON i.oid = ix.indexrelid
JOIN pg_catalog.pg_class t ON t.oid = ix.indrelid
JOIN pg_catalog.pg_namespace n ON n.oid = i.relnamespace
JOIN pg_catalog.pg_am am ON am.oid = i.relam
WHERE i.relname = $1`
	args := []interface{}{indexName}
	if schema := getString(params, "schema"); schema != "" {
		args = append(args, schema)
		sqlStr += fmt.Sprintf(" AND n.nspname = $%d", len(args))
	}
	sqlStr += " LIMIT 1"

	rows, err := database.Query(ctx, sqlStr, args...)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("索引 '%s' 不存在", indexName)
	}

	// 索引列
	cols, err := database.Query(ctx, `
SELECT a.attname AS column_name
FROM pg_catalog.pg_attribute a
WHERE a.attrelid = $1::regclass AND a.attnum > 0
ORDER BY a.attnum`, indexName)
	if err == nil {
		rows[0]["columns"] = cols
	}

	return rows[0], nil
}

func handleListSequences(params map[string]interface{}) (interface{}, error) {
	ctx, cancel := toolContext()
	defer cancel()

	sqlStr := `
SELECT s.schemaname AS schema_name, s.sequencename AS sequence_name,
       s.start_value, s.min_value, s.max_value, s.increment_by, s.cycle AS cycle, s.cache_size
FROM pg_catalog.pg_sequences s
JOIN pg_catalog.pg_namespace n ON n.nspname = s.schemaname
WHERE ` + nonSystemSchemas

	var args []interface{}
	if schema := getString(params, "schema"); schema != "" {
		args = append(args, schema)
		sqlStr += fmt.Sprintf(" AND s.schemaname = $%d", len(args))
	}
	sqlStr += " ORDER BY s.schemaname, s.sequencename"

	rows, err := database.Query(ctx, sqlStr, args...)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{"sequences": rows, "count": len(rows)}, nil
}

func listRoutines(params map[string]interface{}, proKind string) (interface{}, error) {
	ctx, cancel := toolContext()
	defer cancel()

	sqlStr := `
SELECT n.nspname AS schema_name, p.proname AS name,
       pg_catalog.pg_get_function_identity_arguments(p.oid) AS arguments,
       pg_catalog.pg_get_function_result(p.oid) AS result,
       l.lanname AS language
FROM pg_catalog.pg_proc p
JOIN pg_catalog.pg_namespace n ON n.oid = p.pronamespace
JOIN pg_catalog.pg_language l ON l.oid = p.prolang
WHERE p.prokind = '` + proKind + `' AND ` + nonSystemSchemas

	var args []interface{}
	if schema := getString(params, "schema"); schema != "" {
		args = append(args, schema)
		sqlStr += fmt.Sprintf(" AND n.nspname = $%d", len(args))
	}
	sqlStr += " ORDER BY n.nspname, p.proname"

	rows, err := database.Query(ctx, sqlStr, args...)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{"routines": rows, "count": len(rows)}, nil
}

func handleListFunctions(params map[string]interface{}) (interface{}, error) {
	return listRoutines(params, "f")
}

func handleListProcedures(params map[string]interface{}) (interface{}, error) {
	return listRoutines(params, "p")
}

func handleListTriggers(params map[string]interface{}) (interface{}, error) {
	ctx, cancel := toolContext()
	defer cancel()

	sqlStr := `
SELECT t.trigger_schema AS schema_name, t.event_object_table AS table_name,
       t.trigger_name, t.event_manipulation AS event, t.action_timing AS timing,
       t.action_statement
FROM information_schema.triggers t
WHERE t.trigger_schema NOT IN ('pg_catalog','information_schema')`

	var args []interface{}
	if table := getString(params, "table_name"); table != "" {
		args = append(args, table)
		sqlStr += fmt.Sprintf(" AND t.event_object_table = $%d", len(args))
	}
	sqlStr += " ORDER BY t.trigger_schema, t.event_object_table, t.trigger_name"

	rows, err := database.Query(ctx, sqlStr, args...)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{"triggers": rows, "count": len(rows)}, nil
}

func handleListConstraints(params map[string]interface{}) (interface{}, error) {
	tableName := getString(params, "table_name")
	if tableName == "" {
		return nil, fmt.Errorf("参数 table_name 是必需的")
	}
	schema := getString(params, "schema")

	ctx, cancel := toolContext()
	defer cancel()

	// 用 pg_constraint（全量、含 CHECK），列名通过 pg_attribute 反查
	sqlStr := `
SELECT con.conname AS constraint_name,
       CASE con.contype WHEN 'p' THEN 'PRIMARY KEY' WHEN 'u' THEN 'UNIQUE'
            WHEN 'f' THEN 'FOREIGN KEY' WHEN 'c' THEN 'CHECK' WHEN 'x' THEN 'EXCLUSION'
            ELSE con.contype::text END AS constraint_type,
       (SELECT array_agg(a.attname ORDER BY ord.n)
        FROM unnest(con.conkey) WITH ORDINALITY AS ord(attnum, n)
        JOIN pg_catalog.pg_attribute a ON a.attrelid = con.conrelid AND a.attnum = ord.attnum
       ) AS columns,
       pg_catalog.pg_get_constraintdef(con.oid) AS definition
FROM pg_catalog.pg_constraint con
WHERE con.conrelid = $1::regclass
ORDER BY con.contype, con.conname`

	ref, err := resolveTableRef(tableName, schema)
	if err != nil {
		return nil, err
	}
	rows, err := database.Query(ctx, sqlStr, ref)
	if err != nil {
		return nil, fmt.Errorf("查询约束失败（表 '%s' 是否存在？）: %w", ref, err)
	}
	return map[string]interface{}{"constraints": rows, "count": len(rows)}, nil
}

func handleListTablePartitions(params map[string]interface{}) (interface{}, error) {
	tableName := getString(params, "table_name")
	if tableName == "" {
		return nil, fmt.Errorf("参数 table_name 是必需的")
	}
	schema := getString(params, "schema")

	ref, err := resolveTableRef(tableName, schema)
	if err != nil {
		return nil, err
	}

	ctx, cancel := toolContext()
	defer cancel()

	rows, err := database.Query(ctx, `
SELECT pn.nspname AS schema_name, child.relname AS partition_name,
       pg_catalog.pg_get_expr(child.relpartbound, child.oid) AS partition_bound,
       pg_catalog.pg_size_pretty(pg_catalog.pg_total_relation_size(child.oid)) AS total_size
FROM pg_catalog.pg_inherits i
JOIN pg_catalog.pg_class parent ON parent.oid = i.inhparent
JOIN pg_catalog.pg_class child ON child.oid = i.inhrelid
JOIN pg_catalog.pg_namespace pn ON pn.oid = child.relnamespace
WHERE parent.oid = $1::regclass
ORDER BY child.relname`, ref)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{"partitions": rows, "count": len(rows)}, nil
}

func handleGetTableDDL(params map[string]interface{}) (interface{}, error) {
	tableName := getString(params, "table_name")
	schema := getString(params, "schema")
	ref, err := resolveTableRef(tableName, schema)
	if err != nil {
		return nil, err
	}

	ctx, cancel := toolContext()
	defer cancel()

	// 实际 schema/表名（regclass 解析后）
	meta, err := database.Query(ctx, `
SELECT n.nspname AS schema_name, c.relname AS table_name
FROM pg_catalog.pg_class c
JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
WHERE c.oid = $1::regclass`, ref)
	if err != nil || len(meta) == 0 {
		return nil, fmt.Errorf("表 '%s' 不存在或不可访问", ref)
	}
	schemaName, _ := meta[0]["schema_name"].(string)
	realTable, _ := meta[0]["table_name"].(string)

	// 列
	columns, err := database.Query(ctx, `
SELECT a.attname AS column_name,
       pg_catalog.format_type(a.atttypid, a.atttypmod) AS data_type,
       a.attnotnull AS not_null,
       pg_catalog.pg_get_expr(d.adbin, d.adrelid) AS default_expr
FROM pg_catalog.pg_attribute a
LEFT JOIN pg_catalog.pg_attrdef d ON d.adrelid = a.attrelid AND d.adnum = a.attnum
WHERE a.attrelid = $1::regclass AND a.attnum > 0 AND NOT a.attisdropped
ORDER BY a.attnum`, ref)
	if err != nil {
		return nil, err
	}
	colNameByNum := make(map[int]string, len(columns))
	if nums, err := database.Query(ctx, `
SELECT a.attnum AS num, a.attname AS name
FROM pg_catalog.pg_attribute a
WHERE a.attrelid = $1::regclass AND a.attnum > 0 AND NOT a.attisdropped`, ref); err == nil {
		for _, r := range nums {
			if num, ok := r["num"].(int16); ok {
				if name, ok := r["name"].(string); ok {
					colNameByNum[int(num)] = name
				}
			}
		}
	}

	var colLines []string
	for _, c := range columns {
		line := fmt.Sprintf("    %s %s", quoteIdent(fmt.Sprintf("%v", c["column_name"])), c["data_type"])
		if def, ok := c["default_expr"].(string); ok && def != "" {
			line += " DEFAULT " + def
		}
		if nn, ok := c["not_null"].(bool); ok && nn {
			line += " NOT NULL"
		}
		colLines = append(colLines, line)
	}

	// 主键/唯一约束
	constraints, err := database.Query(ctx, `
SELECT con.conname AS name, con.contype AS kind, con.conkey AS keys
FROM pg_catalog.pg_constraint con
WHERE con.conrelid = $1::regclass AND con.contype IN ('p','u')
ORDER BY con.contype, con.conname`, ref)
	if err == nil {
		for _, con := range constraints {
			kind, _ := con["kind"].(string)
			nums := toIntSlice(con["keys"])
			var names []string
			for _, n := range nums {
				if nm, ok := colNameByNum[n]; ok {
					names = append(names, quoteIdent(nm))
				}
			}
			if len(names) == 0 {
				continue
			}
			label := "UNIQUE"
			if kind == "p" {
				label = "PRIMARY KEY"
			}
			colLines = append(colLines, fmt.Sprintf("    CONSTRAINT %s %s (%s)",
				quoteIdent(fmt.Sprintf("%v", con["name"])), label, strings.Join(names, ", ")))
		}
	}

	ddl := fmt.Sprintf("CREATE TABLE %s (\n%s\n);",
		qualifiedTable(schemaName, realTable), strings.Join(colLines, ",\n"))

	// 二级索引定义（不含主键/唯一约束自带的）
	indexRows, err := database.Query(ctx, `
SELECT i.indexname AS index_name, i.indexdef AS definition
FROM pg_catalog.pg_indexes i
WHERE i.schemaname = $1 AND i.tablename = $2
ORDER BY i.indexname`, schemaName, realTable)
	if err != nil {
		indexRows = nil
	}

	return map[string]interface{}{
		"table_name": qualifiedTable(schemaName, realTable),
		"ddl":        ddl,
		"indexes":    indexRows,
	}, nil
}

func handleListExtensions(params map[string]interface{}) (interface{}, error) {
	ctx, cancel := toolContext()
	defer cancel()

	rows, err := database.Query(ctx, `
SELECT e.extname AS extension_name, e.extversion AS version,
       n.nspname AS schema_name, c.description
FROM pg_catalog.pg_extension e
JOIN pg_catalog.pg_namespace n ON n.oid = e.extnamespace
LEFT JOIN pg_catalog.pg_description c ON c.objoid = e.oid
ORDER BY e.extname`)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{"extensions": rows, "count": len(rows)}, nil
}
