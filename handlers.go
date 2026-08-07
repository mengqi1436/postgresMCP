package main

import (
	"context"
	"encoding/json"
	"fmt"

	"pg-mcp/tools"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// textResult 返回文本内容成功结果（等价旧 SDK 的 NewToolResultText）。
func textResult(s string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: s}}}
}

// errorResult 返回业务错误结果（等价旧 SDK 的 NewToolResultError）：
// IsError=true 且 Content 为错误文本，会话继续。
func errorResult(err error) *mcp.CallToolResult {
	r := &mcp.CallToolResult{}
	r.SetError(err)
	return r
}

func handleListTools(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	category := ""
	if len(req.Params.Arguments) > 0 {
		var args map[string]any
		if err := json.Unmarshal(req.Params.Arguments, &args); err == nil {
			if c, ok := args["category"].(string); ok {
				category = c
			}
		}
	}

	toolList := tools.GetAllTools(category)
	categories := tools.GetCategories()

	response := map[string]any{
		"total":      len(toolList),
		"categories": categories,
		"tools":      toolList,
	}

	data, err := json.MarshalIndent(response, "", "  ")
	if err != nil {
		return errorResult(fmt.Errorf("序列化失败: %v", err)), nil
	}
	return textResult(string(data)), nil
}

func handleExecute(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := make(map[string]any)
	if len(req.Params.Arguments) > 0 {
		_ = json.Unmarshal(req.Params.Arguments, &args)
	}

	toolName, _ := args["tool_name"].(string)
	if toolName == "" {
		return errorResult(fmt.Errorf("参数 tool_name 是必需的")), nil
	}

	params := make(map[string]any)
	if p, ok := args["params"].(map[string]any); ok {
		params = p
	}

	result, err := tools.ExecuteTool(toolName, params)
	if err != nil {
		return errorResult(err), nil
	}

	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return errorResult(fmt.Errorf("结果序列化失败: %v", err)), nil
	}
	return textResult(string(data)), nil
}
