package main

import (
	"context"
	"encoding/json"
	"fmt"

	"pg-mcp/tools"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// registerOperationTools 把内部注册表中的每个工具动态注册为同名 MCP 工具
// （双轨制：既可通过 pg_execute 统一入口调用，也可被 MCP 客户端直接调用）。
func registerOperationTools(s *mcp.Server) {
	for _, info := range tools.GetAllTools("") {
		s.AddTool(buildOperationTool(info), handleOperationTool(info.Name))
	}
}

// buildOperationTool 用集中式 schema 生成器构造 MCP 工具定义
// （name/title/description/inputSchema）。
func buildOperationTool(info tools.ToolInfo) *mcp.Tool {
	return &mcp.Tool{
		Name:        info.Name,
		Title:       toolTitle(info),
		Description: info.Description,
		InputSchema: buildSchema(info),
	}
}

// handleOperationTool 包装内部工具处理器：把 wire 上的 json.RawMessage 参数
// 解包为 map[string]any，转交 tools.ExecuteTool，结果序列化为文本内容。
// 业务错误（含 handler panic）以 CallToolResult.IsError 返回，不中断会话
// （等价旧 SDK 的 WithRecovery + NewToolResultError 行为）。
func handleOperationTool(toolName string) mcp.ToolHandler {
	return func(ctx context.Context, req *mcp.CallToolRequest) (result *mcp.CallToolResult, err error) {
		defer func() {
			if r := recover(); r != nil {
				result = errorResult(fmt.Errorf("工具 %s 执行异常: %v", toolName, r))
			}
		}()

		params := make(map[string]any)
		if len(req.Params.Arguments) > 0 {
			if err := json.Unmarshal(req.Params.Arguments, &params); err != nil {
				return errorResult(fmt.Errorf("参数解析失败: %v", err)), nil
			}
		}
		if params == nil {
			params = make(map[string]any)
		}

		out, err := tools.ExecuteTool(toolName, params)
		if err != nil {
			return errorResult(err), nil
		}

		data, err := json.MarshalIndent(out, "", "  ")
		if err != nil {
			return errorResult(fmt.Errorf("结果序列化失败: %v", err)), nil
		}
		return textResult(string(data)), nil
	}
}
