package main

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// textResult 返回文本内容成功结果。
func textResult(s string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: s}}}
}

// errorResult 返回业务错误结果：IsError=true 且 Content 为错误文本，会话继续。
func errorResult(err error) *mcp.CallToolResult {
	r := &mcp.CallToolResult{}
	r.SetError(err)
	return r
}
