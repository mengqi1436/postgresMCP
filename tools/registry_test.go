package tools

import (
	"os"
	"strings"
	"testing"

	"pg-mcp/config"
)

func TestExecuteToolUnknown(t *testing.T) {
	_, err := ExecuteTool("no_such_tool", nil)
	if err == nil || !strings.Contains(err.Error(), "不存在") {
		t.Fatalf("ExecuteTool(unknown) = %v, want '不存在'", err)
	}
}

// withRestrictedConfig 临时切换到 restricted 模式（白名单用例用）。
func withRestrictedConfig(t *testing.T) {
	t.Helper()
	os.Setenv("PG_ACCESS_MODE", "restricted")
	config.ResetForTest()
	t.Cleanup(func() {
		os.Unsetenv("PG_ACCESS_MODE")
		config.ResetForTest()
	})
}

// TestExecuteToolRestrictedWhitelist：restricted 模式只放行
// query/metadata/monitoring 类别与 explain_plan；写工具被白名单拦截。
func TestExecuteToolRestrictedWhitelist(t *testing.T) {
	withRestrictedConfig(t)
	if !config.Get().IsRestricted() {
		t.Fatal("config should be restricted")
	}

	// 写工具被白名单拦截
	_, err := ExecuteTool("insert", map[string]interface{}{"table": "t", "data": map[string]interface{}{}})
	if err == nil || !strings.Contains(err.Error(), "restricted 模式禁止执行") {
		t.Fatalf("ExecuteTool(insert) = %v, want restricted block", err)
	}
	// 管理/备份/导入类同样拦截
	for _, name := range []string{"create_user", "logical_export", "batch_import_csv", "stop_database_service"} {
		if _, err := ExecuteTool(name, map[string]interface{}{}); err == nil || !strings.Contains(err.Error(), "restricted 模式禁止执行") {
			t.Errorf("ExecuteTool(%s) = %v, want restricted block", name, err)
		}
	}

	// 只读工具通过白名单（后续因无数据库报连接错误，而非白名单错误）
	_, err = ExecuteTool("query", map[string]interface{}{"sql": "SELECT 1"})
	if err == nil {
		t.Fatal("query 应因无数据库报错")
	}
	if strings.Contains(err.Error(), "restricted 模式禁止执行") {
		t.Fatalf("query 不应被白名单拦截: %v", err)
	}

	// explain_plan 额外放行
	_, err = ExecuteTool("explain_plan", map[string]interface{}{"sql": "SELECT 1"})
	if err == nil {
		t.Fatal("explain_plan 应因无数据库报错")
	}
	if strings.Contains(err.Error(), "restricted 模式禁止执行") {
		t.Fatalf("explain_plan 不应被白名单拦截: %v", err)
	}
}

func TestExecuteToolUnrestrictedAllowsAll(t *testing.T) {
	os.Setenv("PG_ACCESS_MODE", "unrestricted")
	config.ResetForTest()
	t.Cleanup(func() {
		os.Unsetenv("PG_ACCESS_MODE")
		config.ResetForTest()
	})
	if config.Get().IsRestricted() {
		t.Fatal("config should be unrestricted")
	}
	// 写工具不被白名单拦截（因无数据库报连接错误，而非白名单错误）
	_, err := ExecuteTool("insert", map[string]interface{}{"table": "t", "data": map[string]interface{}{}})
	if err == nil {
		t.Fatal("insert 应因无数据库报错")
	}
	if strings.Contains(err.Error(), "restricted 模式禁止执行") {
		t.Fatalf("unrestricted 下 insert 不应被白名单拦截: %v", err)
	}
}

func TestGetAllToolsFilterByCategory(t *testing.T) {
	all := GetAllTools("")
	if len(all) < 70 {
		t.Fatalf("expected >= 70 tools, got %d", len(all))
	}
	query := GetAllTools("query")
	if len(query) == 0 {
		t.Fatal("no query-category tools")
	}
	for _, info := range query {
		if info.Category != "query" {
			t.Fatalf("tool %s category = %q, want query", info.Name, info.Category)
		}
	}
}
