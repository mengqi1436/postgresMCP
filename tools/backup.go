package tools

import (
	"fmt"
	"os"
	"path/filepath"

	"pg-mcp/config"
)

func init() { registerBackupTools() }

func registerBackupTools() {
	RegisterTool(ToolInfo{
		Name:        "logical_export",
		Category:    "backup",
		Description: "逻辑导出（pg_dump -Fc 自定义格式，支持选择性恢复）。参数: output_file(必需), tables(可选, 仅导出指定表), extra_args(可选), timeout_seconds(可选, 默认300)",
		Params:      []string{"output_file", "tables", "extra_args", "timeout_seconds"},
	}, handleLogicalExport)

	RegisterTool(ToolInfo{
		Name:        "logical_import",
		Category:    "backup",
		Description: "逻辑导入（pg_restore）。参数: input_file(必需), database(可选), clean(可选, 先删后建), create_db(可选, 连同建库), jobs(可选, 并行度), section(可选, pre-data|data|post-data), extra_args(可选), timeout_seconds(可选, 默认600)",
		Params:      []string{"input_file", "database", "clean", "create_db", "jobs", "section", "extra_args", "timeout_seconds"},
	}, handleLogicalImport)

	RegisterTool(ToolInfo{
		Name:        "physical_backup",
		Category:    "backup",
		Description: "物理备份（pg_basebackup -X stream，备份自包含 WAL；完成后自动 pg_verifybackup 校验）。参数: backup_dir(必需), backup_name(可选, 子目录名), verify(可选, 默认true), timeout_seconds(可选, 默认1800)",
		Params:      []string{"backup_dir", "backup_name", "verify", "timeout_seconds"},
	}, handlePhysicalBackup)

	RegisterTool(ToolInfo{
		Name:        "physical_restore",
		Category:    "backup",
		Description: "物理恢复预检（危险操作，需 confirm=true）：校验备份完整性并返回官方恢复步骤。PostgreSQL 的物理恢复需要停实例、替换数据目录、配置恢复参数，本工具不自动执行替换。参数: backup_dir(必需), confirm(必需, true)",
		Params:      []string{"backup_dir", "confirm"},
	}, handlePhysicalRestore)
}

// targetDSN 外部工具用的 -d 参数：PG_DSN 覆盖优先，否则用当前库名。
func targetDSN(database string) string {
	cfg := config.Get()
	if cfg.RawDSN != "" {
		return cfg.RawDSN
	}
	if database != "" {
		return database
	}
	return cfg.Database
}

func handleLogicalExport(params map[string]interface{}) (interface{}, error) {
	outputFile := getString(params, "output_file")
	if outputFile == "" {
		return nil, fmt.Errorf("参数 output_file 是必需的")
	}

	args := []string{"-Fc", "-f", outputFile}
	for _, t := range getStringSlice(params, "tables") {
		args = append(args, "-t", t)
	}
	args = append(args, getStringSlice(params, "extra_args")...)
	args = append(args, "-d", targetDSN(""))

	ctx, cancel := toolContext()
	defer cancel()

	result, err := runBin(ctx, "pg_dump", args, getIntDefault(params, "timeout_seconds", 300))
	if err != nil {
		result["success"] = false
		result["error"] = err.Error()
		return result, nil
	}
	if fi, statErr := os.Stat(outputFile); statErr == nil {
		result["file_size"] = fi.Size()
	}
	result["success"] = true
	result["output_file"] = outputFile
	return result, nil
}

