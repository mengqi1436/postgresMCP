package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// 注册全部 79 个操作工具的测试用服务器。
func newTestServer(t *testing.T) (*mcp.Server, *mcp.ClientSession) {
	t.Helper()
	server := mcp.NewServer(&mcp.Implementation{Name: "test-server", Version: "0.0.1"}, nil)
	registerOperationTools(server)

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	t1, t2 := mcp.NewInMemoryTransports()

	ctx := context.Background()
	ss, err := server.Connect(ctx, t1, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	t.Cleanup(func() { _ = ss.Close() })

	cs, err := client.Connect(ctx, t2, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return server, cs
}

// resultText 提取 CallToolResult 中的纯文本内容。
func resultText(res *mcp.CallToolResult) string {
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			return tc.Text
		}
	}
	return ""
}

func TestServerListsAllTools(t *testing.T) {
	_, cs := newTestServer(t)

	listRes, err := cs.ListTools(context.Background(), &mcp.ListToolsParams{})
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(listRes.Tools) != 79 {
		t.Fatalf("tools count = %d, want 79", len(listRes.Tools))
	}
	for _, tool := range listRes.Tools {
		if tool.Name == "" {
			t.Fatalf("tool with empty name in list")
		}
		if tool.InputSchema == nil {
			t.Fatalf("tool %s: nil InputSchema", tool.Name)
		}
		if tool.Title == "" {
			t.Fatalf("tool %s: empty title", tool.Name)
		}
	}
	// 抽查 query 工具的 schema 必填字段
	var queryTool *mcp.Tool
	for _, tool := range listRes.Tools {
		if tool.Name == "query" {
			queryTool = tool
			break
		}
	}
	if queryTool == nil {
		t.Fatalf("query tool not found")
	}
	data, err := json.Marshal(queryTool.InputSchema)
	if err != nil {
		t.Fatalf("marshal query inputSchema: %v", err)
	}
	if !strings.Contains(string(data), `"required":["sql"]`) {
		t.Fatalf("query inputSchema missing required sql: %s", data)
	}
}

// TestOperationToolDispatch：工具分派链路（不依赖 DB）——存在/不存在工具、
// 畸形参数、handler panic 恢复。
func TestOperationToolDispatch(t *testing.T) {
	// handler panic 恢复：先注册一个会 panic 的测试工具（随服务器一起注册），
	// 确认包装器把 panic 转 IsError 结果、服务器不崩溃。
	registerPanicTool(t)
	_, cs := newTestServer(t)
	ctx := context.Background()

	// 未注册的工具：SDK 协议级错误（unknown tool），不是业务结果
	_, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "no_such_tool"})
	if err == nil {
		t.Fatalf("call nonexistent tool: expected protocol error")
	}

	// 畸形参数（JSON 数组而非对象）：业务错误且报"参数解析失败"，不崩溃
	res, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "query",
		Arguments: []any{"not", "an", "object"},
	})
	if err != nil {
		t.Fatalf("call with malformed args: %v", err)
	}
	if !res.IsError || !strings.Contains(resultText(res), "参数解析失败") {
		t.Fatalf("malformed args = isError:%v text:%q", res.IsError, resultText(res))
	}

	// 工具缺失必填参数：业务错误（"参数 sql 是必需的"）
	res, err = cs.CallTool(ctx, &mcp.CallToolParams{Name: "query"})
	if err != nil {
		t.Fatalf("call query without args: %v", err)
	}
	if !res.IsError || !strings.Contains(resultText(res), "sql 是必需的") {
		t.Fatalf("query without args = isError:%v text:%q", res.IsError, resultText(res))
	}

	// handler panic 恢复：调用 panic 测试工具，确认返回 IsError 而非崩溃
	res, err = cs.CallTool(ctx, &mcp.CallToolParams{Name: "panic_test_tool"})
	if err != nil {
		t.Fatalf("call panic tool: %v", err)
	}
	if !res.IsError {
		t.Fatalf("panic tool expected error result, got %q", resultText(res))
	}
	// 服务器仍然存活：再调一次未注册工具，应仍返回协议级错误而非崩溃
	_, err = cs.CallTool(ctx, &mcp.CallToolParams{Name: "no_such_tool_2"})
	if err == nil {
		t.Fatalf("server crashed after panic: expected protocol error")
	}
}

// TestQueryToolWorksWithDatabaseOrSkips：有可用 PostgreSQL 时验证 query 工具，
// 否则跳过（CI 无 PG 也能绿）。
func TestQueryToolWorksWithDatabaseOrSkips(t *testing.T) {
	_, cs := newTestServer(t)

	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "query",
		Arguments: map[string]any{"sql": "SELECT 1 AS one"},
	})
	if err != nil {
		t.Fatalf("query call: %v", err)
	}
	if res.IsError {
		t.Skipf("PostgreSQL 不可用，跳过：%s", resultText(res))
	}
	text := resultText(res)
	if !strings.Contains(text, `"one"`) || !strings.Contains(text, `"rows"`) {
		t.Fatalf("query result unexpected: %s", text)
	}
}
