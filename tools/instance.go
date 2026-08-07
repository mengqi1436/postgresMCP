package tools

import (
	"fmt"
	"os"
	"strings"

	"pg-mcp/database"
)

func init() { registerInstanceTools() }

func registerInstanceTools() {
	RegisterTool(ToolInfo{
		Name:        "create_database",
		Category:    "instance",
		Description: "创建数据库（CREATE DATABASE）。参数: database_name(必需), owner(可选), encoding(可选, 如 UTF8)",
		Params:      []string{"database_name", "owner", "encoding"},
	}, handleCreateDatabase)

	RegisterTool(ToolInfo{
		Name:        "delete_database",
		Category:    "instance",
		Description: "删除数据库（危险操作，必须 confirm=true）。参数: database_name(必需), confirm(必需, true)",
		Params:      []string{"database_name", "confirm"},
	}, handleDeleteDatabase)

	RegisterTool(ToolInfo{
		Name:        "database_service_status",
		Category:    "instance",
		Description: "查看实例服务状态（pg_ctl status，需 PG_DATA_DIR 环境变量指向数据目录）。无参数",
		Params:      []string{},
	}, handleDatabaseServiceStatus)

	RegisterTool(ToolInfo{
		Name:        "start_database_service",
		Category:    "instance",
		Description: "启动实例（pg_ctl start，需 PG_DATA_DIR）。参数: timeout_seconds(可选, 默认60)",
		Params:      []string{"timeout_seconds"},
	}, handleStartDatabaseService)

	RegisterTool(ToolInfo{
		Name:        "stop_database_service",
		Category:    "instance",
		Description: "停止实例（pg_ctl stop -m fast，需 PG_DATA_DIR）。参数: mode(可选, smart|fast|immediate, 默认fast), timeout_seconds(可选, 默认60)",
		Params:      []string{"mode", "timeout_seconds"},
	}, handleStopDatabaseService)

	RegisterTool(ToolInfo{
		Name:        "restart_database_service",
		Category:    "instance",
		Description: "重启实例（pg_ctl restart，需 PG_DATA_DIR）。参数: mode(可选, 默认fast), timeout_seconds(可选, 默认120)",
		Params:      []string{"mode", "timeout_seconds"},
	}, handleRestartDatabaseService)
}

func handleCreateDatabase(params map[string]interface{}) (interface{}, error) {
	dbName := getString(params, "database_name")
	if err := validateIdentifier(dbName); err != nil {
		return nil, fmt.Errorf("数据库名校验失败: %w", err)
	}

	sqlStr := "CREATE DATABASE " + quoteIdent(dbName)
	var with []string
	if owner := getString(params, "owner"); owner != "" {
		if err := validateIdentifier(owner); err != nil {
			return nil, fmt.Errorf("owner 校验失败: %w", err)
		}
		with = append(with, "OWNER = "+quoteIdent(owner))
	}
	if enc := getString(params, "encoding"); enc != "" {
		with = append(with, "ENCODING = "+quoteLiteral(enc))
	}
	if len(with) > 0 {
		sqlStr += " WITH " + strings.Join(with, " ")
	}

	ctx, cancel := toolContext()
	defer cancel()

	if err := database.ExecuteDDL(ctx, sqlStr); err != nil {
		return nil, err
	}
	return map[string]interface{}{"success": true, "database_name": dbName, "sql": sqlStr}, nil
}

func handleDeleteDatabase(params map[string]interface{}) (interface{}, error) {
	if !getBool(params, "confirm") {
		return nil, fmt.Errorf("删除数据库是不可逆的危险操作：必须传入 confirm=true")
	}
	dbName := getString(params, "database_name")
	if err := validateIdentifier(dbName); err != nil {
		return nil, fmt.Errorf("数据库名校验失败: %w", err)
	}

	// WITH (FORCE)：PG13+ 断开现有连接后删除
	sqlStr := "DROP DATABASE " + quoteIdent(dbName) + " WITH (FORCE)"

	ctx, cancel := toolContext()
	defer cancel()

	if err := database.ExecuteDDL(ctx, sqlStr); err != nil {
		return nil, err
	}
	return map[string]interface{}{"success": true, "database_name": dbName}, nil
}

// dataDir 读取 PG_DATA_DIR（pg_ctl 必需）。
func dataDir() (string, error) {
	dir := os.Getenv("PG_DATA_DIR")
	if dir == "" {
		return "", fmt.Errorf("需要设置 PG_DATA_DIR 环境变量指向 PostgreSQL 数据目录")
	}
	return dir, nil
}

func handleDatabaseServiceStatus(params map[string]interface{}) (interface{}, error) {
	dir, err := dataDir()
	if err != nil {
		return nil, err
	}

	ctx, cancel := toolContext()
	defer cancel()

	result, err := runBin(ctx, "pg_ctl", []string{"status", "-D", dir}, 30)
	if err != nil {
		// pg_ctl status 在实例未运行时返回非零，属于正常信息而非错误
		result["running"] = false
		result["success"] = true
		return result, nil
	}
	result["running"] = true
	result["success"] = true
	return result, nil
}

func handleStartDatabaseService(params map[string]interface{}) (interface{}, error) {
	dir, err := dataDir()
	if err != nil {
		return nil, err
	}
	timeoutSecs := getIntDefault(params, "timeout_seconds", 60)

	ctx, cancel := toolContext()
	defer cancel()

	result, err := runBin(ctx, "pg_ctl", []string{"start", "-D", dir, "-w", "-t", fmt.Sprintf("%d", timeoutSecs)}, timeoutSecs+30)
	if err != nil {
		result["success"] = false
		result["error"] = err.Error()
		return result, nil
	}
	result["success"] = true
	return result, nil
}

func handleStopDatabaseService(params map[string]interface{}) (interface{}, error) {
	dir, err := dataDir()
	if err != nil {
		return nil, err
	}
	mode := getString(params, "mode")
	if mode == "" {
		mode = "fast"
	}
	if mode != "smart" && mode != "fast" && mode != "immediate" {
		return nil, fmt.Errorf("mode 必须是 smart/fast/immediate 之一")
	}
	timeoutSecs := getIntDefault(params, "timeout_seconds", 60)

	ctx, cancel := toolContext()
	defer cancel()

	result, err := runBin(ctx, "pg_ctl", []string{"stop", "-D", dir, "-m", mode, "-w", "-t", fmt.Sprintf("%d", timeoutSecs)}, timeoutSecs+30)
	if err != nil {
		result["success"] = false
		result["error"] = err.Error()
		return result, nil
	}
	result["success"] = true
	return result, nil
}

func handleRestartDatabaseService(params map[string]interface{}) (interface{}, error) {
	dir, err := dataDir()
	if err != nil {
		return nil, err
	}
	mode := getString(params, "mode")
	if mode == "" {
		mode = "fast"
	}
	if mode != "smart" && mode != "fast" && mode != "immediate" {
		return nil, fmt.Errorf("mode 必须是 smart/fast/immediate 之一")
	}
	timeoutSecs := getIntDefault(params, "timeout_seconds", 120)

	ctx, cancel := toolContext()
	defer cancel()

	result, err := runBin(ctx, "pg_ctl", []string{"restart", "-D", dir, "-m", mode, "-w", "-t", fmt.Sprintf("%d", timeoutSecs)}, timeoutSecs+30)
	if err != nil {
		result["success"] = false
		result["error"] = err.Error()
		return result, nil
	}
	result["success"] = true
	return result, nil
}
