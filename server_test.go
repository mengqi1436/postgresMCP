package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"pg-mcp/tools"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// 注册全部工具（79 个操作工具 + 2 个控制工具）的测试用服务器。
func newTestServer(t *testing.T) (*mcp.Server, *mcp.ClientSession) {
	t.Helper()
	server := mcp.NewServer(&mcp.Implementation{Name: "test-server", Version: "0.0.1"}, nil)
	registerControlTools(server)
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
	if len(listRes.Tools) != 81 {
		t.Fatalf("tools count = %d, want 81", len(listRes.Tools))
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

func TestCallControlTools(t *testing.T) {
	_, cs := newTestServer(t)
	ctx := context.Background()

	// pg_list_tools：无需数据库，返回内部注册表工具目录 JSON
	// （只含 79 个操作工具，不含 pg_list_tools/pg_execute 两个控制工具）
	res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "pg_list_tools"})
	if err != nil {
		t.Fatalf("pg_list_tools call: %v", err)
	}
	if res.IsError {
		t.Fatalf("pg_list_tools returned error: %s", resultText(res))
	}
	var listResp struct {
		Total int `json:"total"`
	}
	if err := json.Unmarshal([]byte(resultText(res)), &listResp); err != nil {
		t.Fatalf("parse pg_list_tools result: %v", err)
	}
	if want := len(tools.GetAllTools("")); listResp.Total != want {
		t.Fatalf("pg_list_tools total = %d, want %d", listResp.Total, want)
	}

	// pg_execute 缺 tool_name：业务错误
	res, err = cs.CallTool(ctx, &mcp.CallToolParams{Name: "pg_execute"})
	if err != nil {
		t.Fatalf("pg_execute(empty) call: %v", err)
	}
	if !res.IsError || !strings.Contains(resultText(res), "tool_name 是必需的") {
		t.Fatalf("pg_execute(empty) = isError:%v text:%q", res.IsError, resultText(res))
	}

	// pg_execute 指向不存在的工具：业务错误
	res, err = cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "pg_execute",
		Arguments: map[string]any{"tool_name": "no_such_tool"},
	})
	if err != nil {
		t.Fatalf("pg_execute(nonexistent) call: %v", err)
	}
	if !res.IsError || !strings.Contains(resultText(res), "不存在") {
		t.Fatalf("pg_execute(nonexistent) = isError:%v text:%q", res.IsError, resultText(res))
	}

	// pg_execute 分派到真实注册工具（database_service_status 不依赖 DB 连接）：
	// 默认 restricted 模式或缺少 PG_DATA_DIR 都会得到确定的业务错误，但证明分派链路可用。
	res, err = cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "pg_execute",
		Arguments: map[string]any{"tool_name": "database_service_status"},
	})
	if err != nil {
		t.Fatalf("pg_execute(database_service_status) call: %v", err)
	}
	if !res.IsError {
		t.Fatalf("pg_execute(database_service_status) expected error, got %q", resultText(res))
	}
	if strings.Contains(resultText(res), "不存在") {
		t.Fatalf("pg_execute dispatch failed: %q", resultText(res))
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
