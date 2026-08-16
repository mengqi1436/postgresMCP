package main

import (
	"strings"

	"pg-mcp/tools"

	"github.com/google/jsonschema-go/jsonschema"
)

// paramKind 描述一个参数的 JSON Schema 类型（与旧 optionForOperationParam
// 同源的集中映射，扩展了数组的 items 类型与参数描述）。
type paramKind int

const (
	kindString paramKind = iota
	kindNumber
	kindBoolean
	kindObject
	kindStringArray
	kindObjectArray
	kindAnyArray
)

// paramKinds 参数名 -> 类型。未收录的参数默认字符串。
var paramKinds = map[string]paramKind{
	// boolean
	"analyze": kindBoolean, "atomic": kindBoolean, "buffers": kindBoolean,
	"cascade": kindBoolean, "clean": kindBoolean, "confirm": kindBoolean,
	"create_db": kindBoolean, "if_exists": kindBoolean, "include_schema": kindBoolean,
	"or_replace": kindBoolean, "unique": kindBoolean, "verify": kindBoolean,
	// number
	"cache_size": kindNumber, "connection_limit": kindNumber, "increment_by": kindNumber,
	"jobs": kindNumber, "limit": kindNumber, "max_parallel": kindNumber,
	"max_value": kindNumber, "page": kindNumber, "page_size": kindNumber,
	"port": kindNumber, "reject_limit": kindNumber, "start_with": kindNumber,
	"timeout_seconds": kindNumber,
	// object
	"data": kindObject, "options": kindObject,
	// string 数组
	"columns": kindStringArray, "exclude_tables": kindStringArray,
	"extra_args": kindStringArray, "index_names": kindStringArray,
	"match_columns": kindStringArray, "queries": kindStringArray,
	"statements": kindStringArray, "table_names": kindStringArray,
	// object 数组
	"files": kindObjectArray, "indexes": kindObjectArray, "rows": kindObjectArray,
	"tables": kindObjectArray, "updates": kindObjectArray, "wheres": kindObjectArray,
	// 任意数组（绑定参数类型不一）
	"params": kindAnyArray,
}

// paramDescriptions 常见参数的说明；未收录的参数以参数名本身作为说明。
var paramDescriptions = map[string]string{
	"sql":             "SQL 语句（可用 $1,$2... 占位符绑定参数）",
	"params":          "绑定参数数组（对应 $1,$2... 占位符）",
	"table":           "表名（可带 schema 前缀）",
	"table_name":      "表名",
	"table_names":     "表名数组",
	"schema":          "模式名（schema）",
	"where":           "WHERE 过滤条件（必填防误删/误更新）",
	"wheres":          "WHERE 条件数组",
	"data":            "数据对象 {字段: 值}",
	"rows":            "数据数组 [{字段: 值}, ...]",
	"columns":         "列名数组",
	"index_name":      "索引名（可为 schema.index）",
	"index_names":     "索引名数组",
	"queries":         "SQL 查询语句数组",
	"statements":      "SQL 语句数组",
	"database_name":   "数据库名",
	"username":        "用户名",
	"password":        "密码",
	"role_name":       "角色名",
	"privilege":       "权限（如 SELECT ON TABLE t / ALL PRIVILEGES ON DATABASE d）",
	"grantee":         "用户或角色名",
	"tablespace_name": "表空间名",
	"datafile":        "数据文件路径",
	"view_name":       "视图名",
	"seq_name":        "序列名",
	"function_name":   "函数名",
	"procedure_name":  "存储过程名",
	"output_file":     "导出文件路径",
	"input_file":      "导入文件路径",
	"backup_dir":      "备份目录",
	"backup_name":     "备份子目录名",
	"files":           "CSV 文件描述数组 [{csv_file, table, schema, columns, ...}]",
	"tables":          "表描述数组",
	"indexes":         "索引描述数组",
	"updates":         "更新数组 [{data:{字段:值}, where:条件}, ...]",
	"match_columns":   "冲突判定列名数组（需有唯一约束/索引）",
	"confirm":         "危险操作确认标记（必须为 true）",
	"if_exists":       "存在时才操作（IF EXISTS）",
	"or_replace":      "是否使用 OR REPLACE",
	"unique":          "是否唯一索引",
	"atomic":          "是否整体事务全有或全无",
	"verify":          "是否校验备份完整性",
	"clean":           "先删后建（--clean）",
	"create_db":       "连同建库（--create）",
	"jobs":            "并行度（-j）",
	"section":         "恢复区段 pre-data|data|post-data",
	"extra_args":      "额外命令行参数数组",
	"limit":           "返回条数上限",
	"detail_level":    "返回粒度 summary|detail|full（summary 只返回概览+示例行最省 token，detail 返回完整行（默认），full 提高默认行数上限到 10000）",
	"page":            "页码（从 1 开始）",
	"page_size":       "每页条数",
	"timeout_seconds": "超时秒数",
	"mode":            "停止模式 smart|fast|immediate",
	"state":           "会话状态过滤（active/idle/...）",
	"format":          "导出格式 insert|json（默认 json）",
	"operation":       "操作类型 ADD|MODIFY|DROP",
	"type":            "列类型（ADD/MODIFY 时需要）",
	"owner":           "属主",
	"encoding":        "编码（如 UTF8）",
	"index_match":     "索引名匹配方式 exact|prefix|like（默认 prefix）",
	"include_system":  "是否包含系统 schema",
	"start_with":      "序列起始值（默认 1）",
	"increment_by":    "步长（默认 1）",
	"max_value":       "最大值",
	"cache_size":      "缓存大小",
}

