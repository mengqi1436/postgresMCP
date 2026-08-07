package tools

import (
	"testing"

	"github.com/pashagolub/pgxmock/v5"
)

func TestHandleListDatabases(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectQuery("pg_database").
		WillReturnRows(pgxmock.NewRows([]string{"database_name", "size"}).AddRow("postgres", "7 MB"))

	out, err := handleListDatabases(map[string]interface{}{})
	if err != nil {
		t.Fatalf("handleListDatabases: %v", err)
	}
	if res := out.(map[string]interface{}); res["count"] != 1 {
		t.Fatalf("count = %v", res["count"])
	}
}

func TestHandleListSchemas(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectQuery("pg_namespace").
		WillReturnRows(pgxmock.NewRows([]string{"schema_name"}).AddRow("public"))

	out, err := handleListSchemas(map[string]interface{}{"include_system": true})
	if err != nil {
		t.Fatalf("handleListSchemas: %v", err)
	}
	if res := out.(map[string]interface{}); res["count"] != 1 {
		t.Fatalf("count = %v", res["count"])
	}
}

func TestHandleListTables(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectQuery("pg_class").
		WithArgs("public").
		WillReturnRows(pgxmock.NewRows([]string{"table_name"}).AddRow("users"))

	out, err := handleListTables(map[string]interface{}{"schema": "public"})
	if err != nil {
		t.Fatalf("handleListTables: %v", err)
	}
	if res := out.(map[string]interface{}); res["count"] != 1 {
		t.Fatalf("count = %v", res["count"])
	}
}

func TestHandleListViews(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectQuery("pg_class").
		WillReturnRows(pgxmock.NewRows([]string{"view_name"}).AddRow("v"))

	if _, err := handleListViews(map[string]interface{}{}); err != nil {
		t.Fatalf("handleListViews: %v", err)
	}
}

func TestHandleDescribeTable(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectQuery("pg_class").
		WithArgs("users").
		WillReturnRows(pgxmock.NewRows([]string{"schema_name", "table_name"}).AddRow("public", "users"))
	mock.ExpectQuery("pg_attribute").
		WithArgs("users").
		WillReturnRows(pgxmock.NewRows([]string{"column_name"}).AddRow("id"))

	out, err := handleDescribeTable(map[string]interface{}{"table_name": "users"})
	if err != nil {
		t.Fatalf("handleDescribeTable: %v", err)
	}
	res := out.(map[string]interface{})
	if res["table"] == nil || res["columns"] == nil {
		t.Fatalf("result = %v", res)
	}
}

func TestHandleDescribeTableNotFound(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectQuery("pg_class").
		WithArgs("nope").
		WillReturnRows(pgxmock.NewRows([]string{"table_name"}))

	if _, err := handleDescribeTable(map[string]interface{}{"table_name": "nope"}); err == nil {
		t.Fatal("表不存在应报错")
	}
}

func TestHandleListIndexes(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectQuery("pg_indexes").
		WithArgs("users", "public").
		WillReturnRows(pgxmock.NewRows([]string{"index_name"}).AddRow("idx_users_name"))

	out, err := handleListIndexes(map[string]interface{}{"table_name": "users", "schema": "public"})
	if err != nil {
		t.Fatalf("handleListIndexes: %v", err)
	}
	if res := out.(map[string]interface{}); res["count"] != 1 {
		t.Fatalf("count = %v", res["count"])
	}
}

func TestHandleListSequences(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectQuery("pg_sequences").
		WillReturnRows(pgxmock.NewRows([]string{"sequence_name"}).AddRow("seq"))

	if _, err := handleListSequences(map[string]interface{}{}); err != nil {
		t.Fatalf("handleListSequences: %v", err)
	}
}

func TestHandleListConstraints(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectQuery("pg_constraint").
		WithArgs("users").
		WillReturnRows(pgxmock.NewRows([]string{"constraint_name"}).AddRow("users_pkey"))

	out, err := handleListConstraints(map[string]interface{}{"table_name": "users"})
	if err != nil {
		t.Fatalf("handleListConstraints: %v", err)
	}
	if res := out.(map[string]interface{}); res["count"] != 1 {
		t.Fatalf("count = %v", res["count"])
	}
}

func TestHandleListFunctions(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectQuery("pg_proc").
		WillReturnRows(pgxmock.NewRows([]string{"name"}).AddRow("my_func"))

	out, err := handleListFunctions(map[string]interface{}{})
	if err != nil {
		t.Fatalf("handleListFunctions: %v", err)
	}
	if res := out.(map[string]interface{}); res["count"] != 1 {
		t.Fatalf("count = %v", res["count"])
	}
}

func TestHandleDescribeIndex(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectQuery("pg_index").
		WithArgs("idx").
		WillReturnRows(pgxmock.NewRows([]string{"index_name"}).AddRow("idx"))
	mock.ExpectQuery("pg_attribute").
		WithArgs("idx").
		WillReturnRows(pgxmock.NewRows([]string{"column_name"}).AddRow("name"))

	out, err := handleDescribeIndex(map[string]interface{}{"index_name": "idx"})
	if err != nil {
		t.Fatalf("handleDescribeIndex: %v", err)
	}
	if out.(map[string]interface{})["index_name"] != "idx" {
		t.Fatalf("result = %v", out)
	}
}

func TestHandleDescribeIndexMissingName(t *testing.T) {
	newMockPool(t)
	if _, err := handleDescribeIndex(map[string]interface{}{}); err == nil {
		t.Fatal("缺 index_name 应报错")
	}
}

func TestKillListViewsResult(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectQuery("pg_class").WillReturnRows(pgxmock.NewRows([]string{"view_name"}).AddRow("v"))
	out, err := handleListViews(map[string]interface{}{})
	if err != nil {
		t.Fatal(err)
	}
	if m := assertMap(t, out); m["count"] != 1 {
		t.Fatalf("result=%v", m)
	}
}

func TestKillListSequencesResult(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectQuery("pg_sequences").WillReturnRows(pgxmock.NewRows([]string{"sequence_name"}).AddRow("s"))
	out, err := handleListSequences(map[string]interface{}{})
	if err != nil {
		t.Fatal(err)
	}
	if m := assertMap(t, out); m["count"] != 1 {
		t.Fatalf("result=%v", m)
	}
}

// describe_index 的 columns 守卫取反（err==nil → err!=nil，不再设置 columns）
func TestKillDescribeIndexColumns(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectQuery("pg_index").WithArgs("idx").
		WillReturnRows(pgxmock.NewRows([]string{"index_name"}).AddRow("idx"))
	mock.ExpectQuery("pg_attribute").WithArgs("idx").
		WillReturnRows(pgxmock.NewRows([]string{"column_name"}).AddRow("c"))
	out, err := handleDescribeIndex(map[string]interface{}{"index_name": "idx"})
	if err != nil {
		t.Fatal(err)
	}
	m := assertMap(t, out)
	cols, ok := m["columns"].([]map[string]interface{})
	if !ok || len(cols) != 1 {
		t.Fatalf("columns=%#v", m["columns"])
	}
}

// tablespace 的 databases 守卫取反（typed-nil 存 interface，用 len 判断）
