//go:build test

package config

import "sync"

// ResetForTest 重置配置单例，让 LoadConfig 重新读取环境变量。
// 仅测试构建（go test -tags test）编译，生产二进制（go build）不含此钩子。
func ResetForTest() {
	loadOnce = sync.Once{}
	cached = nil
}
