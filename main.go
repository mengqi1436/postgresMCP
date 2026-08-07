package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"pg-mcp/database"
	_ "pg-mcp/tools"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func main() {
	s := server.NewMCPServer(
		"postgres-database-mcp",
		"1.0.0",
		server.WithToolCapabilities(true),
		server.WithRecovery(), // handler panic 不崩溃服务器（官方生产建议）
	)

	registerControlTools(s)
	registerOperationTools(s)

	// Go log 包默认输出到 stderr；stdio 传输下 stdout 只能跑 JSON-RPC。
	log.Printf("PostgreSQL 数据库 MCP 服务器启动")
	log.Printf("使用 pg_list_tools 查看所有可用工具")
	log.Printf("使用 pg_execute 执行指定工具")

	// ServeStdio 跑在 goroutine，主 goroutine 等信号：
	// stdin EOF（父进程退出）或 SIGINT/SIGTERM 都会走到清理逻辑。
	errCh := make(chan error, 1)
	go func() {
		errCh <- server.ServeStdio(s)
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-errCh:
		database.Close()
		if err != nil {
			log.Fatalf("服务器错误: %v", err)
		}
	case sig := <-sigCh:
		log.Printf("收到信号 %v，正在关闭...", sig)
		database.Close()
	}
}

func registerControlTools(s *server.MCPServer) {
	s.AddTool(
		mcp.NewTool("pg_list_tools",
			mcp.WithDescription("列出所有可用的 PostgreSQL 数据库操作工具。可按类别筛选：query/dml/ddl/metadata/advanced/admin/monitoring/backup/import/instance"),
			mcp.WithString("category",
				mcp.Description("筛选类别（可选）: query, dml, ddl, metadata, advanced, admin, monitoring, backup, import, instance"),
			),
		),
		handleListTools,
	)

	s.AddTool(
		mcp.NewTool("pg_execute",
			mcp.WithDescription("执行指定的 PostgreSQL 数据库操作工具"),
			mcp.WithString("tool_name",
				mcp.Required(),
				mcp.Description("要执行的工具名称"),
			),
			mcp.WithObject("params",
				mcp.Description("工具参数（JSON对象）"),
			),
		),
		handleExecute,
	)
}