// toolRequired 各工具必填参数；未收录的工具没有必填参数。
// 与各工具描述中的 "(必需)" 标注保持一致（运行时 handler 仍会二次校验）。
var toolRequired = map[string][]string{
	"query": {"sql"}, "query_one": {"sql"}, "query_paginated": {"sql"},
	"count": {"table"}, "batch_query": {"queries"},
	"insert": {"table", "data"}, "insert_batch": {"table", "rows"},
	"update": {"table", "data", "where"}, "delete": {"table", "where"},
	"merge":        {"table", "data", "match_columns"},
	"batch_update": {"table", "updates"}, "batch_delete": {"table", "wheres"},
	"execute_sql": {"sql"}, "execute_transaction": {"statements"},
	"call_function": {"function_name"}, "call_procedure": {"procedure_name"},
	"explain_plan": {"sql"}, "batch_execute_sql": {"statements"},
	"create_table": {"table_name", "columns"},
	"alter_table":  {"table_name", "operation", "column"},
	"drop_table":   {"table_name"}, "create_index": {"index_name", "table_name", "columns"},
	"drop_index": {"index_name"}, "execute_ddl": {"sql"},
	"batch_create_tables": {"tables"}, "batch_create_indexes": {"indexes"},
	"batch_drop_tables": {"table_names"}, "batch_drop_indexes": {"index_names"},
	"create_view": {"view_name", "sql"}, "drop_view": {"view_name"},
	"create_sequence": {"seq_name"}, "drop_sequence": {"seq_name"},
	"describe_table": {"table_name"}, "batch_describe_tables": {"table_names"},
	"describe_index": {"index_name"}, "list_constraints": {"table_name"},
	"list_table_partitions": {"table_name"}, "get_table_ddl": {"table_name"},
	"create_user": {"username", "password"}, "drop_user": {"username"},
	"grant_privilege": {"privilege", "grantee"}, "revoke_privilege": {"privilege", "grantee"},
	"create_role": {"role_name"}, "drop_role": {"role_name"},
	"create_tablespace": {"tablespace_name", "datafile"},
	"logical_export":    {"output_file"}, "logical_import": {"input_file"},
	"physical_backup": {"backup_dir"}, "physical_restore": {"backup_dir", "confirm"},
	"export_table_data": {"table_name"}, "batch_import_csv": {"files"},
	"create_database": {"database_name"}, "delete_database": {"database_name", "confirm"},
}

// buildSchema 为工具生成输入 JSON Schema（type=object, properties, required）。
// 直接作为 mcp.Tool.InputSchema 使用（官方 SDK 的 AddTool 要求 root type 为 object）。
func buildSchema(info tools.ToolInfo) *jsonschema.Schema {
	props := make(map[string]*jsonschema.Schema, len(info.Params))
	for _, name := range info.Params {
		props[name] = schemaForParam(name)
	}
	return &jsonschema.Schema{
		Type:       "object",
		Properties: props,
		Required:   toolRequired[info.Name],
	}
}

// schemaForParam 按参数名生成属性 schema（类型表 + 描述表，缺省 string/参数名）。
func schemaForParam(name string) *jsonschema.Schema {
	s := &jsonschema.Schema{
		Type:        "string",
		Description: paramDescription(name),
	}
	switch paramKinds[name] {
	case kindNumber:
		s.Type = "number"
	case kindBoolean:
		s.Type = "boolean"
	case kindObject:
		s.Type = "object"
	case kindStringArray:
		s.Type = "array"
		s.Items = &jsonschema.Schema{Type: "string"}
	case kindObjectArray:
		s.Type = "array"
		s.Items = &jsonschema.Schema{Type: "object"}
	case kindAnyArray:
		s.Type = "array"
	}
	return s
}

// toolTitle 从描述首句推导展示名（按中文句号/换行切分），回退工具名。
func toolTitle(info tools.ToolInfo) string {
	if d := info.Description; d != "" {
		if i := strings.IndexAny(d, "。\n"); i > 0 {
			return strings.TrimSpace(d[:i])
		}
	}
	return info.Name
}

func paramDescription(name string) string {
	if d, ok := paramDescriptions[name]; ok {
		return d
	}
	return name
}
