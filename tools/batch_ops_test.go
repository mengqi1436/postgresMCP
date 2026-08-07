package tools

import (
	"testing"

	"github.com/pashagolub/pgxmock/v5"
)

func TestHandleBatchExecuteSQLNonAtomic(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectQuery("SELECT 1").WillReturnRows(pgxmock.NewRows([]string{"one"}).AddRow(1))
	mock.ExpectExec(`UPDATE "t"`).WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	out, err := handleBatchExecuteSQL(map[string]interface{}{
		"statements": []interface{}{"SELECT 1", `UPDATE "t" SET x = 1`},
	})
	if err != nil {
		t.Fatalf("handleBatchExecuteSQL: %v", err)
	}
	res := out.(map[string]interface{})
	if res["ok_count"] != 2 || res["fail_count"] != 0 || res["atomic"] != false {
		t.Fatalf("result = %v", res)
	}
}

func TestHandleBatchExecuteSQLAtomic(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO "a"`).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectCommit()

	out, err := handleBatchExecuteSQL(map[string]interface{}{
		"statements": []interface{}{`INSERT INTO "a" (x) VALUES (1)`}, "atomic": true,
	})
	if err != nil {
		t.Fatalf("handleBatchExecuteSQL(atomic): %v", err)
	}
	res := out.(map[string]interface{})
	if res["atomic"] != true || res["ok_count"] != 1 {
		t.Fatalf("result = %v", res)
	}
}

func TestHandleBatchExecuteSQLMissingStatements(t *testing.T) {
	newMockPool(t)
	if _, err := handleBatchExecuteSQL(map[string]interface{}{}); err == nil {
		t.Fatal("缺 statements 应报错")
	}
}

func TestKillBatchExecuteSQLSuccessFlag(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectQuery("SELECT 1").WillReturnRows(pgxmock.NewRows([]string{"one"}).AddRow(1))
	out, err := handleBatchExecuteSQL(map[string]interface{}{"statements": []interface{}{"SELECT 1"}})
	if err != nil {
		t.Fatal(err)
	}
	if m := assertMap(t, out); m["success"] != true {
		t.Fatalf("result=%v", m)
	}
}
