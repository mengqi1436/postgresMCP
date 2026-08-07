package tools

import (
	"strings"
	"testing"

	"github.com/pashagolub/pgxmock/v5"
)

func TestHandleCreateTable(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectExec(`CREATE TABLE "users"`).
		WillReturnResult(pgxmock.NewResult("CREATE TABLE", 0))

	out, err := handleCreateTable(map[string]interface{}{
		"table_name": "users",
		"columns": []interface{}{
			map[string]interface{}{"name": "id", "type": "bigint", "primary_key": true},
			map[string]interface{}{"name": "name", "type": "varchar", "length": 100, "not_null": true},
		},
	})
	if err != nil {
		t.Fatalf("handleCreateTable: %v", err)
	}
	sql := out.(map[string]interface{})["sql"].(string)
	if !strings.Contains(sql, `"id" bigint PRIMARY KEY`) || !strings.Contains(sql, `"name" varchar(100) NOT NULL`) {
		t.Fatalf("SQL 生成异常: %s", sql)
	}
}

func TestHandleCreateTableDuplicatePK(t *testing.T) {
	newMockPool(t)
	_, err := handleCreateTable(map[string]interface{}{
		"table_name": "users",
		"columns": []interface{}{
			map[string]interface{}{"name": "a", "type": "int", "primary_key": true},
			map[string]interface{}{"name": "b", "type": "int", "primary_key": true},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "多个 primary_key") {
		t.Fatalf("期望多主键报错, got %v", err)
	}
}

func TestHandleCreateTableDefaultExpr(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectExec(`CREATE TABLE`).WillReturnResult(pgxmock.NewResult("CREATE TABLE", 0))

	out, err := handleCreateTable(map[string]interface{}{
		"table_name": "logs",
		"columns": []interface{}{
			map[string]interface{}{"name": "id", "type": "bigint"},
			map[string]interface{}{"name": "created_at", "type": "timestamp", "default": "CURRENT_TIMESTAMP"},
			map[string]interface{}{"name": "note", "type": "text", "default": "hi; drop"},
		},
	})
	if err != nil {
		t.Fatalf("handleCreateTable: %v", err)
	}
	sql := out.(map[string]interface{})["sql"].(string)
	if !strings.Contains(sql, "DEFAULT CURRENT_TIMESTAMP") {
		t.Fatalf("关键字 default 应直出: %s", sql)
	}
	if !strings.Contains(sql, `DEFAULT 'hi; drop'`) {
		t.Fatalf("危险 default 应按字符串引用: %s", sql)
	}
}

func TestHandleAlterTable(t *testing.T) {
	cases := []struct {
		op      string
		wantSQL string
	}{
		{"ADD", `ADD COLUMN "age" int`},
		{"MODIFY", `ALTER COLUMN "age" TYPE bigint`},
		{"DROP", `DROP COLUMN "age"`},
	}
	for _, c := range cases {
		mock := newMockPool(t)
		mock.ExpectExec(`ALTER TABLE "users"`).WillReturnResult(pgxmock.NewResult("ALTER TABLE", 0))

		params := map[string]interface{}{"table_name": "users", "operation": c.op, "column": "age"}
		if c.op != "DROP" {
			params["type"] = "int"
			if c.op == "MODIFY" {
				params["type"] = "bigint"
			}
		}
		out, err := handleAlterTable(params)
		if err != nil {
			t.Fatalf("handleAlterTable(%s): %v", c.op, err)
		}
		sql := out.(map[string]interface{})["sql"].(string)
		if !strings.Contains(sql, c.wantSQL) {
			t.Fatalf("SQL = %q, want contains %q", sql, c.wantSQL)
		}
	}
}

func TestHandleAlterTableInvalidOp(t *testing.T) {
	newMockPool(t)
	if _, err := handleAlterTable(map[string]interface{}{
		"table_name": "users", "operation": "REWRITE", "column": "c",
	}); err == nil {
		t.Fatal("非法 operation 应报错")
	}
}

func TestHandleDropTable(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectExec(`DROP TABLE IF EXISTS "users"`).
		WillReturnResult(pgxmock.NewResult("DROP TABLE", 0))

	out, err := handleDropTable(map[string]interface{}{"table_name": "users", "if_exists": true})
	if err != nil {
		t.Fatalf("handleDropTable: %v", err)
	}
	if sql := out.(map[string]interface{})["sql"].(string); !strings.Contains(sql, "IF EXISTS") {
		t.Fatalf("SQL = %s", sql)
	}
}

func TestHandleCreateIndex(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectExec(`CREATE UNIQUE INDEX "idx_users_name"`).
		WillReturnResult(pgxmock.NewResult("CREATE INDEX", 0))

	out, err := handleCreateIndex(map[string]interface{}{
		"index_name": "idx_users_name", "table_name": "users",
		"columns": []interface{}{"name"}, "unique": true,
	})
	if err != nil {
		t.Fatalf("handleCreateIndex: %v", err)
	}
	sql := out.(map[string]interface{})["sql"].(string)
	if !strings.Contains(sql, `ON "users" ("name")`) {
		t.Fatalf("SQL = %s", sql)
	}
}

func TestHandleDropIndex(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectExec(`DROP INDEX IF EXISTS "idx"`).
		WillReturnResult(pgxmock.NewResult("DROP INDEX", 0))

	if _, err := handleDropIndex(map[string]interface{}{"index_name": "idx", "if_exists": true}); err != nil {
		t.Fatalf("handleDropIndex: %v", err)
	}
}

func TestHandleExecuteDDL(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectExec(`ALTER TABLE "t" ADD COLUMN "c" int`).
		WillReturnResult(pgxmock.NewResult("ALTER TABLE", 0))

	out, err := handleExecuteDDL(map[string]interface{}{"sql": `ALTER TABLE "t" ADD COLUMN "c" int`})
	if err != nil {
		t.Fatalf("handleExecuteDDL: %v", err)
	}
	if res := out.(map[string]interface{}); res["success"] != true {
		t.Fatalf("result = %v", res)
	}
}

func TestHandleExecuteDDLMissingSQL(t *testing.T) {
	newMockPool(t)
	if _, err := handleExecuteDDL(map[string]interface{}{}); err == nil {
		t.Fatal("缺 sql 应报错")
	}
}

func TestHandleBatchCreateTablesNonAtomic(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectExec(`CREATE TABLE "a"`).WillReturnResult(pgxmock.NewResult("CREATE TABLE", 0))
	mock.ExpectExec(`CREATE TABLE "b"`).WillReturnResult(pgxmock.NewResult("CREATE TABLE", 0))

	out, err := handleBatchCreateTables(map[string]interface{}{
		"tables": []interface{}{
			map[string]interface{}{"table_name": "a", "columns": []interface{}{map[string]interface{}{"name": "id", "type": "int"}}},
			map[string]interface{}{"table_name": "b", "columns": []interface{}{map[string]interface{}{"name": "id", "type": "int"}}},
		},
	})
	if err != nil {
		t.Fatalf("handleBatchCreateTables: %v", err)
	}
	res := out.(map[string]interface{})
	if res["ok_count"] != 2 || res["atomic"] != false {
		t.Fatalf("result = %v", res)
	}
}

func TestHandleBatchDropTablesAtomic(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectBegin()
	mock.ExpectExec(`DROP TABLE "a"`).WillReturnResult(pgxmock.NewResult("DROP TABLE", 0))
	mock.ExpectExec(`DROP TABLE "b"`).WillReturnResult(pgxmock.NewResult("DROP TABLE", 0))
	mock.ExpectCommit()

	out, err := handleBatchDropTables(map[string]interface{}{
		"table_names": []interface{}{"a", "b"}, "atomic": true,
	})
	if err != nil {
		t.Fatalf("handleBatchDropTables: %v", err)
	}
	res := out.(map[string]interface{})
	if res["atomic"] != true || res["ok_count"] != 2 {
		t.Fatalf("result = %v", res)
	}
}

func TestKillDropIndexResult(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectExec(`DROP INDEX "i"`).WillReturnResult(pgxmock.NewResult("DROP INDEX", 0))
	out, err := handleDropIndex(map[string]interface{}{"index_name": "i"})
	if err != nil {
		t.Fatal(err)
	}
	if m := assertMap(t, out); m["success"] != true {
		t.Fatalf("result=%v", m)
	}
}

func TestKillBatchCreateTablesSuccessFlag(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectExec(`CREATE TABLE "a"`).WillReturnResult(pgxmock.NewResult("CREATE TABLE", 0))
	out, err := handleBatchCreateTables(map[string]interface{}{
		"tables": []interface{}{
			map[string]interface{}{"table_name": "a", "columns": []interface{}{map[string]interface{}{"name": "id", "type": "int"}}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if m := assertMap(t, out); m["success"] != true {
		t.Fatalf("result=%v", m)
	}
}

// ---- connection_limit > 0 边界变异：无 connection_limit 时 SQL 以 PASSWORD 结尾 ----
