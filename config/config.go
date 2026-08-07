package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
)

// Config 保存从环境变量加载的连接与安全配置。
// 单实例单连接模式：一个 MCP 进程只服务一个 PostgreSQL 数据库，
// 多库场景由 mcphub 编排多个实例（各自不同 env）。
type Config struct {
	Host             string
	Port             int
	User             string
	Password         string
	Database         string
	Schema           string
	SSLMode          string
	AccessMode       string // restricted | unrestricted
	StatementTimeout int    // 毫秒；0 表示不设置
	ConnectTimeout   int    // 秒
	BinPath          string // pg_dump / pg_restore / pg_ctl 等二进制所在目录
	RawDSN           string // PG_DSN 整体覆盖（非空时忽略其余连接参数）
}

var (
	cached     *Config
	loadOnce   sync.Once
)

// LoadConfig 读取 PG_* 环境变量。日志一律走 stderr（stdio 传输下 stdout 只能跑 JSON-RPC）。
func LoadConfig() *Config {
	loadOnce.Do(func() {
		cfg := &Config{
			Host:             getEnvOrDefault("PG_HOST", "localhost"),
			Port:             getEnvAsInt("PG_PORT", 5432),
			User:             getEnvOrDefault("PG_USER", "postgres"),
			Password:         getEnvOrDefault("PG_PASSWORD", ""),
			Database:         getEnvOrDefault("PG_DATABASE", "postgres"),
			Schema:           getEnvOrDefault("PG_SCHEMA", ""),
			SSLMode:          getEnvOrDefault("PG_SSLMODE", "prefer"),
			AccessMode:       strings.ToLower(getEnvOrDefault("PG_ACCESS_MODE", "restricted")),
			StatementTimeout: getEnvAsInt("PG_STATEMENT_TIMEOUT", 0),
			ConnectTimeout:   getEnvAsInt("PG_CONNECT_TIMEOUT", 10),
			BinPath:          getEnvOrDefault("PG_BIN_PATH", ""),
			RawDSN:           getEnvOrDefault("PG_DSN", ""),
		}
		if cfg.AccessMode != "restricted" && cfg.AccessMode != "unrestricted" {
			fmt.Fprintf(os.Stderr, "[警告] PG_ACCESS_MODE 的值 '%s' 无效，回退为 restricted\n", cfg.AccessMode)
			cfg.AccessMode = "restricted"
		}
		if cfg.AccessMode == "restricted" && cfg.StatementTimeout == 0 {
			cfg.StatementTimeout = 30000 // restricted 默认 30 秒语句超时
		}

		// 不打印密码
		fmt.Fprintf(os.Stderr, "[配置] 主机: %s, 端口: %d, 用户: %s, 数据库: %s, 模式: %s\n",
			cfg.Host, cfg.Port, cfg.User, cfg.Database, cfg.AccessMode)

		cached = cfg
	})
	return cached
}

// Get 返回已加载的配置（未加载则触发加载）。
func Get() *Config {
	return LoadConfig()
}

// IsRestricted 是否为受限只读模式。
func (c *Config) IsRestricted() bool {
	return c.AccessMode == "restricted"
}

// IsValid 校验必填项。
func (c *Config) IsValid() bool {
	if c.RawDSN != "" {
		return true
	}
	return c.Host != "" && c.Database != "" && c.Password != ""
}

// GetDSN 拼装连接串。
//
// runtime 参数（pgx 源码确认 search_path / statement_timeout /
// default_transaction_read_only 均可通过 query string 下发为会话默认值）：
//   - application_name：在 pg_stat_activity 中标识来源
//   - search_path：<schema>,pg_temp，收紧防止 schema 劫持（官方 libpq 文档建议）
//   - default_query_exec_mode=exec：pgx 专属参数，动态 SQL 场景规避 prepared
//     缓存的类型漂移错误
//   - restricted 模式追加 default_transaction_read_only=on：连接级强制只读
func (c *Config) GetDSN() string {
	if c.RawDSN != "" {
		return c.RawDSN
	}

	q := url.Values{}
	q.Set("sslmode", c.SSLMode)
	if c.ConnectTimeout > 0 {
		q.Set("connect_timeout", strconv.Itoa(c.ConnectTimeout))
	}
	q.Set("application_name", "pg-mcp")
	q.Set("default_query_exec_mode", "exec")
	if c.Schema != "" {
		q.Set("search_path", c.Schema+",pg_temp")
	}
	if c.StatementTimeout > 0 {
		q.Set("statement_timeout", strconv.Itoa(c.StatementTimeout))
	}
	if c.IsRestricted() {
		q.Set("default_transaction_read_only", "on")
	}

	u := url.URL{
		Scheme:   "postgresql",
		User:     url.UserPassword(c.User, c.Password),
		Host:     fmt.Sprintf("%s:%d", c.Host, c.Port),
		Path:     c.Database,
		RawQuery: q.Encode(),
	}
	return u.String()
}

// MaskedDSN 返回脱敏 DSN，用于错误信息/日志。
func (c *Config) MaskedDSN() string {
	if c.RawDSN != "" {
		if u, err := url.Parse(c.RawDSN); err == nil && u.User != nil {
			u.User = url.UserPassword(u.User.Username(), "***")
			return u.String()
		}
		return "***"
	}
	return fmt.Sprintf("postgresql://%s:***@%s:%d/%s", c.User, c.Host, c.Port, c.Database)
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvAsInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if v, err := strconv.Atoi(value); err == nil {
			return v
		}
		fmt.Fprintf(os.Stderr, "[警告] 环境变量 %s 的值 '%s' 不是有效整数，使用默认值 %d\n", key, value, defaultValue)
	}
	return defaultValue
}
