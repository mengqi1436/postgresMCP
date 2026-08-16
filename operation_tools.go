package main

import (
	"context"
	"encoding/json"
	"fmt"
	"unicode/utf8"

	"pg-mcp/tools"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// registerOperationTools 把内部注册表中的每个工具动态注册为同名原生 MCP 工具，
// 经 tools/list 发现、tools/call 调用（协议原生方式，无中间分发层）。
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

		out, err := tools.ExecuteTool(toolName, params)
		if err != nil {
			return errorResult(err), nil
		}

		data, err := marshalToolResult(out)
		if err != nil {
			return errorResult(fmt.Errorf("结果序列化失败: %v", err)), nil
		}
		return textResult(data), nil
	}
}

// maxOutputChars 工具输出预算（字符数）：超出即截断并追加提示。
// 全局兜底：任何工具（含元数据/视图定义等大返回）都不会撑爆模型上下文。
const maxOutputChars = 30000

// marshalToolResult 把工具结果序列化为紧凑 JSON（无缩进，节省 token）；
// 超过输出预算时按完整字符截断（不会切断多字节字符）并追加截断提示。
func marshalToolResult(out any) (string, error) {
	data, err := json.Marshal(out)
	if err != nil {
		return "", err
	}
	s := string(data)
	if n := utf8.RuneCountInString(s); n > maxOutputChars {
		runes := []rune(s)
		s = string(runes[:maxOutputChars]) +
			"\n... [输出已截断：超出 token 预算，请使用 query_paginated 翻页或缩小查询范围（WHERE/limit）]"
	}
	return s, nil
}
