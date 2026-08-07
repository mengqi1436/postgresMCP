# pg-mcp — PostgreSQL 数据库 MCP 服务器

基于 Go + [官方 MCP Go SDK](https://github.com/modelcontextprotocol/go-sdk)（协议 **2026-07-28**）+ [pgx v5](https://github.com/jackc/pgx) 的 PostgreSQL 数据库 MCP 服务器，与 `dm-mcp`（达梦版）架构对等：**单实例单库、stdio 传输、79 个工具、双安全模式**。

## 特性

- **最新 MCP 协议（2026-07-28）**：官方 SDK 实现 stateless 请求（`_meta` 携带协议版本/能力/身份）、`server/discover` 发现与版本协商、`subscriptions/listen` 变更通知流；`tools/list` 返回带 `title` 与完整 JSON Schema（`required`/类型/描述）的工具定义
- **stdio 传输**：作为 MCP 客户端（mcphub/Claude/ZCode 等）的子进程运行
- **原生 MCP 工具**：79 个操作工具直接经 `tools/list` 发现、`tools/call` 调用（协议原生方式，无中间分发层）
- **restricted / unrestricted 双安全模式**（`PG_ACCESS_MODE`）：
  - `restricted`（默认）：连接级 `default_transaction_read_only=on` + `statement_timeout` + 工具类别白名单（query/metadata/monitoring/explain_plan），只读防线三层
  - `unrestricted`：全能力，保留强制 WHERE、confirm 门禁、标识符校验、值参数化
- **pgxpool 原生连接池**：AfterConnect 钩子、健康检查；`rows.Values()` 通用扫描
- **numeric 精度保护**：自定义解码器把 `numeric` 列输出为十进制字符串，规避 float64 精度丢失
- **元数据查询全参数化**（`$n` 占位符），标识符正则校验 + 双引号引用，修复 dm 版 `'%s'` 插值注入面
- **PG 原生 COPY 协议导入 CSV**：服务器端解析类型，无需外部工具
- **官方文档最佳实践落地**：`pg_locks LEFT JOIN pg_stat_activity` + `pg_blocking_pids`、`EXPLAIN (FORMAT JSON)` + 写语句 `BEGIN;EXPLAIN ANALYZE;ROLLBACK` 范式、`pg_dump -Fc` / `pg_basebackup -X stream` + `pg_verifybackup` 校验

## 快速开始

```bash
# 构建
go build -o pg-mcp.exe .

# 配置（环境变量）
set PG_HOST=localhost
set PG_PORT=5432
set PG_USER=postgres
set PG_PASSWORD=yourpassword
set PG_DATABASE=postgres
set PG_ACCESS_MODE=restricted

# 运行（stdio）
pg-mcp.exe
```

## 环境变量

| 变量 | 默认值 | 说明 |
|---|---|---|
| `PG_HOST` | localhost | 主机 |
| `PG_PORT` | 5432 | 端口 |
| `PG_USER` | postgres | 用户 |
| `PG_PASSWORD` | （必填） | 密码 |
| `PG_DATABASE` | postgres | 数据库 |
| `PG_SCHEMA` | （空） | 默认 schema；设置后 search_path 收紧为 `<schema>,pg_temp` |
| `PG_SSLMODE` | prefer | disable/allow/prefer/require/verify-ca/verify-full |
| `PG_ACCESS_MODE` | restricted | restricted（只读）/ unrestricted（全能力） |
| `PG_STATEMENT_TIMEOUT` | restricted 时 30000 | 语句超时（毫秒） |
| `PG_CONNECT_TIMEOUT` | 10 | 连接超时（秒） |
| `PG_BIN_PATH` | （空） | pg_dump/pg_restore/pg_basebackup/pg_ctl 所在目录，缺省走 PATH |
| `PG_DATA_DIR` | （空） | 数据目录（pg_ctl 启停实例必需） |
| `PG_DSN` | （空） | 整体覆盖连接串（设置后忽略 PG_HOST 等连接参数；备份/实例工具仍需 PG_*） |

## mcphub 注册（本机实际配置）

本机运行的 mcphub 位于 `D:\Tool\mcphub`（计划任务 `MCPHub` 托管，开机自启），配置文件为
`D:\Tool\mcphub\config\mcp_settings.json`。已注册 `pg` 条目（unrestricted 模式）：

```json
"pg": {
  "type": "stdio",
  "command": "D:/MCP/postgresCLI/pg-mcp.exe",
  "args": [],
  "env": {
    "PG_HOST": "localhost",
    "PG_PORT": "5432",
    "PG_USER": "postgres",
    "PG_PASSWORD": "yourpassword",
    "PG_DATABASE": "postgres",
    "PG_SCHEMA": "public",
    "PG_ACCESS_MODE": "unrestricted",
    "PG_BIN_PATH": "D:/Tool/PostgreSQL17/bin",
    "PG_DATA_DIR": "D:/Tool/PostgreSQL17/data"
  },
  "options": { "resetTimeoutOnProgress": true },
  "visibility": "private",
  "owner": "admin",
  "enabled": true,
  "tools": {}
}
```

注意事项：
- **改配置后需重启 mcphub**（不支持热加载新 server）：`schtasks /Run /TN MCPHub`（先停旧进程）
- 经 mcphub 暴露的工具名带 **`pg-` 前缀**：`pg-query`、`pg-insert`、`pg-list_tables`…
- MCP 端点受 OAuth/bearer 保护：使用 mcp_settings.json 中 `bearerKeys` 的 system key
  （`Authorization: Bearer mcphub_...`），或走 OAuth 授权码流程
- 多库方案：mcphub 起多个实例、各自配不同 env（与 dm-mcp 一致）

## 文档

- [DESIGN.md](./DESIGN.md) — 架构与安全设计
- [TOOLS.md](./TOOLS.md) — 79 个工具完整清单

## 开发与测试

```bash
# 测试（测试钩子位于 //go:build test 文件，需 -tags test；生产 go build 不含任何测试钩子）
go test -tags test ./...
go vet -tags test ./...

# 变异测试（gremlins，需 -tags test）
gremlins unleash --tags test --timeout-coefficient 20 ./tools
```

## 与 dm-mcp 的关键差异

| 维度 | dm-mcp | pg-mcp |
|---|---|---|
| 驱动 | `gitee.com/chunanyong/dm`（database/sql） | pgx/v5 pgxpool 原生 |
| 占位符 | `:1, :2` | `$1, $2` |
| DDL 事务性 | 隐式提交，atomic 不可靠 | **DDL 可回滚，atomic=true 真原子**（VACUUM/CONCURRENTLY 例外） |
| upsert | `MERGE INTO` | `INSERT ... ON CONFLICT` |
| CSV 导入 | dmfldr 外部进程 | PG 原生 COPY 协议 |
| 只读模式 | 无 | restricted 三层防线 |
| schema 设置 | `ALTER SESSION`（仅首个连接） | DSN `search_path`（全池生效） |
| 日志 | 部分走 stdout | **全部 stderr**（stdio 协议安全） |
| numeric | float64 | 字符串（无精度丢失） |
