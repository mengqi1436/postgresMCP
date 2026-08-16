package database

import (
	"context"
	"encoding/binary"
	"errors"
	"testing"
	"time"

	"github.com/pashagolub/pgxmock/v5"
)

// newMockPool 创建 pgxmock 池并注入单例；测试结束校验期望。
func newMockPool(t *testing.T) pgxmock.PgxPoolIface {
	t.Helper()
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("创建 mock 池失败: %v", err)
	}
	SetPoolForTest(mock)
	t.Cleanup(func() {
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("mock 期望未全部满足: %v", err)
		}
	})
	return mock
}

func TestQueryScansRows(t *testing.T) {
	mock := newMockPool(t)
	ts := time.Date(2026, 8, 7, 12, 0, 0, 0, time.UTC)
	var uuid [16]byte
	uuid[0], uuid[1] = 0x12, 0x34
	mock.ExpectQuery("SELECT 1").
		WillReturnRows(pgxmock.NewRows([]string{"n", "b", "ts", "id", "nilv"}).
			AddRow(42, []byte("bytes"), ts, uuid, nil))

	rows, err := Query(context.Background(), "SELECT 1")
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d", len(rows))
	}
	r := rows[0]
	if r["n"] != 42 {
		t.Errorf("n = %v", r["n"])
	}
	if r["b"] != "bytes" {
		t.Errorf("b = %v", r["b"])
	}
	if r["ts"] != "2026-08-07T12:00:00Z" {
		t.Errorf("ts = %v", r["ts"])
	}
	if r["id"] != "12340000-0000-0000-0000-000000000000" {
		t.Errorf("id = %v", r["id"])
	}
	if r["nilv"] != nil {
		t.Errorf("nilv = %v", r["nilv"])
	}
}

// ---- QueryLimit：行数上限 + 截断标记 + 精确总数 ----

func TestQueryLimitUnderLimit(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectQuery("SELECT 1").
		WillReturnRows(pgxmock.NewRows([]string{"n"}).AddRow(1).AddRow(2))

	qr, err := QueryLimit(context.Background(), 5, "SELECT 1")
	if err != nil {
		t.Fatalf("QueryLimit: %v", err)
	}
	if qr.Truncated {
		t.Fatalf("未超限不应截断")
	}
	if qr.Total != 2 || len(qr.Rows) != 2 {
		t.Fatalf("Total=%d rows=%d, want 2/2", qr.Total, len(qr.Rows))
	}
}

func TestQueryLimitTruncates(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectQuery("SELECT 1").
		WillReturnRows(pgxmock.NewRows([]string{"n"}).AddRow(1).AddRow(2).AddRow(3).AddRow(4))

	qr, err := QueryLimit(context.Background(), 2, "SELECT 1")
	if err != nil {
		t.Fatalf("QueryLimit: %v", err)
	}
	if !qr.Truncated {
		t.Fatalf("超过 2 行应截断")
	}
	if len(qr.Rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(qr.Rows))
	}
	if qr.Total != 4 {
		t.Fatalf("Total = %d, want 4", qr.Total)
	}
}

func TestQueryLimitNoLimitWhenZero(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectQuery("SELECT 1").
		WillReturnRows(pgxmock.NewRows([]string{"n"}).AddRow(1).AddRow(2).AddRow(3))

	qr, err := QueryLimit(context.Background(), 0, "SELECT 1")
	if err != nil {
		t.Fatalf("QueryLimit: %v", err)
	}
	if qr.Truncated || qr.Total != 3 || len(qr.Rows) != 3 {
		t.Fatalf("maxRows=0 应不限制: truncated=%v total=%d rows=%d", qr.Truncated, qr.Total, len(qr.Rows))
	}
}

func TestExecuteReturnsRowsAffected(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectExec(`UPDATE "t"`).WillReturnResult(pgxmock.NewResult("UPDATE", 7))

	affected, err := Execute(context.Background(), `UPDATE "t" SET x = 1`)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if affected != 7 {
		t.Fatalf("affected = %d, want 7", affected)
	}
}

func TestExecuteStatementsCommits(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "a"`).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectExec(`INSERT INTO "b"`).WithArgs(1).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectCommit()

	err := ExecuteStatements(context.Background(), []Statement{
		{SQL: `UPDATE "a" SET x = 1`},
		{SQL: `INSERT INTO "b" (x) VALUES ($1)`, Args: []any{1}},
	})
	if err != nil {
		t.Fatalf("ExecuteStatements: %v", err)
	}
}

func TestExecuteStatementsRollsBackOnError(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "a"`).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectExec(`BAD`).WillReturnError(errors.New("boom"))
	mock.ExpectRollback()

	err := ExecuteStatements(context.Background(), []Statement{
		{SQL: `UPDATE "a" SET x = 1`},
		{SQL: `BAD`},
	})
	if err == nil {
		t.Fatal("应返回错误并回滚")
	}
}

func TestExecuteTransaction(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectBegin()
	mock.ExpectExec(`DELETE FROM "t"`).WillReturnResult(pgxmock.NewResult("DELETE", 1))
	mock.ExpectCommit()

	if err := ExecuteTransaction(context.Background(), []string{`DELETE FROM "t" WHERE id = 1`}); err != nil {
		t.Fatalf("ExecuteTransaction: %v", err)
	}
}

