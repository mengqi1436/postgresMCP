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
// 分页 SQL 多取一行判断 has_more，故 LIMIT 为 page_size+1。
func TestHandleQueryPaginatedClamp(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectQuery(`LIMIT 21 OFFSET 0`).WillReturnRows(pgxmock.NewRows([]string{"one"}))
	out, err := handleQueryPaginated(map[string]interface{}{"sql": "SELECT 1", "page": 0, "page_size": 5000})
	if err != nil {
		t.Fatalf("handleQueryPaginated clamp: %v", err)
	}
	res := out.(map[string]interface{})
	if res["page"] != 1 || res["page_size"] != 20 || res["offset"] != 0 {
		t.Fatalf("clamp = %v", res)
	}
	if res["has_more"] != false {
		t.Fatalf("空结果 has_more 应为 false, got %v", res["has_more"])
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
	mock.ExpectQuery(`LIMIT 1001 OFFSET 0`).WillReturnRows(pgxmock.NewRows([]string{"one"}))
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

// ---- token 优化：limit 行数上限与 has_more ----

func TestHandleQueryLimitParam(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectQuery("SELECT 1").
		WillReturnRows(pgxmock.NewRows([]string{"n"}).AddRow(1).AddRow(2).AddRow(3))

	out, err := handleQuery(map[string]interface{}{"sql": "SELECT 1", "limit": 2})
	if err != nil {
		t.Fatalf("handleQuery limit: %v", err)
	}
	res := out.(map[string]interface{})
	if res["count"] != 2 || res["total"] != 3 || res["truncated"] != true {
		t.Fatalf("limit 截断结果 = %v", res)
	}
}

func TestHandleQueryDefaultLimit(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectQuery("SELECT 1").
		WillReturnRows(pgxmock.NewRows([]string{"n"}).AddRow(1).AddRow(2))

	out, err := handleQuery(map[string]interface{}{"sql": "SELECT 1"})
	if err != nil {
		t.Fatalf("handleQuery: %v", err)
	}
	res := out.(map[string]interface{})
	if res["count"] != 2 || res["total"] != 2 || res["truncated"] != false {
		t.Fatalf("默认 limit 结果 = %v", res)
	}
}

func TestHandleQueryPaginatedHasMore(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectQuery("SELECT 1").
		WillReturnRows(pgxmock.NewRows([]string{"n"}).AddRow(1).AddRow(2).AddRow(3))

	out, err := handleQueryPaginated(map[string]interface{}{"sql": "SELECT 1", "page": 1, "page_size": 2})
	if err != nil {
		t.Fatalf("handleQueryPaginated: %v", err)
	}
	res := out.(map[string]interface{})
	if res["has_more"] != true || res["count"] != 2 {
		t.Fatalf("has_more 结果 = %v", res)
	}
}

func TestHandleQueryPaginatedNoMore(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectQuery("SELECT 1").
		WillReturnRows(pgxmock.NewRows([]string{"n"}).AddRow(1).AddRow(2))

	out, err := handleQueryPaginated(map[string]interface{}{"sql": "SELECT 1", "page": 1, "page_size": 5})
	if err != nil {
		t.Fatalf("handleQueryPaginated: %v", err)
	}
	res := out.(map[string]interface{})
	if res["has_more"] != false || res["count"] != 2 {
		t.Fatalf("末页 has_more 应为 false, got %v", res)
	}
}

// ---- detail_level 分级返回（summary 只返回概览+示例，省 token）----

func TestHandleQuerySummary(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectQuery("SELECT 1").
		WillReturnRows(pgxmock.NewRows([]string{"n"}).AddRow(1).AddRow(2).AddRow(3).AddRow(4).AddRow(5))

	out, err := handleQuery(map[string]interface{}{"sql": "SELECT 1", "detail_level": "summary"})
	if err != nil {
		t.Fatalf("handleQuery summary: %v", err)
	}
	res := out.(map[string]interface{})
	if _, hasRows := res["rows"]; hasRows {
		t.Fatalf("summary 不应返回 rows: %v", res)
	}
	if res["count"] != 3 || res["total"] != 5 || res["truncated"] != true {
		t.Fatalf("summary 概览 = %v", res)
	}
	sample, ok := res["sample"].([]map[string]interface{})
	if !ok || len(sample) != 3 {
		t.Fatalf("sample 应为 3 行示例, got %v", res["sample"])
	}
}

func TestHandleQueryFullDefaultMaxRows(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectQuery("SELECT 1").
		WillReturnRows(pgxmock.NewRows([]string{"n"}).AddRow(1).AddRow(2).AddRow(3))

	out, err := handleQuery(map[string]interface{}{"sql": "SELECT 1", "detail_level": "full"})
	if err != nil {
		t.Fatalf("handleQuery full: %v", err)
	}
	res := out.(map[string]interface{})
	if res["count"] != 3 || res["total"] != 3 || res["truncated"] != false {
		t.Fatalf("full 档小结果 = %v", res)
	}
	if _, hasRows := res["rows"]; !hasRows {
		t.Fatalf("full 应返回 rows")
	}
}

func TestHandleQueryDetailLevelInvalidFallback(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectQuery("SELECT 1").
		WillReturnRows(pgxmock.NewRows([]string{"n"}).AddRow(1).AddRow(2).AddRow(3))

	out, err := handleQuery(map[string]interface{}{"sql": "SELECT 1", "detail_level": "verbose"})
	if err != nil {
		t.Fatalf("handleQuery: %v", err)
	}
	res := out.(map[string]interface{})
	if _, hasRows := res["rows"]; !hasRows {
		t.Fatalf("非法 detail_level 应回退 detail 并返回 rows")
	}
	if res["count"] != 3 || res["total"] != 3 || res["truncated"] != false {
		t.Fatalf("回退结果 = %v", res)
	}
}

// ---- 条件边界精确值：杀 > vs >= / < vs <= 变异 ----

// summary 档 + 空结果：不应有 sample 键（针对 len(rows)>0 的 >= 变异）。
func TestHandleQuerySummaryEmpty(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectQuery("SELECT 1").WillReturnRows(pgxmock.NewRows([]string{"n"}))

	out, err := handleQuery(map[string]interface{}{"sql": "SELECT 1", "detail_level": "summary"})
	if err != nil {
		t.Fatalf("handleQuery summary empty: %v", err)
	}
	res := out.(map[string]interface{})
	if _, hasSample := res["sample"]; hasSample {
		t.Fatalf("空结果 summary 不应有 sample: %v", res)
	}
	if res["count"] != 0 || res["total"] != 0 || res["truncated"] != false {
		t.Fatalf("空结果概览 = %v", res)
	}
}

// page_size=1（下边界）不应被钳制（针对 pageSize<1 的 <= 变异）。
func TestHandleQueryPaginatedPageSizeOne(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectQuery(`LIMIT 2 OFFSET 0`).WillReturnRows(pgxmock.NewRows([]string{"n"}).AddRow(1).AddRow(2))

	out, err := handleQueryPaginated(map[string]interface{}{"sql": "SELECT 1", "page": 1, "page_size": 1})
	if err != nil {
		t.Fatalf("handleQueryPaginated page_size=1: %v", err)
	}
	res := out.(map[string]interface{})
	if res["page_size"] != 1 {
		t.Fatalf("page_size=1 不应钳制, got %v", res["page_size"])
	}
	if res["has_more"] != true || res["count"] != 1 {
		t.Fatalf("结果 = %v", res)
	}
}

// 恰好满页（len == page_size）：has_more 应为 false 且不截断（针对 > pageSize 的 >= 变异）。
func TestHandleQueryPaginatedExactFullPage(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectQuery("SELECT 1").WillReturnRows(pgxmock.NewRows([]string{"n"}).AddRow(1).AddRow(2))

	out, err := handleQueryPaginated(map[string]interface{}{"sql": "SELECT 1", "page": 1, "page_size": 2})
	if err != nil {
		t.Fatalf("handleQueryPaginated exact full: %v", err)
	}
	res := out.(map[string]interface{})
	if res["has_more"] != false || res["count"] != 2 {
		t.Fatalf("恰好满页应 has_more=false 且返回 2 行, got %v", res)
	}
}

// limit=1（下边界）→ maxRows=1 截断（针对 clampLimit 的 <= 变异经集成路径）。
func TestHandleQueryLimitOne(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectQuery("SELECT 1").WillReturnRows(pgxmock.NewRows([]string{"n"}).AddRow(1).AddRow(2))

	out, err := handleQuery(map[string]interface{}{"sql": "SELECT 1", "limit": 1})
	if err != nil {
		t.Fatalf("handleQuery limit=1: %v", err)
	}
	res := out.(map[string]interface{})
	if res["count"] != 1 || res["total"] != 2 || res["truncated"] != true {
		t.Fatalf("limit=1 截断结果 = %v", res)
	}
}
