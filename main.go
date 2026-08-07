package main

import (
	"context"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"pg-mcp/database"
	_ "pg-mcp/tools"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	// 官方 go-sdk（协议 2026-07-28）：stateless _meta / server/discover /
	// subscriptions/listen 等新协议语义由 SDK 自动处理。
	// slog 输出到 stderr：stdio 传输下 stdout 只能跑 JSON-RPC。
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	s := mcp.NewServer(&mcp.Implementation{
		Name:    "postgres-database-mcp",
		Version: "2.0.0",
	}, &mcp.ServerOptions{Logger: logger})

	registerControlTools(s)
	registerOperationTools(s)

	log.Printf("PostgreSQL 数据库 MCP 服务器启动（协议 2026-07-28）")
	log.Printf("使用 pg_list_tools 查看所有可用工具")
	log.Printf("使用 pg_execute 执行指定工具")

	// Run 在客户端断开（stdio EOF）或 ctx 取消（SIGINT/SIGTERM）时返回，
	// 取代旧的 goroutine + select 生命周期；返回后统一清理连接池。
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	err := s.Run(ctx, &mcp.StdioTransport{})
	database.Close()
	if err != nil {
		log.Fatalf("服务器错误: %v", err)
	}
}

func registerControlTools(s *mcp.Server) {
	s.AddTool(&mcp.Tool{
		Name:        "pg_list_tools",
		Title:       "列出所有可用工具",
		Description: "列出所有可用的 PostgreSQL 数据库操作工具。可按类别筛选：query/dml/ddl/metadata/advanced/admin/monitoring/backup/import/instance",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"category": {
					Type:        "string",
					Description: "筛选类别（可选）: query, dml, ddl, metadata, advanced, admin, monitoring, backup, import, instance",
				},
			},
		},
	}, handleListTools)

	s.AddTool(&mcp.Tool{
		Name:        "pg_execute",
		Title:       "执行指定工具",
		Description: "执行指定的 PostgreSQL 数据库操作工具",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"tool_name": {
					Type:        "string",
					Description: "要执行的工具名称",
				},
				"params": {
					Type:        "object",
					Description: "工具参数（JSON对象）",
				},
			},
			Required: []string{"tool_name"},
		},
	}, handleExecute)
}
