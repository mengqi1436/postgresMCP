package tools

import (
	"errors"
	"testing"

	"github.com/pashagolub/pgxmock/v5"
)

func TestHandleActiveSessions(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectQuery("pg_stat_activity").
		WithArgs("active").
		WillReturnRows(pgxmock.NewRows([]string{"pid"}).AddRow(100))

	out, err := handleActiveSessions(map[string]interface{}{"state": "active", "limit": 10})
	if err != nil {
		t.Fatalf("handleActiveSessions: %v", err)
	}
	if res := out.(map[string]interface{}); res["count"] != 1 {
		t.Fatalf("count = %v", res["count"])
	}
}

func TestHandleLockInfo(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectQuery("pg_locks").
		WillReturnRows(pgxmock.NewRows([]string{"waiting_pid", "blocker_pid"}).AddRow(100, 200))

	out, err := handleLockInfo(map[string]interface{}{})
	if err != nil {
		t.Fatalf("handleLockInfo: %v", err)
	}
	if res := out.(map[string]interface{}); res["count"] != 1 {
		t.Fatalf("count = %v", res["count"])
	}
}

func TestHandleSlowQueries(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectQuery("pg_stat_statements").
		WillReturnRows(pgxmock.NewRows([]string{"query", "calls"}).AddRow("SELECT 1", 5))

	out, err := handleSlowQueries(map[string]interface{}{})
	if err != nil {
		t.Fatalf("handleSlowQueries: %v", err)
	}
	res := out.(map[string]interface{})
	if res["available"] != true || res["count"] != 1 {
		t.Fatalf("result = %v", res)
	}
}

func TestHandleSlowQueriesExtensionMissing(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectQuery("pg_stat_statements").
		WillReturnError(errors.New(`ERROR: relation "pg_stat_statements" does not exist`))

	out, err := handleSlowQueries(map[string]interface{}{})
	if err != nil {
		t.Fatalf("扩展缺失应优雅返回: %v", err)
	}
	res := out.(map[string]interface{})
	if res["available"] != false || res["hint"] == nil {
		t.Fatalf("result = %v", res)
	}
}

func TestHandleTablespaceUsage(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectQuery("pg_tablespace").
		WillReturnRows(pgxmock.NewRows([]string{"tablespace_name"}).AddRow("pg_default"))
	mock.ExpectQuery("pg_database").
		WillReturnRows(pgxmock.NewRows([]string{"database_name"}).AddRow("postgres"))

	out, err := handleTablespaceUsage(map[string]interface{}{})
	if err != nil {
		t.Fatalf("handleTablespaceUsage: %v", err)
	}
	res := out.(map[string]interface{})
	if res["tablespaces"] == nil || res["databases"] == nil {
		t.Fatalf("result = %v", res)
	}
}

func TestHandleInstanceParameters(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectQuery("pg_settings").
		WithArgs("%work_mem%").
		WillReturnRows(pgxmock.NewRows([]string{"name"}).AddRow("work_mem"))

	out, err := handleInstanceParameters(map[string]interface{}{"name": "work_mem"})
	if err != nil {
		t.Fatalf("handleInstanceParameters: %v", err)
	}
	if res := out.(map[string]interface{}); res["count"] != 1 {
		t.Fatalf("count = %v", res["count"])
	}
}

func TestHandleSessionMemory(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectQuery("pg_backend_memory_contexts").
		WillReturnRows(pgxmock.NewRows([]string{"name"}).AddRow("TopMemoryContext"))

	out, err := handleSessionMemory(map[string]interface{}{})
	if err != nil {
		t.Fatalf("handleSessionMemory: %v", err)
	}
	if res := out.(map[string]interface{}); res["available"] != true {
		t.Fatalf("result = %v", res)
	}
}

func TestHandleSessionMemoryPG14Missing(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectQuery("pg_backend_memory_contexts").
		WillReturnError(errors.New(`ERROR: relation "pg_backend_memory_contexts" does not exist`))

	out, err := handleSessionMemory(map[string]interface{}{})
	if err != nil {
		t.Fatalf("PG14 缺失应优雅返回: %v", err)
	}
	if res := out.(map[string]interface{}); res["available"] != false {
		t.Fatalf("result = %v", res)
	}
}

// 以下测试针对 limit 钳制边界变异（monitoring.go 58/90/129/219）：
// 传入越界 limit，断言生成的 SQL 使用了钳制后的值。
func TestHandleActiveSessionsLimitClampHigh(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectQuery(`LIMIT 50`).WillReturnRows(pgxmock.NewRows([]string{"pid"}))
	if _, err := handleActiveSessions(map[string]interface{}{"limit": 1000}); err != nil {
		t.Fatalf("limit=1000 应钳制为 50: %v", err)
	}
}

