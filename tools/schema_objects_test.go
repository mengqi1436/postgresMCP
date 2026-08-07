package tools

import (
	"strings"
	"testing"

	"github.com/pashagolub/pgxmock/v5"
)

func TestHandleCreateView(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectExec(`CREATE OR REPLACE VIEW "v" AS SELECT 1`).
		WillReturnResult(pgxmock.NewResult("CREATE VIEW", 0))

	out, err := handleCreateView(map[string]interface{}{
		"view_name": "v", "sql": "SELECT 1", "or_replace": true,
	})
	if err != nil {
		t.Fatalf("handleCreateView: %v", err)
	}
	sql := out.(map[string]interface{})["sql"].(string)
	if !strings.Contains(sql, "CREATE OR REPLACE VIEW") {
		t.Fatalf("SQL = %s", sql)
	}
}

func TestHandleCreateViewMissingSQL(t *testing.T) {
	newMockPool(t)
	if _, err := handleCreateView(map[string]interface{}{"view_name": "v"}); err == nil {
		t.Fatal("缺 sql 应报错")
	}
}

func TestHandleDropView(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectExec(`DROP VIEW IF EXISTS "v"`).
		WillReturnResult(pgxmock.NewResult("DROP VIEW", 0))

	if _, err := handleDropView(map[string]interface{}{"view_name": "v", "if_exists": true}); err != nil {
		t.Fatalf("handleDropView: %v", err)
	}
}

func TestHandleCreateSequence(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectExec(`CREATE SEQUENCE "seq"`).
		WillReturnResult(pgxmock.NewResult("CREATE SEQUENCE", 0))

	out, err := handleCreateSequence(map[string]interface{}{
		"seq_name": "seq", "start_with": 10, "increment_by": 5,
	})
	if err != nil {
		t.Fatalf("handleCreateSequence: %v", err)
	}
	sql := out.(map[string]interface{})["sql"].(string)
	if !strings.Contains(sql, "START WITH 10") || !strings.Contains(sql, "INCREMENT BY 5") {
		t.Fatalf("SQL = %s", sql)
	}
}

func TestHandleDropSequence(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectExec(`DROP SEQUENCE "seq"`).
		WillReturnResult(pgxmock.NewResult("DROP SEQUENCE", 0))

	if _, err := handleDropSequence(map[string]interface{}{"seq_name": "seq"}); err != nil {
		t.Fatalf("handleDropSequence: %v", err)
	}
}

// create_sequence 可选子句：max_value/cache_size 守卫（杀 CONDITIONALS_NEGATION）。
func TestHandleCreateSequenceAllOptions(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectExec(`CREATE SEQUENCE "seq"`).WillReturnResult(pgxmock.NewResult("CREATE SEQUENCE", 0))

	out, err := handleCreateSequence(map[string]interface{}{
		"seq_name": "seq", "start_with": 1, "increment_by": 1, "max_value": 999, "cache_size": 10,
	})
	if err != nil {
		t.Fatalf("handleCreateSequence: %v", err)
	}
	sql := out.(map[string]interface{})["sql"].(string)
	if !strings.Contains(sql, "MAXVALUE 999") || !strings.Contains(sql, "CACHE 10") {
		t.Fatalf("SQL 缺少 MAXVALUE/CACHE: %s", sql)
	}
}

func TestKillDropViewResult(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectExec(`DROP VIEW "v"`).WillReturnResult(pgxmock.NewResult("DROP VIEW", 0))
	out, err := handleDropView(map[string]interface{}{"view_name": "v"})
	if err != nil {
		t.Fatal(err)
	}
	if m := assertMap(t, out); m["success"] != true {
		t.Fatalf("result=%v", m)
	}
}

func TestKillDropSequenceResult(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectExec(`DROP SEQUENCE "s"`).WillReturnResult(pgxmock.NewResult("DROP SEQUENCE", 0))
	out, err := handleDropSequence(map[string]interface{}{"seq_name": "s"})
	if err != nil {
		t.Fatal(err)
	}
	if m := assertMap(t, out); m["success"] != true {
		t.Fatalf("result=%v", m)
	}
}
