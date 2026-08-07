package main

import (
	"context"
	"encoding/json"

	"pg-mcp/tools"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// registerOperationTools 把内部注册表中的每个工具动态注册为同名 MCP 工具（双轨制：
// 既可通过 pg_execute 统一入口调用，也可被 MCP 客户端直接调用）。
func registerOperationTools(s *server.MCPServer) {
	for _, info := range tools.GetAllTools("") {
		toolInfo := info
		s.AddTool(buildOperationTool(toolInfo), handleOperationTool(toolInfo.Name))
	}
}

func buildOperationTool(info tools.ToolInfo) mcp.Tool {
	options := []mcp.ToolOption{mcp.WithDescription(info.Description)}
	for _, param := range info.Params {
		options = append(options, optionForOperationParam(info.Name, param))
	}
	return mcp.NewTool(info.Name, options...)
}

// optionForOperationParam 按参数名生成 schema 类型（与 dm-mcp 同款映射表，
// 按 PG 工具集调整）。
func optionForOperationParam(toolName, param string) mcp.ToolOption {
	switch param {
	case "analyze", "atomic", "buffers", "cascade", "clean", "confirm", "create_db",
		"if_exists", "include_schema", "or_replace", "unique", "verify":
		return mcp.WithBoolean(param)
	case "cache_size", "connection_limit", "increment_by", "jobs", "limit",
		"max_parallel", "max_value", "page", "page_size", "port",
		"reject_limit", "start_with", "timeout_seconds":
		return mcp.WithNumber(param)
	case "data", "options":
		return mcp.WithObject(param)
	case "columns", "exclude_tables", "extra_args", "files", "indexes",
		"index_names", "match_columns", "params", "queries", "rows",
		"statements", "table_names", "tables", "updates", "wheres":
		return mcp.WithArray(param)
	default:
		return mcp.WithString(param)
	}
}

func handleOperationTool(toolName string) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		params, ok := req.Params.Arguments.(map[string]interface{})
		if !ok || params == nil {
			params = make(map[string]interface{})
		}

		result, err := tools.ExecuteTool(toolName, params)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		data, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return mcp.NewToolResultError("结果序列化失败: " + err.Error()), nil
		}
		return mcp.NewToolResultText(string(data)), nil
	}
}