func TestQueryInAbortedTx(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectBegin()
	mock.ExpectQuery("EXPLAIN").
		WillReturnRows(pgxmock.NewRows([]string{"QUERY PLAN"}).AddRow("Seq Scan"))
	mock.ExpectRollback()

	rows, err := QueryInAbortedTx(context.Background(), "EXPLAIN (ANALYZE) UPDATE t SET x = 1")
	if err != nil {
		t.Fatalf("QueryInAbortedTx: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d", len(rows))
	}
}

func TestNormalizeValue(t *testing.T) {
	if normalizeValue(0, nil) != nil {
		t.Error("nil 应保持 nil")
	}
	if normalizeValue(0, []byte("x")) != "x" {
		t.Error("[]byte 应转 string")
	}
	ts := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	if normalizeValue(0, ts) != "2026-01-02T03:04:05Z" {
		t.Errorf("time = %v", normalizeValue(0, ts))
	}
	if normalizeValue(0, 5*time.Second) != "5s" {
		t.Errorf("duration = %v", normalizeValue(0, 5*time.Second))
	}
}

func TestNumericBinaryToString(t *testing.T) {
	// 12.34：digits=[12,3400], weight=0, dscale=2
	b := make([]byte, 12)
	binary.BigEndian.PutUint16(b[0:2], 2) // ndigits
	binary.BigEndian.PutUint16(b[2:4], 0) // weight
	binary.BigEndian.PutUint16(b[4:6], 0) // sign
	binary.BigEndian.PutUint16(b[6:8], 2) // dscale
	binary.BigEndian.PutUint16(b[8:10], 12)
	binary.BigEndian.PutUint16(b[10:12], 3400)
	if got, err := numericBinaryToString(b); err != nil || got != "12.34" {
		t.Fatalf("12.34 = %q, err %v", got, err)
	}

	// -1
	b = make([]byte, 10)
	binary.BigEndian.PutUint16(b[0:2], 1)
	binary.BigEndian.PutUint16(b[2:4], 0)
	binary.BigEndian.PutUint16(b[4:6], 0x4000) // 负号
	binary.BigEndian.PutUint16(b[6:8], 0)
	binary.BigEndian.PutUint16(b[8:10], 1)
	if got, _ := numericBinaryToString(b); got != "-1" {
		t.Fatalf("-1 = %q", got)
	}

	// NaN
	b = make([]byte, 8)
	binary.BigEndian.PutUint16(b[0:2], 0)
	binary.BigEndian.PutUint16(b[2:4], 0)
	binary.BigEndian.PutUint16(b[4:6], 0xC000)
	binary.BigEndian.PutUint16(b[6:8], 0)
	if got, _ := numericBinaryToString(b); got != "NaN" {
		t.Fatalf("NaN = %q", got)
	}

	// 过短数据报错
	if _, err := numericBinaryToString([]byte{1, 2}); err == nil {
		t.Fatal("短数据应报错")
	}

	// 小数补零：0.50 → digits=[5000], weight=-1, dscale=2
	b = make([]byte, 10)
	binary.BigEndian.PutUint16(b[0:2], 1)
	binary.BigEndian.PutUint16(b[2:4], 0xFFFF) // weight=-1
	binary.BigEndian.PutUint16(b[4:6], 0)
	binary.BigEndian.PutUint16(b[6:8], 2)
	binary.BigEndian.PutUint16(b[8:10], 5000)
	if got, _ := numericBinaryToString(b); got != "0.50" {
		t.Fatalf("0.50 = %q", got)
	}
}

func TestNumericCodecTextFormat(t *testing.T) {
	c := &numericStringCodec{}
	// text 格式（TextFormatCode = 0）
	got, err := c.DecodeValue(nil, 0, 0, []byte("123.45"))
	if err != nil || got != "123.45" {
		t.Fatalf("text 格式 = %v, err %v", got, err)
	}
	// 二进制格式（BinaryFormatCode = 1）走 numericBinaryToString
	if _, err := c.DecodeValue(nil, 0, 1, []byte{1}); err == nil {
		t.Fatal("二进制短数据应报错")
	}
}

// ndigits=3 且缓冲区恰好 8+2*3=14 字节：覆盖多位解析路径，
// 并杀长度校验 8+2*ndigits 的算术变异（如 + → * 使阈值变大）。
func TestNumericBinaryToStringThreeDigits(t *testing.T) {
	b := make([]byte, 14)
	binary.BigEndian.PutUint16(b[0:2], 3) // ndigits
	binary.BigEndian.PutUint16(b[2:4], 2) // weight
	binary.BigEndian.PutUint16(b[4:6], 0) // sign
	binary.BigEndian.PutUint16(b[6:8], 0) // dscale
	binary.BigEndian.PutUint16(b[8:10], 1)
	binary.BigEndian.PutUint16(b[10:12], 2)
	binary.BigEndian.PutUint16(b[12:14], 3)
	got, err := numericBinaryToString(b)
	if err != nil || got != "100020003" {
		t.Fatalf("ndigits=3 = %q, err %v", got, err)
	}
}
