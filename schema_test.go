package main

import (
	"encoding/json"
	"strings"
	"testing"

	"pg-mcp/tools"
)

func TestBuildSchemaQuery(t *testing.T) {
	info := tools.ToolInfo{Name: "query", Params: []string{"sql", "params"}}
	s := buildSchema(info)

	if s.Type != "object" {
		t.Fatalf("schema type = %q, want object", s.Type)
	}
	if len(s.Required) != 1 || s.Required[0] != "sql" {
		t.Fatalf("required = %v, want [sql]", s.Required)
	}
	if p := s.Properties["sql"]; p == nil || p.Type != "string" {
		t.Fatalf("sql property = %+v, want type string", p)
	}
	if p := s.Properties["params"]; p == nil || p.Type != "array" {
		t.Fatalf("params property = %+v, want type array", p)
	} else if p.Items != nil {
		t.Fatalf("params items = %+v, want nil（绑定值类型不一）", p.Items)
	}
}

func TestBuildSchemaInsert(t *testing.T) {
	info := tools.ToolInfo{Name: "insert", Params: []string{"table", "data"}}
	s := buildSchema(info)

	want := []string{"table", "data"}
	if len(s.Required) != 2 {
		t.Fatalf("required = %v, want %v", s.Required, want)
	}
	for _, r := range want {
		if !contains(s.Required, r) {
			t.Fatalf("required %v missing %q", s.Required, r)
		}
	}
	if p := s.Properties["data"]; p == nil || p.Type != "object" {
		t.Fatalf("data property = %+v, want type object", p)
	}
}

func TestUnknownParamDefaultsToString(t *testing.T) {
	info := tools.ToolInfo{Name: "custom", Params: []string{"weird_param"}}
	s := buildSchema(info)
	if p := s.Properties["weird_param"]; p == nil || p.Type != "string" {
		t.Fatalf("unknown param = %+v, want type string", p)
	}
	if p := s.Properties["weird_param"]; p.Description != "weird_param" {
		t.Fatalf("unknown param description = %q, want param name", p.Description)
	}
}

func TestSchemaMarshalsToValidJSONSchema(t *testing.T) {
	info := tools.ToolInfo{Name: "query", Params: []string{"sql", "params"}}
	data, err := json.Marshal(buildSchema(info))
	if err != nil {
		t.Fatalf("marshal schema: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("schema is not valid JSON: %v", err)
	}
	if m["type"] != "object" {
		t.Fatalf("marshaled type = %v, want object", m["type"])
	}
	req, ok := m["required"].([]any)
	if !ok || len(req) != 1 || req[0] != "sql" {
		t.Fatalf("marshaled required = %v, want [sql]", m["required"])
	}
	if _, ok := m["properties"].(map[string]any); !ok {
		t.Fatalf("marshaled properties missing")
	}
}

func TestToolTitleDerivation(t *testing.T) {
	info := tools.ToolInfo{
		Name:        "query",
		Description: "执行 SELECT 查询。参数: sql(必需), params(可选)",
	}
	if got := toolTitle(info); got != "执行 SELECT 查询" {
		t.Fatalf("toolTitle = %q, want 执行 SELECT 查询", got)
	}

	fallback := tools.ToolInfo{Name: "no_desc"}
	if got := toolTitle(fallback); got != "no_desc" {
		t.Fatalf("toolTitle fallback = %q, want no_desc", got)
	}
}

func TestAllRegisteredToolsHaveObjectSchemas(t *testing.T) {
	infos := tools.GetAllTools("")
	if len(infos) < 70 {
		t.Fatalf("expected at least 70 registered tools, got %d", len(infos))
	}
	for _, info := range infos {
		s := buildSchema(info)
		if s.Type != "object" {
			t.Fatalf("tool %s: schema type = %q, want object", info.Name, s.Type)
		}
		for _, p := range info.Params {
			if s.Properties[p] == nil {
				t.Fatalf("tool %s: param %q missing from schema", info.Name, p)
			}
		}
		for _, r := range toolRequired[info.Name] {
			if !contains(info.Params, r) {
				t.Fatalf("tool %s: required %q not in Params %v", info.Name, r, info.Params)
			}
		}
		if info.Description != "" {
			if got := toolTitle(info); got == "" {
				t.Fatalf("tool %s: derived title is empty", info.Name)
			}
		}
	}
}

func contains(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}

// 顺带校验 title 不包含描述中的参数说明部分（首句截断正确）。
func TestTitlesDoNotLeakParamDocs(t *testing.T) {
	for _, info := range tools.GetAllTools("") {
		title := toolTitle(info)
		if strings.Contains(title, "参数:") || strings.Contains(title, "(必需)") {
			t.Fatalf("tool %s: title %q leaks param docs", info.Name, title)
		}
	}
}
