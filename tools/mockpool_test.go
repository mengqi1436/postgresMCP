package tools

import (
	"context"
	"fmt"
	"testing"

	"pg-mcp/database"

	"github.com/jackc/pgx/v5"
	"github.com/pashagolub/pgxmock/v5"
)

// mockPool 在 pgxmock.PgxPoolIface 之上补上 Begin 方法
// （pgxmockPool 实现了 Begin，但未在 PgxPoolIface 接口中声明）。
type mockPool struct {
	pgxmock.PgxPoolIface
}

func (m *mockPool) Begin(ctx context.Context) (pgx.Tx, error) {
	bp, ok := m.PgxPoolIface.(interface {
		Begin(context.Context) (pgx.Tx, error)
	})
	if !ok {
		return nil, fmt.Errorf("mock 池不支持 Begin")
	}
	return bp.Begin(ctx)
}

// newMockPool 创建 pgxmock 池并注入 database 单例；测试结束校验期望全部满足。
func newMockPool(t *testing.T) pgxmock.PgxPoolIface {
	t.Helper()
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("创建 mock 池失败: %v", err)
	}
	database.SetPoolForTest(&mockPool{PgxPoolIface: mock})
	t.Cleanup(func() {
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("mock 期望未全部满足: %v", err)
		}
	})
	return mock
}

func assertMap(t *testing.T, out interface{}) map[string]interface{} {
	t.Helper()
	m, ok := out.(map[string]interface{})
	if !ok || m == nil {
		t.Fatalf("结果应为非空 map, got %#v", out)
	}
	return m
}

// ---- 错误守卫取反变异：断言成功结果内容（杀 err!=nil → err==nil 返回 (nil,nil)）----
