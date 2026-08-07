package tools

import (
	"fmt"
	"sync"

	"pg-mcp/config"
)

// ToolInfo 工具信息
type ToolInfo struct {
	Name        string   `json:"name"`
	Category    string   `json:"category"`
	Description string   `json:"description"`
	Params      []string `json:"params"`
}

// ToolHandler 工具处理函数
type ToolHandler func(params map[string]interface{}) (interface{}, error)

var (
	toolRegistry = make(map[string]ToolInfo)
	toolHandlers = make(map[string]ToolHandler)
	mu           sync.RWMutex
)

// restrictedAllowedCategories 受限模式允许的类别（只读/元数据/监控）。
var restrictedAllowedCategories = map[string]bool{
	"query":      true,
	"metadata":   true,
	"monitoring": true,
}

// restrictedAllowedTools 受限模式额外放行的工具（只读安全）。
var restrictedAllowedTools = map[string]bool{
	"explain_plan": true, // EXPLAIN 不真正执行语句；ANALYZE 写操作会被连接级只读拦截
}

// RegisterTool 注册工具
func RegisterTool(info ToolInfo, handler ToolHandler) {
	mu.Lock()
	defer mu.Unlock()
	toolRegistry[info.Name] = info
	toolHandlers[info.Name] = handler
}

// GetAllTools 获取所有工具（可按类别筛选）
func GetAllTools(category string) []ToolInfo {
	mu.RLock()
	defer mu.RUnlock()

	var result []ToolInfo
	for _, info := range toolRegistry {
		if category == "" || info.Category == category {
			result = append(result, info)
		}
	}
	return result
}

// ExecuteTool 执行指定工具。restricted 模式下先做工具级白名单检查
// （连接级 default_transaction_read_only 是第二道防线）。
func ExecuteTool(name string, params map[string]interface{}) (interface{}, error) {
	mu.RLock()
	handler, exists := toolHandlers[name]
	info := toolRegistry[name]
	mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("工具 '%s' 不存在", name)
	}

	if params == nil {
		params = make(map[string]interface{})
	}

	if config.Get().IsRestricted() &&
		!restrictedAllowedCategories[info.Category] &&
		!restrictedAllowedTools[name] {
		return nil, fmt.Errorf("restricted 模式禁止执行工具 '%s'（类别 %s）：只读模式下仅允许查询/元数据/监控类工具，如需写操作请以 PG_ACCESS_MODE=unrestricted 启动", name, info.Category)
	}

	return handler(params)
}

// GetCategories 获取所有类别
func GetCategories() []string {
	mu.RLock()
	defer mu.RUnlock()

	categoryMap := make(map[string]bool)
	for _, info := range toolRegistry {
		categoryMap[info.Category] = true
	}

	var categories []string
	for cat := range categoryMap {
		categories = append(categories, cat)
	}
	return categories
}
