package tools

import (
	"fmt"
	"strings"

	"pg-mcp/database"
)

func init() { registerMonitoringTools() }

func registerMonitoringTools() {
	RegisterTool(ToolInfo{
		Name:        "active_sessions",
		Category:    "monitoring",
		Description: "查看当前活跃会话（pg_stat_activity）。参数: limit(可选, 默认50), state(可选, 如 active/idle/idle in transaction)",
		Params:      []string{"limit", "state"},
	}, handleActiveSessions)

	RegisterTool(ToolInfo{
		Name:        "lock_info",
		Category:    "monitoring",
		Description: "查看当前锁等待与阻塞关系（pg_locks 关联 pg_stat_activity，官方推荐模式）。参数: limit(可选, 默认50)",
		Params:      []string{"limit"},
	}, handleLockInfo)

	RegisterTool(ToolInfo{
		Name:        "slow_queries",
		Category:    "monitoring",
		Description: "慢查询统计（需 pg_stat_statements 扩展，未安装时返回安装指引）。参数: limit(可选, 默认20)",
		Params:      []string{"limit"},
	}, handleSlowQueries)

	RegisterTool(ToolInfo{
		Name:        "tablespace_usage",
		Category:    "monitoring",
		Description: "查看表空间与各数据库空间占用。无参数",
		Params:      []string{},
	}, handleTablespaceUsage)

	RegisterTool(ToolInfo{
		Name:        "instance_parameters",
		Category:    "monitoring",
		Description: "查看实例参数（pg_settings）。参数: name(可选, 模糊搜索; 不传返回常用参数)",
		Params:      []string{"name"},
	}, handleInstanceParameters)

	RegisterTool(ToolInfo{
		Name:        "session_memory",
		Category:    "monitoring",
		Description: "查看当前会话内存上下文（pg_backend_memory_contexts，PG14+）。参数: limit(可选, 默认30)",
		Params:      []string{"limit"},
	}, handleSessionMemory)
}

func handleActiveSessions(params map[string]interface{}) (interface{}, error) {
	limit := getIntDefault(params, "limit", 50)
	if limit < 1 || limit > 500 {
		limit = 50
	}

	ctx, cancel := toolContext()
	defer cancel()

	sqlStr := `
SELECT a.pid, a.datname AS database, a.usename AS username,
       a.application_name, a.client_addr::text AS client_addr,
       a.state, a.backend_start, a.xact_start, a.query_start,
       a.wait_event_type, a.wait_event,
       left(a.query, 1024) AS query
FROM pg_catalog.pg_stat_activity a
WHERE a.pid <> pg_catalog.pg_backend_pid()`

	var args []interface{}
	if state := getString(params, "state"); state != "" {
		args = append(args, state)
		sqlStr += fmt.Sprintf(" AND a.state = $%d", len(args))
	}
	sqlStr += fmt.Sprintf(" ORDER BY a.query_start NULLS LAST LIMIT %d", limit)

	rows, err := database.Query(ctx, sqlStr, args...)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{"sessions": rows, "count": len(rows)}, nil
}

func handleLockInfo(params map[string]interface{}) (interface{}, error) {
	limit := getIntDefault(params, "limit", 50)
	if limit < 1 || limit > 500 {
		limit = 50
	}

	ctx, cancel := toolContext()
	defer cancel()

	// 官方推荐模式：pg_locks LEFT JOIN pg_stat_activity；阻塞链用 pg_blocking_pids
	sqlStr := fmt.Sprintf(`
SELECT pl.pid AS waiting_pid,
       bp.blocker_pid,
       pl.locktype, pl.mode AS requested_mode, bl.mode AS blocker_mode,
       COALESCE(pl.relation::regclass::text, pl.virtualxid, '') AS resource,
       psa_wait.usename AS waiting_user, left(psa_wait.query, 300) AS waiting_query,
       psa_blk.usename AS blocker_user, left(psa_blk.query, 300) AS blocker_query,
       pl.waitstart
FROM pg_catalog.pg_locks pl
CROSS JOIN LATERAL unnest(pg_catalog.pg_blocking_pids(pl.pid)) AS bp(blocker_pid)
LEFT JOIN pg_catalog.pg_locks bl
       ON bl.pid = bp.blocker_pid AND bl.locktype = pl.locktype
      AND bl.relation IS NOT DISTINCT FROM pl.relation AND bl.granted
LEFT JOIN pg_catalog.pg_stat_activity psa_wait ON psa_wait.pid = pl.pid
LEFT JOIN pg_catalog.pg_stat_activity psa_blk ON psa_blk.pid = bp.blocker_pid
WHERE NOT pl.granted AND pl.pid <> pg_catalog.pg_backend_pid()
ORDER BY pl.waitstart NULLS LAST
LIMIT %d`, limit)

	rows, err := database.Query(ctx, sqlStr)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"blocking_pairs": rows, "count": len(rows),
		"note": "waiting_pid 被 blocker_pid 阻塞；query 列受 track_activity_query_size 限制（默认截断 1024 字节）",
	}, nil
}

