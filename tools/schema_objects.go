package tools

import (
	"fmt"

	"pg-mcp/database"
)

func init() { registerSchemaObjectTools() }

func registerSchemaObjectTools() {
	RegisterTool(ToolInfo{
		Name:        "create_view",
		Category:    "ddl",
		Description: "创建视图。参数: view_name(必需), sql(必需, SELECT 语句), or_replace(可选)",
		Params:      []string{"view_name", "sql", "or_replace"},
	}, handleCreateView)

	RegisterTool(ToolInfo{
		Name:        "drop_view",
		Category:    "ddl",
		Description: "删除视图。参数: view_name(必需), if_exists(可选)",
		Params:      []string{"view_name", "if_exists"},
	}, handleDropView)

	RegisterTool(ToolInfo{
		Name:        "create_sequence",
		Category:    "ddl",
		Description: "创建序列。参数: seq_name(必需), start_with(默认1), increment_by(默认1), max_value(可选), cache_size(可选)",
		Params:      []string{"seq_name", "start_with", "increment_by", "max_value", "cache_size"},
	}, handleCreateSequence)

	RegisterTool(ToolInfo{
		Name:        "drop_sequence",
		Category:    "ddl",
		Description: "删除序列。参数: seq_name(必需), if_exists(可选)",
		Params:      []string{"seq_name", "if_exists"},
	}, handleDropSequence)
}

func handleCreateView(params map[string]interface{}) (interface{}, error) {
	viewName := getString(params, "view_name")
	schema, name, err := validateTable(viewName)
	if err != nil {
		return nil, err
	}
	sqlStr := getString(params, "sql")
	if sqlStr == "" {
		return nil, fmt.Errorf("参数 sql 是必需的（视图的 SELECT 语句）")
	}

	stmt := "CREATE "
	if getBool(params, "or_replace") {
		stmt += "OR REPLACE "
	}
	stmt += fmt.Sprintf("VIEW %s AS %s", qualifiedTable(schema, name), sqlStr)

	ctx, cancel := toolContext()
	defer cancel()

	if err := database.ExecuteDDL(ctx, stmt); err != nil {
		return nil, err
	}
	return map[string]interface{}{"success": true, "sql": stmt}, nil
}

func handleDropView(params map[string]interface{}) (interface{}, error) {
	viewName := getString(params, "view_name")
	schema, name, err := validateTable(viewName)
	if err != nil {
		return nil, err
	}

	stmt := "DROP VIEW "
	if getBool(params, "if_exists") {
		stmt += "IF EXISTS "
	}
	stmt += qualifiedTable(schema, name)

	ctx, cancel := toolContext()
	defer cancel()

	if err := database.ExecuteDDL(ctx, stmt); err != nil {
		return nil, err
	}
	return map[string]interface{}{"success": true, "sql": stmt}, nil
}

func handleCreateSequence(params map[string]interface{}) (interface{}, error) {
	seqName := getString(params, "seq_name")
	schema, name, err := validateTable(seqName)
	if err != nil {
		return nil, err
	}

	stmt := fmt.Sprintf("CREATE SEQUENCE %s", qualifiedTable(schema, name))
	if v := getIntDefault(params, "start_with", 0); v != 0 {
		stmt += fmt.Sprintf(" START WITH %d", v)
	}
	if v := getIntDefault(params, "increment_by", 0); v != 0 {
		stmt += fmt.Sprintf(" INCREMENT BY %d", v)
	}
	if v := getIntDefault(params, "max_value", 0); v != 0 {
		stmt += fmt.Sprintf(" MAXVALUE %d", v)
	}
	if v := getIntDefault(params, "cache_size", 0); v != 0 {
		stmt += fmt.Sprintf(" CACHE %d", v)
	}

	ctx, cancel := toolContext()
	defer cancel()

	if err := database.ExecuteDDL(ctx, stmt); err != nil {
		return nil, err
	}
	return map[string]interface{}{"success": true, "sql": stmt}, nil
}

func handleDropSequence(params map[string]interface{}) (interface{}, error) {
	seqName := getString(params, "seq_name")
	schema, name, err := validateTable(seqName)
	if err != nil {
		return nil, err
	}

	stmt := "DROP SEQUENCE "
	if getBool(params, "if_exists") {
		stmt += "IF EXISTS "
	}
	stmt += qualifiedTable(schema, name)

	ctx, cancel := toolContext()
	defer cancel()

	if err := database.ExecuteDDL(ctx, stmt); err != nil {
		return nil, err
	}
	return map[string]interface{}{"success": true, "sql": stmt}, nil
}
