package tools

import (
	"testing"

	"github.com/pashagolub/pgxmock/v5"
)

func TestHandleDatabaseInfo(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectQuery("version()").
		WillReturnRows(pgxmock.NewRows([]string{"version"}).AddRow("PostgreSQL 17.10"))

	out, err := handleDatabaseInfo(map[string]interface{}{})
	if err != nil {
		t.Fatalf("handleDatabaseInfo: %v", err)
	}
	if out.(map[string]interface{})["version"] != "PostgreSQL 17.10" {
		t.Fatalf("result = %v", out)
	}
}

func TestHandleListUsers(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectQuery("pg_roles").
		WillReturnRows(pgxmock.NewRows([]string{"role_name"}).AddRow("postgres"))

	out, err := handleListUsers(map[string]interface{}{})
	if err != nil {
		t.Fatalf("handleListUsers: %v", err)
	}
	if res := out.(map[string]interface{}); res["count"] != 1 {
		t.Fatalf("count = %v", res["count"])
	}
}

func TestHandleCreateUser(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectExec(`CREATE ROLE "alice" LOGIN PASSWORD`).
		WillReturnResult(pgxmock.NewResult("CREATE ROLE", 0))

	out, err := handleCreateUser(map[string]interface{}{
		"username": "alice", "password": "s3cret", "connection_limit": 5,
	})
	if err != nil {
		t.Fatalf("handleCreateUser: %v", err)
	}
	sqlStr := "" // 通过返回确认
	if res := out.(map[string]interface{}); res["username"] != "alice" {
		t.Fatalf("result = %v", res)
	}
	_ = sqlStr
}

func TestHandleCreateUserRequiresPassword(t *testing.T) {
	newMockPool(t)
	if _, err := handleCreateUser(map[string]interface{}{"username": "alice"}); err == nil {
		t.Fatal("缺 password 应报错")
	}
}

func TestHandleDropUser(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectExec(`DROP ROLE "alice"`).
		WillReturnResult(pgxmock.NewResult("DROP ROLE", 0))

	if _, err := handleDropUser(map[string]interface{}{"username": "alice"}); err != nil {
		t.Fatalf("handleDropUser: %v", err)
	}
}

func TestHandleGrantPrivilege(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectExec(`GRANT SELECT ON TABLE users TO "alice"`).
		WillReturnResult(pgxmock.NewResult("GRANT", 0))

	out, err := handleGrantPrivilege(map[string]interface{}{
		"privilege": "SELECT ON TABLE users", "grantee": "alice",
	})
	if err != nil {
		t.Fatalf("handleGrantPrivilege: %v", err)
	}
	if res := out.(map[string]interface{}); res["success"] != true {
		t.Fatalf("result = %v", res)
	}
}

func TestHandleGrantPrivilegeRejectsInjection(t *testing.T) {
	newMockPool(t)
	if _, err := handleGrantPrivilege(map[string]interface{}{
		"privilege": "ALL; DROP TABLE users", "grantee": "alice",
	}); err == nil {
		t.Fatal("含分号的 privilege 应被拒绝")
	}
}

func TestHandleCreateRole(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectExec(`CREATE ROLE "reader"`).
		WillReturnResult(pgxmock.NewResult("CREATE ROLE", 0))

	if _, err := handleCreateRole(map[string]interface{}{"role_name": "reader"}); err != nil {
		t.Fatalf("handleCreateRole: %v", err)
	}
}

func TestHandleCreateTablespace(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectExec("CREATE TABLESPACE").
		WillReturnResult(pgxmock.NewResult("CREATE TABLESPACE", 0))

	out, err := handleCreateTablespace(map[string]interface{}{
		"tablespace_name": "ts", "datafile": "/data/ts",
	})
	if err != nil {
		t.Fatalf("handleCreateTablespace: %v", err)
	}
	if res := out.(map[string]interface{}); res["success"] != true {
		t.Fatalf("result = %v", res)
	}
}

func TestHandleTableStatistics(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectQuery("pg_stat_user_tables").
		WithArgs("users").
		WillReturnRows(pgxmock.NewRows([]string{"relname"}).AddRow("users"))

	out, err := handleTableStatistics(map[string]interface{}{"table_name": "users"})
	if err != nil {
		t.Fatalf("handleTableStatistics: %v", err)
	}
	if res := out.(map[string]interface{}); res["count"] != 1 {
		t.Fatalf("count = %v", res["count"])
	}
}

func TestKillDropUserResult(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectExec(`DROP ROLE "u"`).WillReturnResult(pgxmock.NewResult("DROP ROLE", 0))
	out, err := handleDropUser(map[string]interface{}{"username": "u"})
	if err != nil {
		t.Fatal(err)
	}
	if m := assertMap(t, out); m["success"] != true {
		t.Fatalf("result=%v", m)
	}
}

func TestKillCreateRoleResult(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectExec(`CREATE ROLE "r"`).WillReturnResult(pgxmock.NewResult("CREATE ROLE", 0))
	out, err := handleCreateRole(map[string]interface{}{"role_name": "r"})
	if err != nil {
		t.Fatal(err)
	}
	if m := assertMap(t, out); m["success"] != true {
		t.Fatalf("result=%v", m)
	}
}

func TestKillCreateUserNoConnLimit(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectExec(`PASSWORD 'p'$`).WillReturnResult(pgxmock.NewResult("CREATE ROLE", 0))
	out, err := handleCreateUser(map[string]interface{}{"username": "u", "password": "p"})
	if err != nil {
		t.Fatal(err)
	}
	assertMap(t, out)
}