func handleSlowQueries(params map[string]interface{}) (interface{}, error) {
	limit := getIntDefault(params, "limit", 20)
	if limit < 1 || limit > 200 {
		limit = 20
	}

	ctx, cancel := toolContext()
	defer cancel()

	sqlStr := fmt.Sprintf(`
SELECT s.query, s.calls,
       round(s.total_exec_time::numeric, 2) AS total_ms,
       round(s.mean_exec_time::numeric, 2) AS mean_ms,
       round(s.max_exec_time::numeric, 2) AS max_ms,
       s.rows,
       round((100.0 * s.shared_blks_hit / NULLIF(s.shared_blks_hit + s.shared_blks_read, 0))::numeric, 2) AS hit_percent
FROM pg_stat_statements s
ORDER BY s.total_exec_time DESC
LIMIT %d`, limit)

	rows, err := database.Query(ctx, sqlStr)
	if err != nil {
		if strings.Contains(err.Error(), "pg_stat_statements") && strings.Contains(err.Error(), "does not exist") {
			return map[string]interface{}{
				"available": false,
				"hint":      "pg_stat_statements 扩展未安装：请执行 CREATE EXTENSION pg_stat_statements（需在 shared_preload_libraries 中配置并重启实例）",
			}, nil
		}
		return nil, err
	}
	return map[string]interface{}{"available": true, "queries": rows, "count": len(rows)}, nil
}

func handleTablespaceUsage(params map[string]interface{}) (interface{}, error) {
	ctx, cancel := toolContext()
	defer cancel()

	tablespaces, err := database.Query(ctx, `
SELECT t.spcname AS tablespace_name,
       pg_catalog.pg_size_pretty(pg_catalog.pg_tablespace_size(t.oid)) AS size
FROM pg_catalog.pg_tablespace t
ORDER BY pg_catalog.pg_tablespace_size(t.oid) DESC`)
	if err != nil {
		return nil, err
	}

	databases, err := database.Query(ctx, `
SELECT d.datname AS database_name,
       pg_catalog.pg_size_pretty(pg_catalog.pg_database_size(d.oid)) AS size
FROM pg_catalog.pg_database d
WHERE d.datistemplate = false
ORDER BY pg_catalog.pg_database_size(d.oid) DESC`)
	if err != nil {
		databases = nil
	}

	return map[string]interface{}{"tablespaces": tablespaces, "databases": databases}, nil
}

func handleInstanceParameters(params map[string]interface{}) (interface{}, error) {
	ctx, cancel := toolContext()
	defer cancel()

	sqlStr := `
SELECT s.name, s.setting, s.unit, s.category, s.source, left(s.short_desc, 200) AS description
FROM pg_catalog.pg_settings s`

	var args []interface{}
	if name := getString(params, "name"); name != "" {
		args = append(args, "%"+name+"%")
		sqlStr += fmt.Sprintf(" WHERE s.name LIKE $%d", len(args))
	} else {
		// 不传 name 时返回常用参数，避免一次返回全部 ~400 行
		sqlStr += ` WHERE s.name IN (
			'max_connections','shared_buffers','work_mem','maintenance_work_mem',
			'effective_cache_size','wal_buffers','max_wal_size','min_wal_size',
			'statement_timeout','lock_timeout','idle_in_transaction_session_timeout',
			'default_transaction_read_only','search_path','max_parallel_workers',
			'max_parallel_workers_per_gather','autovacuum','track_activity_query_size',
			'timezone','server_version','listen_addresses','port','max_worker_processes')`
	}
	sqlStr += " ORDER BY s.name"

	rows, err := database.Query(ctx, sqlStr, args...)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{"parameters": rows, "count": len(rows)}, nil
}

func handleSessionMemory(params map[string]interface{}) (interface{}, error) {
	limit := getIntDefault(params, "limit", 30)
	if limit < 1 || limit > 200 {
		limit = 30
	}

	ctx, cancel := toolContext()
	defer cancel()

	sqlStr := fmt.Sprintf(`
SELECT m.name, m.ident, m.level,
       m.total_bytes, m.used_bytes, m.free_bytes
FROM pg_backend_memory_contexts m
ORDER BY m.total_bytes DESC
LIMIT %d`, limit)

	rows, err := database.Query(ctx, sqlStr)
	if err != nil {
		if strings.Contains(err.Error(), "pg_backend_memory_contexts") {
			return map[string]interface{}{
				"available": false,
				"hint":      "pg_backend_memory_contexts 需要 PostgreSQL 14+",
			}, nil
		}
		return nil, err
	}
	return map[string]interface{}{"available": true, "memory_contexts": rows, "count": len(rows)}, nil
}