func TestHandleActiveSessionsLimitClampLow(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectQuery(`LIMIT 50`).WillReturnRows(pgxmock.NewRows([]string{"pid"}))
	if _, err := handleActiveSessions(map[string]interface{}{"limit": 0}); err != nil {
		t.Fatalf("limit=0 应钳制为 50: %v", err)
	}
}

func TestHandleLockInfoLimitClamp(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectQuery(`LIMIT 50`).WillReturnRows(pgxmock.NewRows([]string{"waiting_pid"}))
	if _, err := handleLockInfo(map[string]interface{}{"limit": -5}); err != nil {
		t.Fatalf("limit=-5 应钳制为 50: %v", err)
	}
}

func TestHandleSlowQueriesLimitClamp(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectQuery(`LIMIT 20`).WillReturnRows(pgxmock.NewRows([]string{"query"}))
	if _, err := handleSlowQueries(map[string]interface{}{"limit": 9999}); err != nil {
		t.Fatalf("limit=9999 应钳制为 20: %v", err)
	}
}

// 边界精确值测试：limit=1 与 limit=500 不触发钳制（杀 CONDITIONALS_BOUNDARY）。
func TestHandleActiveSessionsLimitBoundary(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectQuery(`LIMIT 1\b`).WillReturnRows(pgxmock.NewRows([]string{"pid"}))
	if _, err := handleActiveSessions(map[string]interface{}{"limit": 1}); err != nil {
		t.Fatalf("limit=1 不应钳制: %v", err)
	}

	mock2 := newMockPool(t)
	mock2.ExpectQuery(`LIMIT 500\b`).WillReturnRows(pgxmock.NewRows([]string{"pid"}))
	if _, err := handleActiveSessions(map[string]interface{}{"limit": 500}); err != nil {
		t.Fatalf("limit=500 不应钳制: %v", err)
	}
}

func TestHandleLockInfoLimitBoundary(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectQuery(`LIMIT 500\b`).WillReturnRows(pgxmock.NewRows([]string{"waiting_pid"}))
	if _, err := handleLockInfo(map[string]interface{}{"limit": 500}); err != nil {
		t.Fatalf("limit=500 不应钳制: %v", err)
	}
}

func TestHandleSlowQueriesLimitBoundary(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectQuery(`LIMIT 200\b`).WillReturnRows(pgxmock.NewRows([]string{"query"}))
	if _, err := handleSlowQueries(map[string]interface{}{"limit": 200}); err != nil {
		t.Fatalf("limit=200 不应钳制: %v", err)
	}
}

func TestKillTablespaceDatabases(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectQuery("pg_tablespace").WillReturnRows(pgxmock.NewRows([]string{"tablespace_name"}).AddRow("ts"))
	mock.ExpectQuery("pg_database").WillReturnRows(pgxmock.NewRows([]string{"database_name"}).AddRow("db"))
	out, err := handleTablespaceUsage(map[string]interface{}{})
	if err != nil {
		t.Fatal(err)
	}
	m := assertMap(t, out)
	dbs, ok := m["databases"].([]map[string]interface{})
	if !ok || len(dbs) != 1 {
		t.Fatalf("databases=%#v", m["databases"])
	}
}

// ---- len(results)>0 边界变异：空结果时 results[0] 触发 panic ----

func TestKillLockInfoLimitOne(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectQuery(`LIMIT 1\b`).WillReturnRows(pgxmock.NewRows([]string{"waiting_pid"}))
	if _, err := handleLockInfo(map[string]interface{}{"limit": 1}); err != nil {
		t.Fatalf("limit=1 不应钳制: %v", err)
	}
}

func TestKillSlowQueriesLimitOne(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectQuery(`LIMIT 1\b`).WillReturnRows(pgxmock.NewRows([]string{"query"}))
	if _, err := handleSlowQueries(map[string]interface{}{"limit": 1}); err != nil {
		t.Fatalf("limit=1 不应钳制: %v", err)
	}
}

func TestKillSessionMemoryLimitBoundary(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectQuery(`LIMIT 1\b`).WillReturnRows(pgxmock.NewRows([]string{"name"}))
	if _, err := handleSessionMemory(map[string]interface{}{"limit": 1}); err != nil {
		t.Fatalf("limit=1 不应钳制: %v", err)
	}
	mock2 := newMockPool(t)
	mock2.ExpectQuery(`LIMIT 200\b`).WillReturnRows(pgxmock.NewRows([]string{"name"}))
	if _, err := handleSessionMemory(map[string]interface{}{"limit": 200}); err != nil {
		t.Fatalf("limit=200 不应钳制: %v", err)
	}
}
