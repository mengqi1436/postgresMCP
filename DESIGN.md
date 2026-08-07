# pg-mcp 架构与安全设计

> 基于官方 MCP 文档（`/docs/2026-07-28/`）+ 官方 Go SDK（`modelcontextprotocol/go-sdk`）+ `jackc/pgx` 最佳实践落地。

## 1. 总体架构

```
MCP 客户端（mcphub / Claude / ZCode）
        │ stdio（JSON-RPC，stdout 专用）
        ▼
┌─────────────────────────────────────────┐
│ main.go                                  │  官方 go-sdk NewServer + Run（协议 2026-07-28）
│  ├─ 控制工具: pg_list_tools / pg_execute │  （handlers.go）
│  └─ 操作工具: 注册表动态注册              │  （operation_tools.go + schema.go）
├─────────────────────────────────────────┤
│ tools/                                   │
│  ├─ registry.go   注册中心（与 MCP 解耦） │  restricted 工具白名单在此执行
│  ├─ query/dml/ddl/metadata/...           │  81 个工具，init() 自注册
│  └─ binexec.go    外部二进制执行器        │  pg_dump / pg_ctl / pg_basebackup
├─────────────────────────────────────────┤
│ database/connection.go                   │  pgxpool 单例（MaxConns=10）
│  Query / Execute / ExecuteDDL /          │  rows.Values() 通用扫描
│  ExecuteTransaction / QueryInAbortedTx / │  numeric→string 解码
│  CopyFromReader                          │
├─────────────────────────────────────────┤
│ config/config.go                         │  PG_* env → DSN（含 runtime 参数）
└─────────────────────────────────────────┘
        │ pgx v5（扩展协议，default_query_exec_mode=exec）
        ▼
   PostgreSQL
```

**设计原则**（与 dm-mcp 一致）：
- **单实例单连接**：一个 MCP 进程只服务一个数据库；多库由 mcphub 编排多实例
- **工具与 MCP 解耦**：`tools.ToolHandler` 统一签名 `func(map[string]interface{}) (interface{}, error)`，既被 MCP 直接调用，也被 `pg_execute` 统一入口调用
- **统一返回**：成功返回 JSON（`json.MarshalIndent` → `textResult`）；业务错误 `errorResult`（IsError=true，会话继续）；系统故障 `return nil, err`（协议级错误）

## 2. 官方 SDK / MCP 协议（2026-07-28）落地

| 实践 | 出处 | 落地 |
|---|---|---|
| 官方 Go SDK | docs 2026-07-28 的 Go 教程 | `mcp.NewServer` + `s.AddTool`（mark3labs v0.57.0 已停更，最高仅 2025-11-25，故整体迁移） |
| stateless 请求 | spec 2026-07-28（SEP-2575） | 协议版本/客户端能力/身份经 `_meta` 随每个请求携带，SDK 自动处理，无初始化握手 |
| `server/discover` | spec server/discover | SDK 实现；返回 `supportedVersions`（2026-07-28…2024-11-05）与能力协商 |
| 变更通知 | spec subscriptions/listen | 工具注册即广告 `tools.listChanged:true`，SDK 在增删工具时自动推送 `notifications/tools/list_changed` |
| 工具 `title` + 完整 schema | spec tools | schema.go 集中映射：参数类型表 + `required` 表 + 描述表，`buildSchema` 生成 JSON Schema；`title` 由描述首句推导 |
| handler panic 不崩溃 | 原 WithRecovery 行为保留 | operation_tools.go handler 包装器内 `recover()`，panic 转 IsError 结果 |
| stdio 日志走 stderr | docs stdio 最佳实践 | `log` 包 + `slog`（TextHandler → stderr）；**禁用 fmt.Printf**（dm-mcp config 的隐患，不继承） |
| 错误分级 | spec tools/call | 业务错误 `errorResult`（IsError=true，Content 为错误文本）；系统故障 `return nil, err` |
| 生命周期 | SDK `Run` | `signal.NotifyContext` + `s.Run(ctx, &mcp.StdioTransport{})`：客户端断开或 SIGINT/SIGTERM 时返回，统一 `database.Close()` |

