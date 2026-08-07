package tools

import (
	"testing"

	"github.com/pashagolub/pgxmock/v5"
)

func TestHandleInsert(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectExec(`INSERT INTO "users"`).
		WithArgs(30, "john").
		WillReturnResult(pgxmock.NewResult("INSERT", 1))

	out, err := handleInsert(map[string]interface{}{
		"table": "users",
		"data":  map[string]interface{}{"name": "john", "age": 30},
	})
	if err != nil {
		t.Fatalf("handleInsert: %v", err)
	}
	res := out.(map[string]interface{})
	if res["affected_rows"] != int64(1) {
		t.Fatalf("affected_rows = %v", res["affected_rows"])
	}
}

func TestHandleInsertRequiresData(t *testing.T) {
	newMockPool(t)
	if _, err := handleInsert(map[string]interface{}{"table": "users"}); err == nil {
		t.Fatal("缺 data 应报错")
	}
}

func TestHandleInsertBatch(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectExec(`INSERT INTO "users"`).
		WithArgs(30, "john", 25, "jane").
		WillReturnResult(pgxmock.NewResult("INSERT", 2))

	out, err := handleInsertBatch(map[string]interface{}{
		"table": "users",
		"rows": []interface{}{
			map[string]interface{}{"name": "john", "age": 30},
			map[string]interface{}{"name": "jane", "age": 25},
		},
	})
	if err != nil {
		t.Fatalf("handleInsertBatch: %v", err)
	}
	res := out.(map[string]interface{})
	if res["inserted_rows"] != int64(2) || res["submitted"] != 2 {
		t.Fatalf("result = %v", res)
	}
}

func TestHandleUpdate(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectExec(`UPDATE "users"`).
		WithArgs("newname").
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	out, err := handleUpdate(map[string]interface{}{
		"table": "users",
		"data":  map[string]interface{}{"name": "newname"},
		"where": "id = 1",
	})
	if err != nil {
		t.Fatalf("handleUpdate: %v", err)
	}
	if res := out.(map[string]interface{}); res["affected_rows"] != int64(1) {
		t.Fatalf("affected_rows = %v", res["affected_rows"])
	}
}

func TestHandleUpdateRequiresWhere(t *testing.T) {
	newMockPool(t)
	if _, err := handleUpdate(map[string]interface{}{
		"table": "users", "data": map[string]interface{}{"name": "x"},
	}); err == nil {
		t.Fatal("缺 where 应报错（防全表误更新）")
	}
}

func TestHandleDelete(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectExec(`DELETE FROM "users"`).
		WillReturnResult(pgxmock.NewResult("DELETE", 3))

	out, err := handleDelete(map[string]interface{}{"table": "users", "where": "id < 4"})
	if err != nil {
		t.Fatalf("handleDelete: %v", err)
	}
	if res := out.(map[string]interface{}); res["affected_rows"] != int64(3) {
		t.Fatalf("affected_rows = %v", res["affected_rows"])
	}
}

func TestHandleDeleteRequiresWhere(t *testing.T) {
	newMockPool(t)
	if _, err := handleDelete(map[string]interface{}{"table": "users"}); err == nil {
		t.Fatal("缺 where 应报错（防全表误删）")
	}
}

func TestHandleMerge(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectExec(`ON CONFLICT`).
		WithArgs(30, "john").
		WillReturnResult(pgxmock.NewResult("INSERT", 1))

	out, err := handleMerge(map[string]interface{}{
		"table":         "users",
		"data":          map[string]interface{}{"name": "john", "age": 30},
		"match_columns": []interface{}{"name"},
	})
	if err != nil {
		t.Fatalf("handleMerge: %v", err)
	}
	if res := out.(map[string]interface{}); res["affected_rows"] != int64(1) {
		t.Fatalf("affected_rows = %v", res["affected_rows"])
	}
}

func TestHandleMergeAllConflictColumns(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectExec(`DO NOTHING`).
		WithArgs(1).
		WillReturnResult(pgxmock.NewResult("INSERT", 0))

	out, err := handleMerge(map[string]interface{}{
		"table":         "users",
		"data":          map[string]interface{}{"id": 1},
		"match_columns": []interface{}{"id"},
	})
	if err != nil {
		t.Fatalf("handleMerge(DO NOTHING): %v", err)
	}
	if res := out.(map[string]interface{}); res["affected_rows"] != int64(0) {
		t.Fatalf("affected_rows = %v", res["affected_rows"])
	}
}

func TestHandleBatchUpdate(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "users"`).WithArgs("a").WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectExec(`UPDATE "users"`).WithArgs("b").WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectCommit()

	out, err := handleBatchUpdate(map[string]interface{}{
		"table": "users",
		"updates": []interface{}{
			map[string]interface{}{"data": map[string]interface{}{"name": "a"}, "where": "id = 1"},
			map[string]interface{}{"data": map[string]interface{}{"name": "b"}, "where": "id = 2"},
		},
	})
	if err != nil {
		t.Fatalf("handleBatchUpdate: %v", err)
	}
	if res := out.(map[string]interface{}); res["total"] != 2 {
		t.Fatalf("total = %v", res["total"])
	}
}

func TestHandleBatchDelete(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectBegin()
	mock.ExpectExec(`DELETE FROM "users"`).WillReturnResult(pgxmock.NewResult("DELETE", 1))
	mock.ExpectCommit()

	out, err := handleBatchDelete(map[string]interface{}{
		"table":  "users",
		"wheres": []interface{}{"id = 1"},
	})
	if err != nil {
		t.Fatalf("handleBatchDelete: %v", err)
	}
	if res := out.(map[string]interface{}); res["total"] != 1 {
		t.Fatalf("total = %v", res["total"])
	}
}

func TestKillInsertPlaceholders(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectExec(`VALUES \(\$1, \$2\)`).WithArgs(1, "a").
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	if _, err := handleInsert(map[string]interface{}{
		"table": "t", "data": map[string]interface{}{"a": 1, "b": "a"},
	}); err != nil {
		t.Fatal(err)
	}
}

func TestKillInsertBatchPlaceholders(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectExec(`\$3, \$4`).WithArgs(1, "x", 2, "y").
		WillReturnResult(pgxmock.NewResult("INSERT", 2))
	if _, err := handleInsertBatch(map[string]interface{}{
		"table": "t",
		"rows": []interface{}{
			map[string]interface{}{"a": 1, "b": "x"},
			map[string]interface{}{"a": 2, "b": "y"},
		},
	}); err != nil {
		t.Fatal(err)
	}
}

func TestKillUpdatePlaceholder(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectExec(`SET "a" = \$1`).WithArgs("v").
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	if _, err := handleUpdate(map[string]interface{}{
		"table": "t", "data": map[string]interface{}{"a": "v"}, "where": "id=1",
	}); err != nil {
		t.Fatal(err)
	}
}

func TestKillMergePlaceholders(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectExec(`VALUES \(\$1, \$2\)`).WithArgs(1, 2).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	if _, err := handleMerge(map[string]interface{}{
		"table": "t", "data": map[string]interface{}{"k": 1, "v": 2}, "match_columns": []interface{}{"k"},
	}); err != nil {
		t.Fatal(err)
	}
}

func TestKillBatchUpdatePlaceholder(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectBegin()
	mock.ExpectExec(`SET "x" = \$1`).WithArgs(5).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectCommit()
	if _, err := handleBatchUpdate(map[string]interface{}{
		"table":   "t",
		"updates": []interface{}{map[string]interface{}{"data": map[string]interface{}{"x": 5}, "where": "id=1"}},
	}); err != nil {
		t.Fatal(err)
	}
}
