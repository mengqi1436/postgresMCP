# @pg-mcp/server

PostgreSQL 数据库 MCP 服务器（Go 二进制包装，stdio 传输）。

本包是 [postgresMCP](https://github.com/mengqi1436/postgresMCP) 的 npm 发行版：
安装时按平台自动从 GitHub Releases 下载对应版本的原生 Go 二进制，
通过 `pg-mcp` 命令以 stdio 方式作为 MCP 客户端（Claude / mcphub / ZCode 等）的子进程运行。

- 协议：MCP 2026-07-28（官方 Go SDK）
- 传输：stdio
- 工具：79 个操作工具
- 安全：`restricted`（默认只读）/ `unrestricted` 双模式

## 支持平台

| 平台 | 架构 |
|---|---|
| Linux | x64 |
| Windows | x64 |

> 本包在 `package.json` 的 `os`/`cpu` 中声明了上述范围；不支持的平台可安装但无可用二进制。

## 安装

```bash
npm install -g @pg-mcp/server
# 或局部安装
npm install @pg-mcp/server
```

安装时会自动执行 `postinstall` 下载对应平台二进制。

## 配置（环境变量）

| 变量 | 默认值 | 说明 |
|---|---|---|
| `PG_HOST` | localhost | 主机 |
| `PG_PORT` | 5432 | 端口 |
| `PG_USER` | postgres | 用户 |
| `PG_PASSWORD` | （必填） | 密码 |
| `PG_DATABASE` | postgres | 数据库 |
| `PG_SCHEMA` | （空） | 默认 schema |
| `PG_SSLMODE` | prefer | disable/allow/prefer/require/verify-ca/verify-full |
| `PG_ACCESS_MODE` | restricted | restricted（只读）/ unrestricted（全能力） |
| `PG_STATEMENT_TIMEOUT` | restricted 时 30000 | 语句超时（毫秒） |
| `PG_CONNECT_TIMEOUT` | 10 | 连接超时（秒） |
| `PG_DSN` | （空） | 直接使用连接串，优先级高于分项变量 |

## 在 MCP 客户端中使用

以 Claude Desktop（`claude_desktop_config.json`）为例：

```json
{
  "mcpServers": {
    "pg-mcp": {
      "command": "pg-mcp",
      "env": {
        "PG_HOST": "localhost",
        "PG_PORT": "5432",
        "PG_USER": "postgres",
        "PG_PASSWORD": "yourpassword",
        "PG_DATABASE": "postgres",
        "PG_ACCESS_MODE": "restricted"
      }
    }
  }
}
```

使用 `npx`（局部安装）时：

```json
{
  "mcpServers": {
    "pg-mcp": {
      "command": "npx",
      "args": ["-y", "@pg-mcp/server"],
      "env": { "PG_HOST": "localhost", "PG_PASSWORD": "yourpassword" }
    }
  }
}
```

## 版本对齐

npm 版本号与 GitHub Release 的 tag 对齐：`@pg-mcp/server@2.2.0`
对应 [GitHub Release `v2.2.0`](https://github.com/mengqi1436/postgresMCP/releases)，
postinstall 从 `releases/download/v2.2.0/` 下载完全一致的二进制。

## 命令行

```bash
pg-mcp            # 以 stdio 模式启动（供 MCP 客户端调用）
pg-mcp --help     # 查看二进制自身参数
```

## 开发 / 发布

- 发布流程见仓库 `.github/workflows/release.yml`：push `v*` tag 时 CI 自动发布 GitHub Release 与 npm 包。
- 仓库根目录的 `npm/` 是本包装包的源码。
