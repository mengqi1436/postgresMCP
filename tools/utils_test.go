package tools

import (
	"strings"
	"testing"
)

func TestValidateIdentifier(t *testing.T) {
	valid := []string{"users", "user_1", "_t", "table$1", "A", strings.Repeat("a", 63)}
	for _, v := range valid {
		if err := validateIdentifier(v); err != nil {
			t.Errorf("validateIdentifier(%q) = %v, want nil", v, err)
		}
	}

	invalid := []string{"", "1abc", "a-b", "a b", `a"b`, "a;b", strings.Repeat("a", 64)}
	for _, v := range invalid {
		if err := validateIdentifier(v); err == nil {
			t.Errorf("validateIdentifier(%q) = nil, want error", v)
		}
	}
}

func TestQuoteIdent(t *testing.T) {
	cases := map[string]string{
		"users":   `"users"`,
		`weird"x`: `"weird""x"`,
		"":        `""`,
	}
	for in, want := range cases {
		if got := quoteIdent(in); got != want {
			t.Errorf("quoteIdent(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestQuoteLiteral(t *testing.T) {
	cases := map[string]string{
		"abc":        `'abc'`,
		"it's":       `'it''s'`,
		`back\slash`: `'back\slash'`,
	}
	for in, want := range cases {
		if got := quoteLiteral(in); got != want {
			t.Errorf("quoteLiteral(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSplitSchemaName(t *testing.T) {
	cases := []struct {
		in     string
		schema string
		name   string
	}{
		{"public.users", "public", "users"},
		{"users", "", "users"},
		{"a.b.c", "a", "b.c"},
		{"  public  .  users  ", "public", "users"},
	}
	for _, c := range cases {
		s, n := splitSchemaName(c.in)
		if s != c.schema || n != c.name {
			t.Errorf("splitSchemaName(%q) = (%q,%q), want (%q,%q)", c.in, s, n, c.schema, c.name)
		}
	}
}

func TestQualifiedTable(t *testing.T) {
	if got := qualifiedTable("", "users"); got != `"users"` {
		t.Errorf("qualifiedTable('',users) = %q", got)
	}
	if got := qualifiedTable("public", "users"); got != `"public"."users"` {
		t.Errorf("qualifiedTable(public,users) = %q", got)
	}
}

// clampLimit 边界精确值：1 与 10000 不应钳制，0 与 10001 应回退默认值。
// 针对 <1 / >10000 的条件边界变异（<=1 / >=10000）在精确边界值上行为不同。
func TestClampLimitBoundary(t *testing.T) {
	if got := clampLimit(1); got != 1 {
		t.Errorf("clampLimit(1) = %d, want 1", got)
	}
	if got := clampLimit(10000); got != 10000 {
		t.Errorf("clampLimit(10000) = %d, want 10000", got)
	}
	if got := clampLimit(0); got != DefaultMaxRows {
		t.Errorf("clampLimit(0) = %d, want %d", got, DefaultMaxRows)
	}
	if got := clampLimit(10001); got != DefaultMaxRows {
		t.Errorf("clampLimit(10001) = %d, want %d", got, DefaultMaxRows)
	}
}

func TestParamExtractors(t *testing.T) {
	params := map[string]interface{}{
		"str":     "  hello  ",
		"b":       true,
		"n":       float64(42),
		"ns":      "7",
		"arr":     []interface{}{"a", "b"},
		"obj":     map[string]interface{}{"k": "v"},
		"nothing": nil,
	}
	if got := getString(params, "str"); got != "hello" {
		t.Errorf("getString = %q", got)
	}
	if !getBool(params, "b") || getBool(params, "missing") {
		t.Errorf("getBool mismatch")
	}
	if got := getIntDefault(params, "n", 1); got != 42 {
		t.Errorf("getIntDefault(n) = %d", got)
	}
	if got := getIntDefault(params, "ns", 1); got != 7 {
		t.Errorf("getIntDefault(ns) = %d", got)
	}
	if got := getIntDefault(params, "missing", 9); got != 9 {
		t.Errorf("getIntDefault(missing) = %d", got)
	}
	if got := getArray(params, "arr"); len(got) != 2 {
		t.Errorf("getArray = %v", got)
	}
	if got := getStringSlice(params, "arr"); len(got) != 2 || got[0] != "a" {
		t.Errorf("getStringSlice = %v", got)
	}
	if got := getObject(params, "obj"); got == nil || got["k"] != "v" {
		t.Errorf("getObject = %v", got)
	}
}