## 3. pgx 最佳实践落地

| 实践 | 出处 | 落地 |
|---|---|---|
| pgxpool 原生而非 database/sql | pgx 文档：AfterConnect 只在 pgxpool.Config、CopyFrom 只在原生 pgx、避免双层池 | connection.go 单例 pgxpool |
| DSN runtime 参数 | pgconn/config.go：search_path/statement_timeout/default_transaction_read_only 均可进 query string | config.GetDSN() |
| `default_query_exec_mode=exec` | pgx wiki：prepared 缓存对动态 SQL 有类型漂移风险 | DSN 固定参数 |
| AfterConnect 钩子 | pgxpool 文档："连接建立后、入池前" | 注册 numeric→string 解码器 |
| 池参数 | README 推荐 | MaxConns=10, MinConns=1, MaxConnLifetime=1h, MaxConnIdleTime=30m, HealthCheckPeriod=1m |
| 启动 `pool.Ping` | "canonical readiness check" | GetPool 初始化时执行 |
| `defer tx.Rollback(ctx)` 幂等事务 | pgx 事务文档 | ExecuteStatements / QueryInAbortedTx |
| COPY 流式导入 | pgconn.CopyFrom | CopyFromReader：CSV 字节流直推服务器，服务器端原生解析类型（等价 \copy） |
| numeric 精度 | pgx wiki："只能转 float64 或 string，不理想" | numericStringCodec 解码为十进制字符串（支持 binary/text 双格式） |
| 带未结束事务的连接释放即销毁 | pgxpool 释放语义 | 无需额外防护 |

## 4. 安全模型（三层防线）

### restricted 模式（默认）
1. **连接级**：DSN 追加 `default_transaction_read_only=on` —— 服务器强制拒绝一切写操作（官方机制，无法被工具绕过）
2. **超时**：`statement_timeout`（restricted 默认 30s；官方明确"不要全局设置，应会话级"）
3. **工具级**：registry.ExecuteTool 白名单 —— 仅放行 query/metadata/monitoring 类别 + explain_plan

### unrestricted 模式（继承 dm-mcp 规则）
- `update`/`delete` **强制 WHERE**（防全表误操作）
- 危险操作 **confirm=true 门禁**：`delete_database`、`physical_restore`
- **标识符校验**：正则 `^[A-Za-z_][A-Za-z0-9_$]*$`（≤63 字节）+ `quoteIdent` 双引号引用
- **值一律参数化**（`$n` 占位符）；`CREATE ROLE PASSWORD` 等无法参数化的位置用 `quoteLiteral`
- **DEFAULT 表达式白名单**：仅数字/`CURRENT_TIMESTAMP` 等关键字直出，其余按字符串引用
- **元数据查询全参数化**：消除 dm 版 `'%s'` 插值的注入面
- **search_path 防劫持**（libpq 官方建议）：`PG_SCHEMA` 设置时 search_path 收紧为 `<schema>,pg_temp`；元数据查询一律 `pg_catalog.`/`information_schema.` 全限定

### 日志安全
- 配置日志不打印密码；错误信息用 `MaskedDSN()`

## 5. PostgreSQL 官方要点落地