func handleLogicalImport(params map[string]interface{}) (interface{}, error) {
	inputFile := getString(params, "input_file")
	if inputFile == "" {
		return nil, fmt.Errorf("参数 input_file 是必需的")
	}
	if _, err := os.Stat(inputFile); err != nil {
		return nil, fmt.Errorf("备份文件不存在: %s", inputFile)
	}

	db := targetDSN(getString(params, "database"))
	args := []string{"-d", db}
	if getBool(params, "clean") {
		args = append(args, "--clean")
	}
	if getBool(params, "create_db") {
		args = append(args, "--create")
	}
	if jobs := getIntDefault(params, "jobs", 0); jobs > 1 {
		args = append(args, "-j", fmt.Sprintf("%d", jobs))
	}
	if section := getString(params, "section"); section != "" {
		args = append(args, "--section", section)
	}
	args = append(args, getStringSlice(params, "extra_args")...)
	args = append(args, inputFile)

	ctx, cancel := toolContext()
	defer cancel()

	result, err := runBin(ctx, "pg_restore", args, getIntDefault(params, "timeout_seconds", 600))
	if err != nil {
		// pg_restore 警告（如对象已存在）也走非零退出码，输出仍在 result 中
		result["success"] = false
		result["error"] = err.Error()
		return result, nil
	}
	result["success"] = true
	return result, nil
}

func handlePhysicalBackup(params map[string]interface{}) (interface{}, error) {
	backupDir := getString(params, "backup_dir")
	if backupDir == "" {
		return nil, fmt.Errorf("参数 backup_dir 是必需的")
	}
	target := backupDir
	if name := getString(params, "backup_name"); name != "" {
		target = filepath.Join(backupDir, name)
	}

	args := []string{"-D", target, "-X", "stream", "-c", "fast"}
	ctx, cancel := toolContext()
	defer cancel()

	result, err := runBin(ctx, "pg_basebackup", args, getIntDefault(params, "timeout_seconds", 1800))
	if err != nil {
		result["success"] = false
		result["error"] = err.Error()
		return result, nil
	}
	result["backup_path"] = target

	// 官方建议：用 pg_verifybackup 校验备份清单完整性
	verify := true
	if v, ok := params["verify"].(bool); ok {
		verify = v
	}
	if verify {
		vctx, vcancel := toolContext()
		defer vcancel()
		vResult, vErr := runBin(vctx, "pg_verifybackup", []string{target}, 300)
		if vErr != nil {
			result["verify_ok"] = false
			result["verify_output"] = vResult
			result["success"] = false
			result["error"] = "备份完成但完整性校验失败: " + vErr.Error()
			return result, nil
		}
		result["verify_ok"] = true
	}
	result["success"] = true
	return result, nil
}

func handlePhysicalRestore(params map[string]interface{}) (interface{}, error) {
	if !getBool(params, "confirm") {
		return nil, fmt.Errorf("物理恢复是危险操作：必须传入 confirm=true")
	}
	backupDir := getString(params, "backup_dir")
	if backupDir == "" {
		return nil, fmt.Errorf("参数 backup_dir 是必需的")
	}
	if _, err := os.Stat(backupDir); err != nil {
		return nil, fmt.Errorf("备份目录不存在: %s", backupDir)
	}

	ctx, cancel := toolContext()
	defer cancel()

	// 预检：校验备份清单
	vResult, vErr := runBin(ctx, "pg_verifybackup", []string{backupDir}, 300)

	steps := []string{
		"1. 停止 PostgreSQL 实例（pg_ctl stop -D <数据目录>）",
		"2. 备份/移走当前数据目录（保留现场以便回退）",
		"3. 将备份目录内容复制到数据目录（确保属主为 postgres、权限 0700）",
		"4. 如需 PITR：在数据目录创建 recovery.signal，配置 restore_command 与 recovery_target_*",
		"5. 启动实例（pg_ctl start -D <数据目录>），观察日志确认恢复完成",
		"6. 恢复完成后删除 recovery.signal（PITR 场景实例会自动删除）",
	}

	return map[string]interface{}{
		"success":    vErr == nil,
		"verify_ok":  vErr == nil,
		"verify":     vResult,
		"backup_dir": backupDir,
		"note":       "PostgreSQL 物理恢复需要停实例并替换数据目录，本工具仅做预检与步骤指引，不自动执行替换",
		"steps":      steps,
	}, nil
}
