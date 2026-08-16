#!/usr/bin/env node
'use strict';

// postgresqlmcp-server launcher
// 定位 postinstall 下载的 Go 二进制并作为子进程运行，透传 stdio
// （MCP stdio 传输：JSON-RPC 走 stdin/stdout）。

const { spawn } = require('child_process');
const path = require('path');
const fs = require('fs');

const isWindows = process.platform === 'win32';
const assetName = isWindows ? 'pg-mcp.exe' : 'pg-mcp-linux';
const binPath = path.join(__dirname, process.platform, assetName);

if (!fs.existsSync(binPath)) {
  console.error(`[pg-mcp] 二进制未找到: ${binPath}`);
  console.error('[pg-mcp] 请运行 npm rebuild postgresqlmcp-server 或重新安装该包。');
  process.exit(1);
}

const child = spawn(binPath, process.argv.slice(2), { stdio: 'inherit' });

child.on('error', (err) => {
  console.error(`[pg-mcp] 启动二进制失败: ${err.message}`);
  process.exit(1);
});

child.on('exit', (code, signal) => {
  if (signal) {
    // 子进程被信号终止时，向自身转发同样的信号
    process.kill(process.pid, signal);
  } else {
    process.exit(code === null ? 0 : code);
  }
});
