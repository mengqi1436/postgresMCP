package tools

import (
	"errors"
	"testing"

	"github.com/pashagolub/pgxmock/v5"
)

func TestHandleExecuteSQLQuery(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectQuery("SELECT 1 AS one").
		WillReturnRows(pgxmock.NewRows([]string{"one"}).AddRow(1))

	out, err := handleExecuteSQL(map[string]interface{}{"sql": "SELECT 1 AS one"})
	if err != nil {
		t.Fatalf("handleExecuteSQL(query): %v", err)
	}
	res := out.(map[string]interface{})
	if res["type"] != "query" || res["count"] != 1 {
		t.Fatalf("result = %v", res)
	}
}

func TestHandleExecuteSQLDML(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectExec(`UPDATE t`).WithArgs(5).WillReturnResult(pgxmock.NewResult("UPDATE", 2))

	out, err := handleExecuteSQL(map[string]interface{}{
		"sql": "UPDATE t SET n = $1", "params": []interface{}{5},
	})
	if err != nil {
		t.Fatalf("handleExecuteSQL(dml): %v", err)
	}
	res := out.(map[string]interface{})
	if res["type"] != "dml" || res["affected_rows"] != int64(2) {
		t.Fatalf("result = %v", res)
	}
}

func TestHandleExecuteSQLDDL(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectExec("CREATE TABLE").WillReturnResult(pgxmock.NewResult("CREATE TABLE", 0))

	out, err := handleExecuteSQL(map[string]interface{}{"sql": "CREATE TABLE t (id int)"})
	if err != nil {
		t.Fatalf("handleExecuteSQL(ddl): %v", err)
	}
	if res := out.(map[string]interface{}); res["type"] != "ddl" {
		t.Fatalf("result = %v", res)
	}
}

func TestHandleExecuteTransaction(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "a"`).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectExec(`INSERT INTO "b"`).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectCommit()

	out, err := handleExecuteTransaction(map[string]interface{}{
		"statements": []interface{}{`UPDATE "a" SET x=1`, `INSERT INTO "b" (x) VALUES (1)`},
	})
	if err != nil {
		t.Fatalf("handleExecuteTransaction: %v", err)
	}
	if res := out.(map[string]interface{}); res["total"] != 2 {
		t.Fatalf("result = %v", res)
	}
}

func TestHandleExecuteTransactionRollbackOnError(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "a"`).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectExec(`BAD SQL`).WillReturnError(errors.New("boom")) // 第二条失败 → 回滚
	mock.ExpectRollback()

	_, err := handleExecuteTransaction(map[string]interface{}{
		"statements": []interface{}{`UPDATE "a" SET x=1`, `BAD SQL`},
	})
	if err == nil {
		t.Fatal("第二条失败应返回错误")
	}
}

func TestHandleCallFunction(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectQuery("my_func").
		WithArgs(1).
		WillReturnRows(pgxmock.NewRows([]string{"result"}).AddRow(42))

	out, err := handleCallFunction(map[string]interface{}{
		"function_name": "my_func", "params": []interface{}{1},
	})
	if err != nil {
		t.Fatalf("handleCallFunction: %v", err)
	}
	res := out.(map[string]interface{})
	if res["result"] != 42 {
		t.Fatalf("result = %v", res["result"])
	}
}

func TestHandleCallProcedure(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectQuery("CALL").
		WillReturnRows(pgxmock.NewRows([]string{"x"}).AddRow(1))

	out, err := handleCallProcedure(map[string]interface{}{"procedure_name": "proc"})
	if err != nil {
		t.Fatalf("handleCallProcedure: %v", err)
	}
	if res := out.(map[string]interface{}); res["count"] != 1 {
		t.Fatalf("result = %v", res)
	}
}

func TestHandleExplainPlanPlain(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectQuery("EXPLAIN").
		WillReturnRows(pgxmock.NewRows([]string{"QUERY PLAN"}).AddRow(`Seq Scan on users`))

	out, err := handleExplainPlan(map[string]interface{}{"sql": "SELECT * FROM users"})
	if err != nil {
		t.Fatalf("handleExplainPlan: %v", err)
	}
	res := out.(map[string]interface{})
	if res["plan"] != "Seq Scan on users" || res["analyze"] != false {
		t.Fatalf("result = %v", res)
	}
}

func TestHandleExplainPlanAnalyzeWriteRollsBack(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectBegin()
	mock.ExpectQuery("EXPLAIN \\(ANALYZE, FORMAT JSON\\) UPDATE").
		WillReturnRows(pgxmock.NewRows([]string{"QUERY PLAN"}).AddRow(`{"Plan": {}}`))
	mock.ExpectRollback()

	out, err := handleExplainPlan(map[string]interface{}{
		"sql": "UPDATE users SET name = 'x'", "analyze": true,
	})
	if err != nil {
		t.Fatalf("handleExplainPlan(analyze): %v", err)
	}
	if res := out.(map[string]interface{}); res["analyze"] != true {
		t.Fatalf("result = %v", res)
	}
}

func TestKillCallFunctionEmptyResults(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectQuery(`"f"`).WillReturnRows(pgxmock.NewRows([]string{"result"}))
	out, err := handleCallFunction(map[string]interface{}{"function_name": "f"})
	if err != nil {
		t.Fatal(err)
	}
	if m := assertMap(t, out); m["result"] != nil {
		t.Fatalf("result=%v", m["result"])
	}
}

func TestKillExplainPlanEmptyResults(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectQuery("EXPLAIN").WillReturnRows(pgxmock.NewRows([]string{"QUERY PLAN"}))
	out, err := handleExplainPlan(map[string]interface{}{"sql": "SELECT 1"})
	if err != nil {
		t.Fatal(err)
	}
	if m := assertMap(t, out); m["plan"] != nil {
		t.Fatalf("plan=%v", m["plan"])
	}
}

// ---- limit/page 边界精确值（杀 < → <= 与 > → >= 的非等价侧）----

func TestKillCallFunctionPlaceholder(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectQuery(`"f"\(\$1\)`).WithArgs(7).
		WillReturnRows(pgxmock.NewRows([]string{"result"}).AddRow(42))
	out, err := handleCallFunction(map[string]interface{}{"function_name": "f", "params": []interface{}{7}})
	if err != nil {
		t.Fatal(err)
	}
	if m := assertMap(t, out); m["result"] != 42 {
		t.Fatalf("result=%v", m["result"])
	}
}

// ---- len(args)==0 变异：execute_sql DDL 带参走 Execute ----

func TestKillExecuteSQLDDLWithArgs(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectExec("CREATE INDEX").WithArgs("x").
		WillReturnResult(pgxmock.NewResult("CREATE INDEX", 1))
	out, err := handleExecuteSQL(map[string]interface{}{
		"sql": "CREATE INDEX i ON t(c) WHERE c = $1", "params": []interface{}{"x"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if m := assertMap(t, out); m["type"] != "ddl" {
		t.Fatalf("result=%v", m)
	}
}

// ---- failCount==0 变异：断言 success 字段 ----
