#!/usr/bin/env node
'use strict';

// pgsql-server-mcp postinstall
// 根据当前平台/架构，从 GitHub Releases 下载对应版本的 Go 二进制，
// 存到 bin/<platform>/ 下供 launcher (bin/pg-mcp.js) 调用。
// 版本来自 package.json（CI 发布时与 GitHub Release tag 对齐，如 v2.2.0）。

const https = require('https');
const http = require('http');
const fs = require('fs');
const path = require('path');
const { execFileSync } = require('child_process');

const PKG_VERSION = process.env.npm_package_version;
const REPO = 'mengqi1436/postgresMCP';

// 平台/架构 → 资产名映射（与 .github/workflows/release.yml 的矩阵一致）
const ASSETS = {
  linux: {
    x64: 'pg-mcp-linux',
  },
  win32: {
    x64: 'pg-mcp.exe',
  },
};

const platform = process.platform;
const arch = process.arch;

const asset = ASSETS[platform] && ASSETS[platform][arch];
if (!asset) {
  // 不支持的平台不报硬错误（例如 darwin 用户仍可安装，只是没有可用二进制）
  console.warn(
    `[pg-mcp] 当前平台/架构 (${platform}/${arch}) 暂无可用二进制，跳过下载。支持: linux/x64, win32/x64`,
  );
  process.exit(0);
}

const tag = PKG_VERSION ? `v${PKG_VERSION}` : 'latest';
const url = `https://github.com/${REPO}/releases/download/${tag}/${asset}`;

const destDir = path.join(__dirname, '..', 'bin', platform);
const destPath = path.join(destDir, asset);

function download(url, redirectsLeft = 5) {
  return new Promise((resolve, reject) => {
    const client = url.startsWith('https:') ? https : http;
    const req = client.get(url, { headers: { 'User-Agent': 'pgsql-server-mcp-install' } }, (res) => {
      if (res.statusCode >= 300 && res.statusCode < 400 && res.headers.location) {
        res.resume();
        if (redirectsLeft <= 0) {
          reject(new Error(`重定向次数过多: ${url}`));
          return;
        }
        // GitHub 302 到 signed URL，绝对/相对均处理
        const next = new URL(res.headers.location, url).toString();
        download(next, redirectsLeft - 1).then(resolve, reject);
        return;
      }
      if (res.statusCode !== 200) {
        res.resume();
        reject(new Error(`下载失败 HTTP ${res.statusCode}: ${url}`));
        return;
      }
      const chunks = [];
      res.on('data', (c) => chunks.push(c));
      res.on('end', () => resolve(Buffer.concat(chunks)));
    });
    req.on('error', reject);
    req.setTimeout(60000, () => {
      req.destroy(new Error('下载超时'));
    });
  });
}

(async () => {
  try {
    console.log(`[pg-mcp] 下载 ${url}`);
    const data = await download(url);
    fs.mkdirSync(destDir, { recursive: true });
    fs.writeFileSync(destPath, data, { mode: 0o755 });
    if (platform !== 'win32') {
      try {
        execFileSync('chmod', ['+x', destPath]);
      } catch (_) {
        /* Windows 无 chmod，忽略 */
      }
    }
    console.log(`[pg-mcp] 已安装到 ${destPath} (${data.length} bytes)`);
  } catch (err) {
    console.error(`[pg-mcp] 二进制下载失败: ${err.message}`);
    console.error('[pg-mcp] 请确认网络可访问 GitHub，或设置 npm_config_ignore_scripts=true 跳过安装。');
    process.exit(1);
  }
})();
