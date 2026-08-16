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

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	// 官方 go-sdk（协议 2026-07-28）：stateless _meta / server/discover /
	// subscriptions/listen 等新协议语义由 SDK 自动处理。
	// slog 输出到 stderr：stdio 传输下 stdout 只能跑 JSON-RPC。
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	// Capabilities 显式置空：不声明 2026-07-28 已弃用的 logging 能力；
	// tools 能力随工具注册自动推断（{"listChanged":true}）。
	s := mcp.NewServer(&mcp.Implementation{
		Name:    "postgres-database-mcp",
		Version: "3.1.0",
	}, &mcp.ServerOptions{
		Logger:       logger,
		Capabilities: &mcp.ServerCapabilities{},
	})

	registerOperationTools(s)

	log.Printf("PostgreSQL 数据库 MCP 服务器启动（协议 2026-07-28，79 个工具）")

	// Run 在客户端断开（stdio EOF）或 ctx 取消（SIGINT/SIGTERM）时返回，
	// 返回后统一清理连接池。
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	err := s.Run(ctx, &mcp.StdioTransport{})
	database.Close()
	if err != nil {
		log.Fatalf("服务器错误: %v", err)
	}
}
