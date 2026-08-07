package tools

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// 默认工具级超时（数据库操作）。
const defaultToolTimeout = 60 * time.Second

// toolContext 返回带默认超时的 context。
func toolContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), defaultToolTimeout)
}

// toolContextWithTimeout 返回自定义超时的 context（长耗时工具：导入/备份）。
func toolContextWithTimeout(d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), d)
}

// getString 提取字符串参数（trim 后返回）。
func getString(params map[string]interface{}, key string) string {
	if v, ok := params[key].(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}

// getBool 提取布尔参数。
func getBool(params map[string]interface{}, key string) bool {
	if v, ok := params[key].(bool); ok {
		return v
	}
	return false
}

// getIntDefault 提取整数参数（JSON 数字为 float64；也兼容字符串），缺省返回 def。
func getIntDefault(params map[string]interface{}, key string, def int) int {
	v, ok := params[key]
	if !ok || v == nil {
		return def
	}
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case string:
		if i, err := strconv.Atoi(strings.TrimSpace(n)); err == nil {
			return i
		}
	}
	return def
}

// getArray 提取数组参数。
func getArray(params map[string]interface{}, key string) []interface{} {
	if v, ok := params[key].([]interface{}); ok {
		return v
	}
	return nil
}

// getStringSlice 提取字符串数组参数。
func getStringSlice(params map[string]interface{}, key string) []string {
	arr := getArray(params, key)
	if arr == nil {
		return nil
	}
	result := make([]string, 0, len(arr))
	for _, item := range arr {
		if s, ok := item.(string); ok {
			result = append(result, s)
		}
	}
	return result
}

// getObject 提取对象参数。
func getObject(params map[string]interface{}, key string) map[string]interface{} {
	if v, ok := params[key].(map[string]interface{}); ok {
		return v
	}
	return nil
}

// toIntSlice 把 pgx 返回的整数数组（[]int16 / []int32 / []int64 / []any）统一转成 []int。
func toIntSlice(v interface{}) []int {
	switch arr := v.(type) {
	case []int16:
		out := make([]int, len(arr))
		for i, x := range arr {
			out[i] = int(x)
		}
		return out
	case []int32:
		out := make([]int, len(arr))
		for i, x := range arr {
			out[i] = int(x)
		}
		return out
	case []int64:
		out := make([]int, len(arr))
		for i, x := range arr {
			out[i] = int(x)
		}
		return out
	case []interface{}:
		var out []int
		for _, x := range arr {
			switch n := x.(type) {
			case int16:
				out = append(out, int(n))
			case int32:
				out = append(out, int(n))
			case int64:
				out = append(out, int(n))
			case float64:
				out = append(out, int(n))
			}
		}
		return out
	}
	return nil
}

// identPattern 用于校验裸标识符（不带引号时）。
var identPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_$]*$`)

// validateIdentifier 校验标识符，防止 SQL 注入。
func validateIdentifier(name string) error {
	if name == "" {
		return fmt.Errorf("标识符不能为空")
	}
	if len(name) > 63 {
		return fmt.Errorf("标识符 '%s' 超过 63 字节上限", name)
	}
	if !identPattern.MatchString(name) {
		return fmt.Errorf("标识符 '%s' 含非法字符（仅允许字母/数字/下划线/$，且不能以数字开头）", name)
	}
	return nil
}

// quoteIdent 引用标识符（双写内部引号），用于拼接 SQL 中的表名/列名。
func quoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

// quoteLiteral 引用字符串字面量（单引号，双写内部单引号）。
func quoteLiteral(s string) string {
	return `'` + strings.ReplaceAll(s, `'`, `''`) + `'`
}

// splitSchemaName 把 "schema.name" 拆成 (schema, name)；无 schema 时返回 ""。
func splitSchemaName(qualified string) (string, string) {
	parts := strings.SplitN(qualified, ".", 2)
	if len(parts) == 2 {
		return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
	}
	return "", qualified
}

// qualifiedTable 生成 schema 限定的表引用（自动引用转义）。schema 为空时只用表名。
func qualifiedTable(schema, table string) string {
	if schema != "" {
		return quoteIdent(schema) + "." + quoteIdent(table)
	}
	return quoteIdent(table)
}
