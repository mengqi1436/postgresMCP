package tools

import (
	"testing"

	"github.com/pashagolub/pgxmock/v5"
)

func TestHandleQuery(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectQuery("SELECT 1 AS one").
		WillReturnRows(pgxmock.NewRows([]string{"one"}).AddRow(1))

	out, err := handleQuery(map[string]interface{}{"sql": "SELECT 1 AS one"})
	if err != nil {
		t.Fatalf("handleQuery: %v", err)
	}
	res := out.(map[string]interface{})
	if res["count"] != 1 {
		t.Fatalf("count = %v, want 1", res["count"])
	}
	if rows := res["rows"].([]map[string]interface{}); rows[0]["one"] != 1 {
		t.Fatalf("row = %v", rows)
	}
}

func TestHandleQueryMissingSQL(t *testing.T) {
	newMockPool(t)
	if _, err := handleQuery(map[string]interface{}{}); err == nil {
		t.Fatal("缺 sql 应报错")
	}
}

func TestHandleQueryOne(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectQuery("_pg_mcp_q").
		WillReturnRows(pgxmock.NewRows([]string{"one"}).AddRow(1))

	out, err := handleQueryOne(map[string]interface{}{"sql": "SELECT 1 AS one;"})
	if err != nil {
		t.Fatalf("handleQueryOne: %v", err)
	}
	res := out.(map[string]interface{})
	if res["found"] != true || res["row"] == nil {
		t.Fatalf("handleQueryOne result = %v", res)
	}
}

func TestHandleQueryOneEmpty(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectQuery("_pg_mcp_q").WillReturnRows(pgxmock.NewRows([]string{"one"}))

	out, err := handleQueryOne(map[string]interface{}{"sql": "SELECT 1"})
	if err != nil {
		t.Fatalf("handleQueryOne: %v", err)
	}
	res := out.(map[string]interface{})
	if res["found"] != false {
		t.Fatalf("found = %v, want false", res["found"])
	}
}

func TestHandleQueryPaginated(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectQuery("SELECT 1 AS one").
		WillReturnRows(pgxmock.NewRows([]string{"one"}).AddRow(1).AddRow(1))

	out, err := handleQueryPaginated(map[string]interface{}{"sql": "SELECT 1 AS one", "page": 2, "page_size": 10})
	if err != nil {
		t.Fatalf("handleQueryPaginated: %v", err)
	}
	res := out.(map[string]interface{})
	if res["page"] != 2 || res["page_size"] != 10 || res["offset"] != 10 {
		t.Fatalf("pagination = %v", res)
	}
}

func TestHandleCount(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectQuery(`COUNT\(\*\)`).
		WillReturnRows(pgxmock.NewRows([]string{"cnt"}).AddRow(42))

	out, err := handleCount(map[string]interface{}{"table": "users", "where": "active = true"})
	if err != nil {
		t.Fatalf("handleCount: %v", err)
	}
	res := out.(map[string]interface{})
	if res["count"] != 42 {
		t.Fatalf("count = %v, want 42", res["count"])
	}
}

func TestHandleBatchQuery(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectQuery("SELECT 1").WillReturnRows(pgxmock.NewRows([]string{"one"}).AddRow(1))
	mock.ExpectQuery("SELECT 2").WillReturnRows(pgxmock.NewRows([]string{"two"}).AddRow(2))

	out, err := handleBatchQuery(map[string]interface{}{"queries": []interface{}{"SELECT 1", "SELECT 2"}})
	if err != nil {
		t.Fatalf("handleBatchQuery: %v", err)
	}
	res := out.(map[string]interface{})
	if res["ok_count"] != 2 || res["fail_count"] != 0 {
		t.Fatalf("batch result = %v", res)
	}
}

// 针对 query_paginated 钳制边界变异：page/page_size 越界时钳制，断言 LIMIT/OFFSET。
func TestHandleQueryPaginatedClamp(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectQuery(`LIMIT 20 OFFSET 0`).WillReturnRows(pgxmock.NewRows([]string{"one"}))
	out, err := handleQueryPaginated(map[string]interface{}{"sql": "SELECT 1", "page": 0, "page_size": 5000})
	if err != nil {
		t.Fatalf("handleQueryPaginated clamp: %v", err)
	}
	res := out.(map[string]interface{})
	if res["page"] != 1 || res["page_size"] != 20 || res["offset"] != 0 {
		t.Fatalf("clamp = %v", res)
	}
}

// 针对 handleCount 的 where 守卫与空结果边界变异。
func TestHandleCountWithWhere(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectQuery(`WHERE active`).WillReturnRows(pgxmock.NewRows([]string{"cnt"}).AddRow(3))
	out, err := handleCount(map[string]interface{}{"table": "users", "where": "active = true"})
	if err != nil {
		t.Fatalf("handleCount: %v", err)
	}
	if res := out.(map[string]interface{}); res["count"] != 3 {
		t.Fatalf("count = %v", res["count"])
	}
}

func TestHandleCountEmptyResults(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectQuery(`COUNT`).WillReturnRows(pgxmock.NewRows([]string{"cnt"}))
	out, err := handleCount(map[string]interface{}{"table": "users"})
	if err != nil {
		t.Fatalf("handleCount empty: %v", err)
	}
	if res := out.(map[string]interface{}); res["count"] != nil {
		t.Fatalf("空结果 count 应为 nil, got %v", res["count"])
	}
}

func TestKillQueryPaginatedPageSizeBoundary(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectQuery(`LIMIT 1000 OFFSET 0`).WillReturnRows(pgxmock.NewRows([]string{"one"}))
	out, err := handleQueryPaginated(map[string]interface{}{"sql": "SELECT 1", "page": 1, "page_size": 1000})
	if err != nil {
		t.Fatalf("page_size=1000 不应钳制: %v", err)
	}
	if m := assertMap(t, out); m["page_size"] != 1000 {
		t.Fatalf("page_size=%v", m["page_size"])
	}
}

// ---- 占位符算术变异：断言 $1/$2/$3 文本 ----

func TestKillBatchQuerySuccessFlag(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectQuery("SELECT 1").WillReturnRows(pgxmock.NewRows([]string{"one"}).AddRow(1))
	out, err := handleBatchQuery(map[string]interface{}{"queries": []interface{}{"SELECT 1"}})
	if err != nil {
		t.Fatal(err)
	}
	if m := assertMap(t, out); m["success"] != true {
		t.Fatalf("result=%v", m)
	}
}
