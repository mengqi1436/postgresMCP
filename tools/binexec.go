package tools

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"pg-mcp/config"
)

// outputMaxBytes 外部进程输出的截断上限。
const outputMaxBytes = 8192

// resolveBin 解析 PG 工具链二进制（pg_dump/pg_restore/pg_basebackup/pg_ctl/pg_verifybackup）。
// 优先 PG_BIN_PATH 目录，其次 PATH。
func resolveBin(name string) (string, error) {
	if binPath := config.Get().BinPath; binPath != "" {
		full := filepath.Join(binPath, name)
		if _, err := os.Stat(full); err == nil {
			return full, nil
		}
		if runtime.GOOS == "windows" {
			if _, err := os.Stat(full + ".exe"); err == nil {
				return full + ".exe", nil
			}
		}
	}
	if p, err := exec.LookPath(name); err == nil {
		return p, nil
	}
	return "", fmt.Errorf("找不到二进制 '%s'：请设置 PG_BIN_PATH 指向 PostgreSQL bin 目录，或确认其在 PATH 中", name)
}

// pgEnv 为外部工具注入连接环境变量（工具不读取 MCP 的连接池配置）。
func pgEnv() []string {
	cfg := config.Get()
	return append(os.Environ(),
		"PGHOST="+cfg.Host,
		fmt.Sprintf("PGPORT=%d", cfg.Port),
		"PGUSER="+cfg.User,
		"PGPASSWORD="+cfg.Password,
	)
}

// runBin 执行外部二进制并返回 {exit_code, output}；output 截断到 outputMaxBytes。
func runBin(ctx context.Context, bin string, args []string, timeoutSecs int) (map[string]interface{}, error) {
	full, err := resolveBin(bin)
	if err != nil {
		return nil, err
	}
	if timeoutSecs <= 0 {
		timeoutSecs = 300
	}

	cctx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSecs)*time.Second)
	defer cancel()

	cmd := exec.CommandContext(cctx, full, args...)
	cmd.Env = pgEnv()
	out, runErr := cmd.CombinedOutput()

	output := string(out)
	truncated := false
	if len(output) > outputMaxBytes {
		output = output[:outputMaxBytes]
		truncated = true
	}

	result := map[string]interface{}{
		"binary":    filepath.Base(full),
		"exit_code": cmd.ProcessState.ExitCode(),
		"output":    output,
		"truncated": truncated,
	}
	if cctx.Err() == context.DeadlineExceeded {
		return result, fmt.Errorf("%s 执行超时（%d 秒）", bin, timeoutSecs)
	}
	if runErr != nil {
		return result, fmt.Errorf("%s 执行失败: %w（输出见 output 字段）", bin, runErr)
	}
	return result, nil
}