| 要点 | 落地 |
|---|---|
| DDL 可事务回滚（tutorial 官方原文） | batch_* / execute_transaction / batch_execute_sql 的 atomic=true 为真原子；工具描述注明例外：VACUUM、CREATE INDEX CONCURRENTLY、REINDEX CONCURRENTLY |
| 序列 nextval 不回滚 | 文档注明（sequence 相关操作） |
| COPY 新式括号选项语法 | batch_import_csv 生成 `WITH (FORMAT csv, HEADER true, ...)` |
| pg_locks 官方关联查询 | lock_info：`pg_locks LEFT JOIN pg_stat_activity` + `pg_blocking_pids()` |
| pg_stat_statements 需扩展 | slow_queries 优雅降级返回安装指引 |
| pg_stat_activity.query 截断 1024 字节 | 工具结果注明 |
| CREATE USER = CREATE ROLE LOGIN | create_user 生成 `CREATE ROLE ... LOGIN PASSWORD` |
| EXPLAIN FORMAT JSON | explain_plan；analyze 写语句包 `BEGIN;...;ROLLBACK`（QueryInAbortedTx，官方范式） |
| pg_dump -Fc / pg_restore --clean --create | logical_export/import；支持 -j 并行、--section 分段 |
| pg_basebackup -X stream + pg_verifybackup | physical_backup 默认备份后自动校验；physical_restore 仅预检+步骤指引（不自动替换数据目录） |
| statement_timeout 会话级 | DSN runtime 参数（每连接生效） |

## 6. 目录结构

```
D:\MCP\postgresCLI\
├── go.mod                  # module pg-mcp（官方 go-sdk v1.7.0 + pgx v5.10.0 + jsonschema-go）
├── main.go                 # stdio 入口 + 控制工具注册 + 信号感知 Run 生命周期
├── handlers.go             # textResult/errorResult + pg_list_tools / pg_execute
├── operation_tools.go      # 注册表 → MCP 动态注册 + handler 包装（含 panic 恢复）
├── schema.go               # 参数类型/required/描述/title 集中映射 + buildSchema
├── schema_test.go          # schema 生成器单元测试
├── server_test.go          # in-memory transport 集成测试（tools/list + 控制工具调用）
├── config/config.go        # PG_* env + DSN 拼装（runtime 参数）
├── database/connection.go  # pgxpool 单例 + 扫描层 + numeric 解码器 + 6 个执行原语
├── tools/
│   ├── registry.go         # 注册中心 + restricted 白名单
│   ├── utils.go            # 参数提取 / 标识符校验 / 引用
│   ├── query.go            # query(5)
│   ├── execute.go          # dml(7)
│   ├── ddl.go              # ddl 表/索引(10)
│   ├── schema_objects.go   # 视图/序列(4)
│   ├── metadata.go         # 元数据(17)
│   ├── advanced.go         # execute_sql/explain(5)
│   ├── batch_ops.go        # batch_execute_sql(1)
│   ├── admin.go            # 用户/角色/表空间(12)
│   ├── monitoring.go       # 会话/锁/慢查询(6)
│   ├── binexec.go          # 外部二进制执行器
│   ├── backup.go           # 逻辑/物理备份(4)
│   ├── import_csv.go       # COPY 导入/导出(2)
│   └── instance.go         # 建库/启停(6)
├── .github/workflows/release.yml  # test(vet+test) → build → release
├── pg-mcp.exe              # 编译产物
└── README.md / DESIGN.md / TOOLS.md
```

## 7. 命名规范

- 工具/参数 snake_case、动词_名词；批量变体 `batch_` 前缀
- 控制工具 `pg_` 前缀；操作工具与 dm-mcp 同名对应（便于 Agent 提示词复用）

## 8. 测试与 CI

- **schema_test.go**（纯单元，无需 DB）：schema 类型/required/描述/title 断言；全部注册工具 schema 完整性校验（每个 Params 都有对应 property，required 都在 Params 内）
- **server_test.go**（官方 SDK `NewInMemoryTransports` 集成）：connect → `tools/list`（81 个工具、title/schema 非空）→ 控制工具调用（`pg_list_tools` / `pg_execute` 分派链路）；依赖 DB 的 `query` 用例在无 PG 环境自动 `t.Skip`
- CI（release.yml `test` job）：`go vet ./...` + `go test ./...`（无需 PG 即可绿），通过后才进入 build/release
