//go:build test

package database

import "sync"

// SetPoolForTest 替换进程级连接池并标记已初始化（注入 pgxmock）。
// 仅测试构建（go test -tags test）编译，生产二进制（go build）不含此钩子。
func SetPoolForTest(p Pool) {
	once = sync.Once{}
	pool = p
	initErr = nil
	once.Do(func() {}) // 标记已初始化，后续 GetPool 直接返回注入的池
}
