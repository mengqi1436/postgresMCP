package main

import (
	"strings"
	"testing"
)

// ---- marshalToolResult：紧凑 JSON + 输出预算截断（token 优化）----

func TestMarshalToolResultCompact(t *testing.T) {
	s, err := marshalToolResult(map[string]interface{}{"rows": []interface{}{1, 2}, "count": 2})
	if err != nil {
		t.Fatalf("marshalToolResult: %v", err)
	}
	if strings.Contains(s, "\n  ") {
		t.Fatalf("输出不应含两空格缩进: %q", s)
	}
	// Go 的 encoding/json 对 map 键排序，顺序确定
	if !strings.HasPrefix(s, `{"count":2,"rows":[1,2]}`) {
		t.Fatalf("紧凑输出异常: %q", s)
	}
}

func TestMarshalToolResultBudget(t *testing.T) {
	big := strings.Repeat("x", maxOutputChars+1000)
	s, err := marshalToolResult(map[string]interface{}{"data": big})
	if err != nil {
		t.Fatalf("marshalToolResult: %v", err)
	}
	if !strings.Contains(s, "输出已截断") {
		t.Fatalf("应包含截断提示")
	}
	if len(s) >= len(big) {
		t.Fatalf("输出未被截断: len=%d（原始 %d）", len(s), len(big))
	}
}

// 截断按完整字符（rune）进行，不得切断多字节字符。
func TestMarshalToolResultBudgetKeepsRunes(t *testing.T) {
	big := strings.Repeat("中", maxOutputChars+100)
	s, err := marshalToolResult(map[string]interface{}{"data": big})
	if err != nil {
		t.Fatalf("marshalToolResult: %v", err)
	}
	if !strings.Contains(s, "输出已截断") {
		t.Fatalf("应包含截断提示")
	}
	if strings.ContainsRune(s, '\uFFFD') {
		t.Fatalf("截断切断了多字节字符")
	}
}

func TestMarshalToolResultUnderBudget(t *testing.T) {
	s, err := marshalToolResult(map[string]interface{}{"ok": true})
	if err != nil {
		t.Fatalf("marshalToolResult: %v", err)
	}
	if strings.Contains(s, "输出已截断") {
		t.Fatalf("小输出不应截断: %q", s)
	}
}

// 恰好等于输出预算的边界：n == maxOutputChars 不应截断（> 语义）。
// 变异 n>=maxOutputChars 会在此处截断，从而被本用例杀死。
func TestMarshalToolResultExactBudgetBoundary(t *testing.T) {
	const fixedLen = len(`{"data":"`) + len(`"}`) // JSON 外壳固定 11 字符
	x := maxOutputChars - fixedLen
	s, err := marshalToolResult(map[string]interface{}{"data": strings.Repeat("x", x)})
	if err != nil {
		t.Fatalf("marshalToolResult: %v", err)
	}
	if strings.Contains(s, "输出已截断") {
		t.Fatalf("恰好等于预算 %d 字符不应截断", maxOutputChars)
	}
}
