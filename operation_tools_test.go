package main

import (
	"context"
	"strings"
	"testing"

	"pg-mcp/tools"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// registerPanicTool 注册一个 handler 必然 panic 的测试工具，验证操作工具
// 包装器的 recover 逻辑（handler panic 转 IsError 结果，不崩溃服务器）。
func registerPanicTool(t *testing.T) {
	t.Helper()
	tools.RegisterTool(tools.ToolInfo{
		Name:        "panic_test_tool",
		Category:    "query",
		Description: "测试用：handler 触发 panic",
		Params:      []string{},
	}, func(params map[string]interface{}) (interface{}, error) {
		panic("boom")
	})
	t.Cleanup(func() { tools.UnregisterTool("panic_test_tool") })
}

// TestKillWrapperEmptyArguments 杀 Arguments 长度边界变异（len>0 → len>=0）：
// 空参数应继续交给 handler（报 "sql 是必需的"），而非 "参数解析失败"。
func TestKillWrapperEmptyArguments(t *testing.T) {
	h := handleOperationTool("query")
	req := &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Arguments: nil}}
	res, err := h(context.Background(), req)
	if err != nil {
		t.Fatalf("空参数应为业务错误而非协议错误: %v", err)
	}
	if !res.IsError || !strings.Contains(resultText(res), "sql 是必需的") {
		t.Fatalf("空参数 = isError:%v text:%q", res.IsError, resultText(res))
	}
}
